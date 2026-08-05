package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore lưu đối tượng thành tệp dưới một thư mục gốc.
type LocalStore struct {
	root string
}

// NewLocalStore chốt thư mục gốc và đảm bảo nó tồn tại.
func NewLocalStore(root string) (*LocalStore, error) {
	if root == "" {
		root = "storage"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

// resolve đổi một khoá thành đường dẫn tuyệt đối, và từ chối khoá thoát khỏi gốc.
//
// Hai lớp, vì mỗi lớp bắt một thứ khác nhau. safeSegment làm sạch từng đoạn nên "../" không
// sống sót qua nó. Phép kiểm prefix sau đó là lưới an toàn cho những gì lớp đầu chưa nghĩ tới
// — symlink, khoá tuyệt đối, hay một lần sửa sau này làm hỏng hàm làm sạch. Chi phí là một
// lần so chuỗi trên mỗi thao tác; cái giá của việc thiếu nó là ghi tệp ra ngoài thư mục
// storage, đúng lỗ hổng đã vá ở đường upload.
func (s *LocalStore) resolve(key string) (string, error) {
	clean := path(key)
	if clean == "" {
		return "", ErrNotFound
	}

	full := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrNotFound
	}
	return full, nil
}

// path làm sạch từng đoạn của khoá và ghép lại theo dấu phân cách của hệ điều hành.
func path(key string) string {
	parts := strings.Split(key, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		out = append(out, safeSegment(p))
	}
	return filepath.Join(out...)
}

func (s *LocalStore) Put(_ context.Context, key string, data []byte) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	// Ghi ra tệp tạm rồi đổi tên: rename trong cùng một thư mục là thao tác nguyên tử trên
	// POSIX, nên người đọc thấy hoặc bản cũ hoặc bản mới trọn vẹn, không bao giờ thấy một tệp
	// đang viết dở. Không có nó, một lượt tải trùng đúng lúc ghi sẽ nhận được đoạn âm thanh cụt.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // không sao nếu rename đã thành công
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, full)
}

func (s *LocalStore) Get(_ context.Context, key string) ([]byte, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(full)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	return data, err
}

func (s *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	return f, err
}

func (s *LocalStore) Exists(_ context.Context, key string) (bool, error) {
	full, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(full)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List quét đúng một cấp dưới prefix và bỏ qua thư mục con.
//
// Không đệ quy có chủ ý: người gọi duy nhất là bộ quét dọn, mà nó chỉ được phép đụng vào
// temp/ — giọng người dùng đã lưu nằm ở nhánh khác và một lần đệ quy nhầm sẽ xoá chúng.
func (s *LocalStore) List(_ context.Context, prefix string) ([]ObjectInfo, error) {
	dir, err := s.resolve(prefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // chưa có gì được ghi
		}
		return nil, err
	}

	out := make([]ObjectInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // tệp vừa bị xoá giữa lúc quét
		}
		out = append(out, ObjectInfo{
			Key:      strings.TrimPrefix(prefix+"/"+e.Name(), "/"),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}
	return out, nil
}
