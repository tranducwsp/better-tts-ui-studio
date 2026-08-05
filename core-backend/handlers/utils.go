package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
)

// UtilsHandler xử lý các API tiện ích hỗ trợ đọc và bóc tách văn bản từ file tài liệu upload.
type UtilsHandler struct{}

// NewUtilsHandler khởi tạo UtilsHandler.
func NewUtilsHandler() *UtilsHandler {
	return &UtilsHandler{}
}

// ExtractText bóc tách văn bản thô từ các định dạng tài liệu được tải lên (.txt, .pdf, .docx, .odt).
func (h *UtilsHandler) ExtractText(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := parseUpload(w, r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Document file is required"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read file content"})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	extractedText := ""

	switch ext {
	case ".txt":
		extractedText = string(content)

	case ".pdf":
		extractedText, err = extractPDFText(content)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read PDF file: " + err.Error()})
			return
		}

	case ".docx":
		extractedText, err = extractDOCXText(content)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read DOCX file: " + err.Error()})
			return
		}

	case ".odt":
		extractedText, err = extractODTText(content)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read ODT file: " + err.Error()})
			return
		}

	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"detail": "Unsupported file format. Supported formats: .txt, .pdf, .docx, .odt",
		})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"text": strings.TrimSpace(extractedText),
	})
}

// maxDocumentXMLBytes là trần cho phần XML đã giải nén của một tệp DOCX/ODT.
//
// MaxBytesReader trong parseUpload chỉ chặn kích thước tệp NÉN, mà DOCX và ODT đều là zip:
// một tệp 597 KB giải nén ra 600 MB (đã đo, khuếch đại ~1000x), nên trần đầu vào không nói
// gì về lượng RAM việc bóc chữ sẽ dùng. Vài request song song là đủ hạ tiến trình, và người
// gửi chỉ cần một tài khoản đã được duyệt.
//
// 64 MB rộng hơn mọi tài liệu văn bản thực tế nhiều lần — văn bản thuần trong một tệp Word
// nghìn trang vẫn ở mức vài MB — nên trần này không chạm tới người dùng thật, chỉ chạm tệp
// được dựng riêng để phình ra.
const maxDocumentXMLBytes = 64 << 20

// readZipEntry giải nén một entry trong zip với trần cố định.
//
// Kiểm hai lần, vì mỗi lần bắt một kiểu tệp khác nhau: UncompressedSize64 lấy từ header của
// zip nên chặn được trước khi đọc byte nào, nhưng header do người tạo tệp ghi và nói dối
// được. LimitReader là thứ thực sự chặn, đo trên dữ liệu đã giải nén. Đọc thêm một byte quá
// trần để phân biệt "vừa đủ" với "vượt".
func readZipEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxDocumentXMLBytes {
		return nil, fmtError(fmt.Sprintf(
			"nội dung tài liệu sau giải nén (%d MB) vượt quá giới hạn %d MB",
			f.UncompressedSize64>>20, maxDocumentXMLBytes>>20))
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, maxDocumentXMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocumentXMLBytes {
		return nil, fmtError(fmt.Sprintf(
			"nội dung tài liệu sau giải nén vượt quá giới hạn %d MB", maxDocumentXMLBytes>>20))
	}
	return data, nil
}

// extractDOCXText đọc file DOCX (Zip archive) và parse XML word/document.xml để lấy toàn bộ chữ.
func extractDOCXText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	var docFile *zip.File
	for _, f := range reader.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmtError("document.xml không tồn tại trong DOCX")
	}

	xmlBytes, err := readZipEntry(docFile)
	if err != nil {
		return "", err
	}

	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var sb strings.Builder

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == "p" {
			}
		case xml.EndElement:
			if elem.Name.Local == "p" {
				sb.WriteString("\n")
			}
		case xml.CharData:
			sb.Write(elem)
		}
	}

	return sb.String(), nil
}

// extractODTText đọc file ODT (OpenDocument Text) và parse XML content.xml để lấy chữ.
func extractODTText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	var contentFile *zip.File
	for _, f := range reader.File {
		if f.Name == "content.xml" {
			contentFile = f
			break
		}
	}
	if contentFile == nil {
		return "", fmtError("content.xml không tồn tại trong ODT")
	}

	xmlBytes, err := readZipEntry(contentFile)
	if err != nil {
		return "", err
	}

	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var sb strings.Builder

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			local := elem.Name.Local
			if local == "s" {
				c := 1
				for _, attr := range elem.Attr {
					if attr.Name.Local == "c" {
						if n, e := strconv.Atoi(attr.Value); e == nil && n > 0 {
							c = n
						}
					}
				}
				sb.WriteString(strings.Repeat(" ", c))
			} else if local == "tab" {
				sb.WriteString("\t")
			} else if local == "line-break" {
				sb.WriteString("\n")
			}
		case xml.EndElement:
			local := elem.Name.Local
			if local == "p" || local == "h" {
				sb.WriteString("\n")
			}
		case xml.CharData:
			sb.Write(elem)
		}
	}

	return sb.String(), nil
}

// Biểu thức chính quy bóc chữ từ PDF, biên dịch một lần lúc nạp gói.
//
// rePDFStringInArray trước đây được biên dịch NGAY TRONG vòng lặp duyệt kết quả khớp: một
// PDF nhiều nghìn mảng chữ là bấy nhiêu lần biên dịch lại đúng một mẫu không đổi. Hai mẫu
// còn lại cũng được biên dịch lại mỗi lần gọi hàm.
var (
	rePDFTextObj       = regexp.MustCompile(`\(([^)]*)\)\s*Tj|\[([^\]]*)\]\s*TJ`)
	rePDFStringInArray = regexp.MustCompile(`\(([^)]*)\)`)
	rePDFReadableRun   = regexp.MustCompile(`[\w\s.,!?;:\"'-]{5,}`)
)

// extractPDFText bóc tách các dòng chữ thô từ PDF Stream Objects.
func extractPDFText(data []byte) (string, error) {
	var sb strings.Builder
	matches := rePDFTextObj.FindAllSubmatch(data, -1)

	for _, m := range matches {
		if len(m[1]) > 0 {
			sb.Write(m[1])
			sb.WriteString(" ")
		} else if len(m[2]) > 0 {
			subMatches := rePDFStringInArray.FindAllSubmatch(m[2], -1)
			for _, sm := range subMatches {
				sb.Write(sm[1])
			}
			sb.WriteString(" ")
		}
	}

	res := sb.String()
	if strings.TrimSpace(res) == "" {
		found := rePDFReadableRun.FindAllString(string(data), -1)
		res = strings.Join(found, "\n")
	}

	return res, nil
}

func fmtError(msg string) error {
	return &customErr{msg: msg}
}

type customErr struct {
	msg string
}

func (e *customErr) Error() string {
	return e.msg
}
