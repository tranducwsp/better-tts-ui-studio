package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"core-backend/handlers"
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
