package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// RepoRoot returns the repository root directory based on the location of this helper file,
// independent of the working directory or test package depth. The marker catches errors if the
// source tree is packaged incorrectly.
func RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine testsupport location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "backend", "go.mod")); err != nil {
		t.Fatalf("cannot find repo root from %s: %v", file, err)
	}
	return root
}

// Path builds an absolute path to a file within the repository.
func Path(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{RepoRoot(t)}, parts...)...)
}
