package handlers_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/handlers"
)

func createMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	if _, err := part.Write(content); err != nil {
		t.Fatalf("Failed to write form content: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/extract-text", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestExtractText_TxtFile(t *testing.T) {
	h := handlers.NewUtilsHandler()
	content := []byte("Hello world from unit test TXT file!")

	req := createMultipartRequest(t, "sample.txt", content)
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if res["text"] != "Hello world from unit test TXT file!" {
		t.Errorf("Unexpected extracted text: %s", res["text"])
	}
}

func TestExtractText_DocxFile(t *testing.T) {
	h := handlers.NewUtilsHandler()

	// Create a mock DOCX zip in memory
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("Failed to create mock docx zip entry: %v", err)
	}
	xmlData := `<?xml version="1.0" encoding="UTF-8"?><w:document><w:body><w:p><w:r><w:t>Hello DOCX Text</w:t></w:r></w:p></w:body></w:document>`
	_, _ = f.Write([]byte(xmlData))
	_ = zw.Close()

	req := createMultipartRequest(t, "sample.docx", zipBuf.Bytes())
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["text"] != "Hello DOCX Text" {
		t.Errorf("Expected 'Hello DOCX Text', got '%s'", res["text"])
	}
}

func TestExtractText_UnsupportedFormat(t *testing.T) {
	h := handlers.NewUtilsHandler()

	req := createMultipartRequest(t, "sample.exe", []byte("binary data"))
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for unsupported format, got %d", rec.Code)
	}
}

func TestExtractText_MissingFile(t *testing.T) {
	h := handlers.NewUtilsHandler()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/extract-text", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 when file is missing, got %d", rec.Code)
	}
}

// buildZipBomb builds a zip file whose named entry decompresses to size bytes.
//
// The content is a single repeated character so deflate compresses it extremely well: this is
// exactly the shape of a file crafted to balloon, not a real document.
func buildZipBomb(t *testing.T, name string, size int) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}

	chunk := bytes.Repeat([]byte("A"), 1<<20)
	for written := 0; written < size; written += len(chunk) {
		n := len(chunk)
		if remain := size - written; remain < n {
			n = remain
		}
		if _, err := f.Write(chunk[:n]); err != nil {
			t.Fatalf("failed to write zip content: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return buf.Bytes()
}

// TestExtractText_RejectsZipBomb locks in the decompression ceiling for DOCX and ODT.
//
// MaxBytesReader only limits the COMPRESSED file size, and both formats are zip — so before
// this ceiling, a sub-megabyte file would decompress to hundreds of megabytes in RAM, and a
// few concurrent requests would be enough to bring down the process.

func TestExtractText_RejectsZipBomb(t *testing.T) {
	cases := []struct {
		filename string
		entry    string
	}{
		{"bomb.docx", "word/document.xml"},
		{"bomb.odt", "content.xml"},
	}

	for _, c := range cases {
		t.Run(c.filename, func(t *testing.T) {
			// 80 MB decompressed, above the 64 MB ceiling. Compressed is only a few dozen KB.
			payload := buildZipBomb(t, c.entry, 80<<20)
			if len(payload) > 4<<20 {
				t.Fatalf("compressed file %d bytes — too large, test case no longer resembles a zip bomb", len(payload))
			}

			h := handlers.NewUtilsHandler()
			req := createMultipartRequest(t, c.filename, payload)
			rec := httptest.NewRecorder()

			h.ExtractText(rec, req)

			if rec.Code == http.StatusOK {
				t.Errorf("zip bomb %s must be rejected, but got 200", c.filename)
			}
		})
	}
}

// TestExtractText_AcceptsLargeButSaneDocument confirms the ceiling does not touch real
// documents.
//
// A very long Word file still only has a few megabytes of text, so it should pass normally —
// otherwise the fix would have traded one vulnerability for blocking legitimate users.
func TestExtractText_AcceptsLargeButSaneDocument(t *testing.T) {
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	body := bytes.Repeat([]byte("<w:p><w:r><w:t>xin chao</w:t></w:r></w:p>"), 20000)
	_, _ = f.Write([]byte(`<?xml version="1.0"?><w:document><w:body>`))
	_, _ = f.Write(body)
	_, _ = f.Write([]byte(`</w:body></w:document>`))
	_ = zw.Close()

	h := handlers.NewUtilsHandler()
	req := createMultipartRequest(t, "long.docx", zipBuf.Bytes())
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("long but valid document must be accepted, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestExtractText_DoesNotLeakParserError ensures error messages do not carry internal details.
//
// Previously this branch returned the raw err.Error() from the parser library, meaning temporary
// file paths and internal structure went straight out. The uploader only needs to know the file
// could not be read; the cause belongs in the log.
func TestExtractText_DoesNotLeakParserError(t *testing.T) {
	h := handlers.NewUtilsHandler()

	// Not a real DOCX: DOCX is a zip file, so garbage bytes cause the library to error.
	req := createMultipartRequest(t, "broken.docx", []byte("this is not a zip file"))
	rec := httptest.NewRecorder()

	h.ExtractText(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("unreadable file is an input error, expect 400, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, leak := range []string{"zip:", "/tmp/", "archive/", ".go:"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries internal detail %q: %s", leak, body)
		}
	}
}
