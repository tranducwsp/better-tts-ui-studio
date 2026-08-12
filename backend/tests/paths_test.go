package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"backend/storage"
)

// TestVoiceKey_ContainsTraversal locks in the defense at the key-construction layer.
//
// The handler already validates model_id against the Manifest, but the key-construction function
// must not trust that: one forgotten check at the upper layer is enough to write outside the
// user's subtree, with attacker-controlled content.
func TestVoiceKey_ContainsTraversal(t *testing.T) {
	cases := []struct {
		name     string
		modeID   string
		userID   string
		filename string
	}{
		{"mode traverses up", "../../../../etc", "user-1", "a.wav"},
		{"mode is dot-dot", "..", "user-1", "a.wav"},
		{"user traverses up", "clone", "../../root", "a.wav"},
		{"slash traversal in mode", "clone/../../etc", "user-1", "a.wav"},
		{"filename traverses up", "clone", "user-1", "../../../passwd"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := storage.VoiceKey(c.modeID, c.userID, c.filename)

			for _, seg := range strings.Split(got, "/") {
				if seg == ".." {
					t.Errorf("VoiceKey(%q,%q,%q) = %q — still contains traversal component",
						c.modeID, c.userID, c.filename, got)
				}
			}
			if strings.HasPrefix(got, "/") {
				t.Errorf("VoiceKey(...) = %q — key must not be an absolute path", got)
			}
		})
	}
}

// TestVoiceKey_KeepsValidNames confirms the sanitization layer does not break valid names.
func TestVoiceKey_KeepsValidNames(t *testing.T) {
	got := storage.VoiceKey("zero_shot_clone", "019f90a6-d5c8-7795-bae5-de6ae408d880", "abc.wav")
	want := "zero_shot_clone/019f90a6-d5c8-7795-bae5-de6ae408d880/voice/abc.wav"
	if got != want {
		t.Errorf("valid key was modified: got %q, want %q", got, want)
	}
}

// TestLocalStore_RejectsEscapingKeys is the final safety net.
//
// Checked at the store layer, not the key layer: even if a caller constructs a key manually
// instead of using VoiceKey/TempKey, the store must not write outside its root.
func TestLocalStore_RejectsEscapingKeys(t *testing.T) {
	root := t.TempDir()
	s, err := storage.NewLocalStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	outside := filepath.Join(filepath.Dir(root), "escaped.txt")

	for _, key := range []string{
		"../escaped.txt",
		"../../escaped.txt",
		"temp/../../escaped.txt",
		"/etc/escaped.txt",
	} {
		// Whether the write succeeds or is rejected is fine; what must NOT happen is a file
		// appearing outside the root.
		_ = s.Put(ctx, key, bytes.NewReader([]byte("x")))

		if _, err := os.Stat(outside); err == nil {
			os.Remove(outside)
			t.Fatalf("key %q was written outside the store root", key)
		}
	}
}

// TestLocalStore_RoundTrip checks what the handler actually relies on.
func TestLocalStore_RoundTrip(t *testing.T) {
	s, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()
	const key = "temp/abc.wav"

	if ok, _ := s.Exists(ctx, key); ok {
		t.Error("key was never written but already exists")
	}
	if _, err := s.Get(ctx, key); err != storage.ErrNotFound {
		t.Errorf("reading a nonexistent key must return ErrNotFound, got %v", err)
	}

	if err := s.Put(ctx, key, bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil || string(got) != "hello" {
		t.Errorf("read back: %q, %v", got, err)
	}

	// Deleting a nonexistent key is not an error: the caller wants it gone, and it is gone.
	if err := s.Delete(ctx, "temp/khong-ton-tai.wav"); err != nil {
		t.Errorf("deleting a nonexistent key should not be an error: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Errorf("delete: %v", err)
	}
	if ok, _ := s.Exists(ctx, key); ok {
		t.Error("key still exists after delete")
	}
}
