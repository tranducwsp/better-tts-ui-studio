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

// buildZipBomb dựng một tệp zip mà entry name giải nén ra size byte.
//
// Nội dung lặp một ký tự nên deflate nén cực tốt: đây đúng là hình dạng của một tệp được
// dựng riêng để phình ra, chứ không phải một tài liệu thật.
func buildZipBomb(t *testing.T, name string, size int) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("không tạo được entry zip: %v", err)
	}

	chunk := bytes.Repeat([]byte("A"), 1<<20)
	for written := 0; written < size; written += len(chunk) {
		n := len(chunk)
		if remain := size - written; remain < n {
			n = remain
		}
		if _, err := f.Write(chunk[:n]); err != nil {
			t.Fatalf("không ghi được nội dung zip: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("không đóng được zip: %v", err)
	}
	return buf.Bytes()
}

// TestExtractText_RejectsZipBomb khoá lại trần giải nén cho DOCX và ODT.
//
// MaxBytesReader chỉ chặn kích thước tệp NÉN, mà cả hai định dạng đều là zip — nên trước khi
// có trần này, một tệp dưới một megabyte giải nén thành hàng trăm megabyte trong RAM, và vài
// request song song là đủ hạ tiến trình.
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
			// 80 MB giải nén, trên trần 64 MB. Nén lại chỉ còn khoảng vài chục KB.
			payload := buildZipBomb(t, c.entry, 80<<20)
			if len(payload) > 4<<20 {
				t.Fatalf("tệp nén %d byte — quá lớn, ca kiểm thử không còn mô tả zip bomb", len(payload))
			}

			h := handlers.NewUtilsHandler()
			req := createMultipartRequest(t, c.filename, payload)
			rec := httptest.NewRecorder()

			h.ExtractText(rec, req)

			if rec.Code == http.StatusOK {
				t.Errorf("zip bomb %s phải bị từ chối, nhưng nhận được 200", c.filename)
			}
		})
	}
}

// TestExtractText_AcceptsLargeButSaneDocument xác nhận trần không chạm tài liệu thật.
//
// Một tệp Word rất dài vẫn chỉ có vài megabyte văn bản, nên nó phải đi qua bình thường —
// nếu không, bản vá đã đổi một lỗ hổng lấy một lỗi từ chối người dùng hợp lệ.
func TestExtractText_AcceptsLargeButSaneDocument(t *testing.T) {
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("không tạo được entry zip: %v", err)
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
		t.Errorf("tài liệu dài nhưng hợp lệ phải được nhận, got %d: %s", rec.Code, rec.Body.String())
	}
}
