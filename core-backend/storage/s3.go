package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config là những gì cần để nói chuyện với một kho tương thích S3.
type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string // rỗng với AWS thật; đặt cho MinIO/R2/Spaces

	// Bỏ trống cả hai để dùng IAM role (IRSA trên k8s, instance profile trên EC2).
	AccessKey string
	SecretKey string

	ForcePathStyle bool
	Prefix         string
}

// S3Store lưu đối tượng trên một kho tương thích S3.
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
	prefix  string
}

// NewS3Store dựng client và xác nhận bucket thật sự dùng được.
//
// Kiểm bucket ngay lúc khởi tạo chứ không đợi lần ghi đầu: sai thông tin đăng nhập hay gõ
// nhầm tên bucket mà vẫn khởi động được nghĩa là lỗi chỉ lộ ra khi người dùng đầu tiên tổng
// hợp xong và không lấy lại được âm thanh — cùng loại hỏng ngầm mà cổng manifest lúc khởi
// động sinh ra để chặn.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("S3_BUCKET không được để trống khi STORAGE_BACKEND=s3")
	}

	// Một nửa cặp khoá là lỗi cấu hình, không phải ý định: hoặc khai cả hai, hoặc bỏ trống cả
	// hai để dùng IAM role (IRSA trên k8s, instance profile trên EC2).
	hasID, hasSecret := cfg.AccessKey != "", cfg.SecretKey != ""
	if hasID != hasSecret {
		return nil, errors.New("S3_ACCESS_KEY_ID và S3_SECRET_ACCESS_KEY phải cùng có hoặc cùng trống " +
			"(bỏ trống cả hai để dùng IAM role)")
	}

	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if hasID {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// MinIO và phần lớn kho tự dựng phục vụ theo đường dẫn (endpoint/bucket/key) thay vì
		// theo tên miền con (bucket.endpoint/key), vì tên miền con cần wildcard DNS.
		o.UsePathStyle = cfg.ForcePathStyle
	})

	store := &S3Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.Bucket,
		prefix:  strings.Trim(cfg.Prefix, "/"),
	}

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)}); err != nil {
		return nil, err
	}
	return store, nil
}

// full ghép tiền tố dùng chung vào khoá, sau khi làm sạch từng đoạn.
//
// Làm sạch ở đây vì cùng một lý do như bản local: người gọi được kỳ vọng đã kiểm đầu vào,
// nhưng lớp lưu trữ không nên tin điều đó. Với S3 thì "../" không thoát ra khỏi bucket, nhưng
// nó tạo ra khoá kỳ dị mà công cụ khác không liệt kê hay xoá được.
func (s *S3Store) full(key string) string {
	parts := strings.Split(key, "/")
	out := make([]string, 0, len(parts)+1)
	if s.prefix != "" {
		out = append(out, s.prefix)
	}
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		out = append(out, safeSegment(p))
	}
	return strings.Join(out, "/")
}

// isNotFound nhận ra "không có khoá này" giữa các lỗi khác.
//
// S3 trả NoSuchKey cho GetObject nhưng NotFound cho HeadObject, và MinIO không luôn dùng cùng
// một kiểu — nên kiểm cả hai thay vì tin vào một.
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	return errors.As(err, &nf)
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
		Body:   strings.NewReader(string(data)),
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	// S3 coi việc xoá một khoá không tồn tại là thành công, đúng như hợp đồng của Store.
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	})
	return err
}

// List liệt kê các đối tượng ngay dưới prefix, không đi vào nhánh con.
//
// Delimiter "/" giữ đúng ràng buộc mà bản local có sẵn nhờ ReadDir không đệ quy: bộ quét dọn
// chỉ được đụng vào nhánh temp, và giọng người dùng đã lưu nằm ở nhánh khác.
func (s *S3Store) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	full := s.full(prefix)
	if full != "" {
		full += "/"
	}

	var out []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(full),
		Delimiter: aws.String("/"),
	})

	// Phân trang thay vì một lượt gọi: ListObjectsV2 trả tối đa 1000 khoá, và bỏ qua phần còn
	// lại nghĩa là bộ quét dọn lặng lẽ không bao giờ chạm tới đuôi danh sách.
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			key := aws.ToString(o.Key)
			// Trả lại khoá theo cách người gọi truyền vào, nếu không họ không xoá được nó.
			if s.prefix != "" {
				key = strings.TrimPrefix(key, s.prefix+"/")
			}
			info := ObjectInfo{Key: key, Size: aws.ToInt64(o.Size)}
			if o.LastModified != nil {
				info.Modified = o.LastModified.Unix()
			}
			out = append(out, info)
		}
	}
	return out, nil
}

// PresignGet ký một URL tải trực tiếp, để client lấy đối tượng không qua backend.
//
// Ký bằng chính client đang dùng cho mọi thao tác khác, nên URL mang tên miền trong
// S3_ENDPOINT. Điều đó đúng khi tên miền ấy vừa gọi được từ backend vừa gọi được từ trình
// duyệt — trường hợp của một bản ghi DNS-only. Nếu backend nói chuyện với kho qua một địa chỉ
// chỉ nội bộ (minio.minio.svc.cluster.local, hay một IP đã ghim), URL ký ra sẽ trỏ vào chỗ
// trình duyệt không tới được; lúc đó cần một endpoint công khai riêng để ký.
func (s *S3Store) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
