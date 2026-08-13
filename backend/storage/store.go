package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is the common answer when a key does not exist.
//
// A dedicated error type because callers need to distinguish "not yet" from "broken": a newly
// created task having no audio is normal and returns 404, while S3 denying permission is an
// incident and must be surfaced. Without this, callers would compare error strings, and
// os's string and S3's string are not the same.
var ErrNotFound = errors.New("storage: object not found")

// Store is where the platform keeps binary files.
//
// Exists so storage becomes a deployment choice rather than an assumption scattered across
// handlers. Previously each handler called os.WriteFile and assembled paths on its own, so
// "where to put files" was rewritten in five places and could not be changed without editing
// all five.
//
// Keys are relative paths using "/" separators (e.g. "temp/<task>.mp3"), matching the key
// shape of S3. The local backend converts to the OS separator internally; callers do not need
// to know.
//
// Every method accepts a Context because the S3 backend is network I/O and must be
// cancellable. The local backend ignores it, but callers write one signature for both.
type Store interface {
	// Put writes content from src to the key. Accepts io.Reader instead of []byte so callers
	// can stream a large file from disk/elsewhere without building it entirely in RAM (the
	// voice upload path previously read the entire file into memory just to pass it here).
	Put(ctx context.Context, key string, src io.Reader) error

	// Get reads the full contents. Returns ErrNotFound if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Open opens a read stream, for large files that should not be loaded entirely into RAM.
	// The caller is responsible for closing. Returns ErrNotFound if the key does not exist.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists reports whether the key exists, without downloading the contents.
	Exists(ctx context.Context, key string) (bool, error)

	// Delete removes the key. A non-existent key is NOT an error: the caller wants it gone,
	// and it is gone — forcing them to handle ErrNotFound here only creates boilerplate at
	// every deletion site.
	Delete(ctx context.Context, key string) error

	// List enumerates keys starting with the prefix, along with modification time and size,
	// so the sweeper can decide what to delete without downloading the contents.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// Presigner is a Store that can issue time-limited direct download URLs.
//
// Optional interface, separated from Store because not every backend can do it: the local
// disk backend has no URL to issue. Callers check via type assertion, so adding a backend
// without presign support later does not require writing a stub method that returns an error.
//
// This type is also why handlers do not need to compare STORAGE_BACKEND against the string
// "s3": they ask "can this store issue URLs" instead of "what is this store's name", so
// adding a new backend does not require handler changes.
type Presigner interface {
	// PresignGet returns a direct download URL, expiring after ttl.
	//
	// Does not check whether the key exists: signing is a local computation, while existence
	// checking is a network call. The caller knows better whether the check is needed.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// ObjectInfo is the metadata the sweeper needs.
type ObjectInfo struct {
	Key      string
	Size     int64
	Modified int64 // Unix seconds
}
