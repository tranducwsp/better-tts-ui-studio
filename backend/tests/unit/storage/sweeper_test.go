package storage_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"backend/storage"
)

// newStore creates a local store in the test's temporary directory.
func newStore(t *testing.T) (storage.Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := storage.NewLocalStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return s, root
}

// writeAged writes an object then backdates its modification time.
//
// Touches the file directly rather than going through the Store because the Store has no API
// for setting timestamps — and the sweeper decides based on that very timestamp, so the test
// must be able to control it.
func writeAged(t *testing.T, s storage.Store, root, key string, size int, age time.Duration) {
	t.Helper()
	if err := s.Put(context.Background(), key, bytes.NewReader(make([]byte, size))); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
	full := filepath.Join(root, filepath.FromSlash(key))
	old := time.Now().Add(-age)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatalf("change time %s: %v", key, err)
	}
}

func TestSweepRemovesOnlyExpired(t *testing.T) {
	s, root := newStore(t)
	ctx := context.Background()

	writeAged(t, s, root, "temp/old1.wav", 1000, 48*time.Hour)
	writeAged(t, s, root, "temp/old2.mp3", 2000, 25*time.Hour)
	writeAged(t, s, root, "temp/fresh.wav", 500, 1*time.Hour)
	writeAged(t, s, root, "temp/borderline.wav", 300, 23*time.Hour)

	// Saved voices live outside the temp branch and must not be touched.
	writeAged(t, s, root, "clone/user-1/voice/keeper.wav", 900, 100*time.Hour)

	n, freed, err := storage.SweepTempObjects(ctx, s, 24*time.Hour)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted %d objects, expected 2", n)
	}
	if freed != 3000 {
		t.Errorf("freed %d bytes, expected 3000", freed)
	}

	for _, key := range []string{"temp/fresh.wav", "temp/borderline.wav"} {
		if ok, _ := s.Exists(ctx, key); !ok {
			t.Errorf("%s should still exist", key)
		}
	}
	for _, key := range []string{"temp/old1.wav", "temp/old2.mp3"} {
		if ok, _ := s.Exists(ctx, key); ok {
			t.Errorf("%s should have been deleted", key)
		}
	}
	if ok, _ := s.Exists(ctx, "clone/user-1/voice/keeper.wav"); !ok {
		t.Error("saved voice should not have been touched")
	}
}

func TestSweepMissingPrefixIsNotAnError(t *testing.T) {
	s, _ := newStore(t)

	n, freed, err := storage.SweepTempObjects(context.Background(), s, time.Hour)
	if err != nil {
		t.Errorf("nonexistent branch should not be an error, got: %v", err)
	}
	if n != 0 || freed != 0 {
		t.Errorf("expected 0/0, got %d/%d", n, freed)
	}
}

func TestSweepEmptyPrefix(t *testing.T) {
	s, root := newStore(t)
	writeAged(t, s, root, "temp/fresh.wav", 10, 0)

	n, _, err := storage.SweepTempObjects(context.Background(), s, time.Hour)
	if err != nil || n != 0 {
		t.Errorf("no expired objects: n=%d err=%v", n, err)
	}
}
