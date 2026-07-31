package tests

import (
	"context"
	"os"
	"testing"

	"core-backend/storage"
)

func TestLocalStorage_SaveAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	provider := storage.NewLocalStorage(tempDir)

	ctx := context.Background()
	testData := []byte("fake audio data stream")

	// Test SaveAudio
	filePath, err := provider.SaveAudio(ctx, "temp", "test_audio.wav", testData)
	if err != nil {
		t.Fatalf("SaveAudio failed: %v", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected file to exist at %s", filePath)
	}

	// Test GetURL
	publicURL, err := provider.GetURL(ctx, filePath)
	if err != nil || publicURL == "" {
		t.Errorf("GetURL failed or returned empty: %v, url: %s", err, publicURL)
	}

	// Test DeleteAudio
	err = provider.DeleteAudio(ctx, filePath)
	if err != nil {
		t.Fatalf("DeleteAudio failed: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Expected file to be deleted at %s", filePath)
	}
}
