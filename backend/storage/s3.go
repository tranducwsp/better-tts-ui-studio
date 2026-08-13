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

// S3Config holds everything needed to talk to an S3-compatible store.
type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string // empty for real AWS; set for MinIO/R2/Spaces

	// Leave both empty to use an IAM role (IRSA on k8s, instance profile on EC2).
	AccessKey string
	SecretKey string

	ForcePathStyle bool
	Prefix         string
}

// S3Store stores objects on an S3-compatible store.
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
	prefix  string
}

// NewS3Store builds the client and confirms the bucket is actually usable.
//
// Check the bucket at construction time rather than waiting for the first write: wrong
// credentials or a mistyped bucket name that still starts means the error only surfaces when
// the first user finishes synthesizing and cannot retrieve their audio — the same class of
// silent breakage that the startup manifest port check was created to prevent.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("S3_BUCKET must not be empty when STORAGE_BACKEND=s3")
	}

	// Half a key pair is a configuration error, not an intent: either provide both, or leave
	// both empty to use an IAM role (IRSA on k8s, instance profile on EC2).
	hasID, hasSecret := cfg.AccessKey != "", cfg.SecretKey != ""
	if hasID != hasSecret {
		return nil, errors.New("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY must both be set or both empty " +
			"(leave both empty to use an IAM role)")
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
		// MinIO and most self-hosted stores serve by path (endpoint/bucket/key) rather than
		// by subdomain (bucket.endpoint/key), because subdomains require wildcard DNS.
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

// full prepends the shared prefix to the key, after sanitizing each component.
//
// Sanitization happens here for the same reason as the local backend: the caller is expected
// to have validated input, but the storage layer should not trust that. With S3, "../" does
// not escape the bucket, but it creates bizarre keys that other tools cannot list or delete.
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

// isNotFound recognizes "key not found" among other errors.
//
// S3 returns NoSuchKey for GetObject but NotFound for HeadObject, and MinIO does not always
// use the same type — so check both instead of trusting one.
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	return errors.As(err, &nf)
}

func (s *S3Store) Put(ctx context.Context, key string, src io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
		Body:   src,
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
	// S3 considers deleting a non-existent key a success, matching the Store contract.
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.full(key)),
	})
	return err
}

// List enumerates objects directly under prefix, without descending into sub-branches.
//
// Delimiter "/" preserves the same constraint the local backend gets from non-recursive
// ReadDir: the sweeper is only allowed to touch the temp branch, and saved user voices live
// in a different branch.
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

	// Paginate rather than a single call: ListObjectsV2 returns at most 1000 keys, and
	// ignoring the remainder means the sweeper silently never reaches the tail of the list.
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			key := aws.ToString(o.Key)
			// Return the key as the caller provided it, otherwise they cannot delete it.
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

// PresignGet signs a direct download URL, so the client retrieves the object without going
// through the backend.
//
// Signed using the same client used for all other operations, so the URL carries the domain
// from S3_ENDPOINT. That is correct when that domain is reachable from both the backend and
// the browser — the case of a DNS-only record. If the backend talks to the store over an
// internal-only address (minio.minio.svc.cluster.local, or a pinned IP), the signed URL will
// point to a location the browser cannot reach; in that case a separate public endpoint is
// needed for signing.
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
