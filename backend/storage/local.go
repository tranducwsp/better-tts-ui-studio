package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore stores objects as files under a root directory.
type LocalStore struct {
	root string
}

// NewLocalStore pins the root directory and ensures it exists.
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

// resolve converts a key to an absolute path, and rejects keys that escape the root.
//
// Two layers, because each catches something different. safeSegment cleans each component so
// "../" does not survive it. The prefix check afterwards is a safety net for what the first
// layer did not anticipate — symlinks, absolute keys, or a future edit that breaks the
// sanitizer. The cost is one string comparison per operation; the cost of not having it is
// writing files outside the storage directory, the exact vulnerability patched in the upload
// path.
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

// path sanitizes each component of the key and joins them with the OS separator.
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

func (s *LocalStore) Put(_ context.Context, key string, src io.Reader) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	// Write to a temp file then rename: rename within the same directory is an atomic
	// operation on POSIX, so readers see either the full old version or the full new version,
	// never a partially-written file. Without this, a concurrent download during a write
	// would receive a truncated audio segment.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // no-op if rename has already succeeded
	}()

	if _, err := io.Copy(tmp, src); err != nil {
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

// List scans exactly one level below prefix and skips subdirectories.
//
// Non-recursive by design: the only caller is the sweeper, and it is only allowed to touch
// temp/ — saved user voices live in a different branch and a mistaken recursive scan would
// delete them.
func (s *LocalStore) List(_ context.Context, prefix string) ([]ObjectInfo, error) {
	dir, err := s.resolve(prefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // nothing has been written yet
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
			continue // file was deleted between the scan
		}
		out = append(out, ObjectInfo{
			Key:      strings.TrimPrefix(prefix+"/"+e.Name(), "/"),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}
	return out, nil
}
