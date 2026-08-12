package handlers_test

import (
	"os"
	"testing"

	"backend/storage"
)

// TestMain prepares storage.Global for handler tests that need storage access.
//
// storage.Global is normally set by main.go at startup, so in tests it is nil and the
// first handler touching storage would panic. Set up a local storage in a temp directory to keep
// tests independent and leave nothing on the machine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "backend-handler-tests-*")
	if err != nil {
		panic("cannot create temp storage directory for tests: " + err.Error())
	}

	if err := storage.Init("local", dir, storage.S3Config{}); err != nil {
		panic("cannot initialize storage for tests: " + err.Error())
	}

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}
