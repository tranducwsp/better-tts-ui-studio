package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bytedance/sonic"
)

// maxExtractBytes là trần số byte đọc từ file upload vào RAM cho việc bóc chữ.
//
// Trước đây ExtractText đọc CẢ upload (đến MAX_UPLOAD_SIZE_MB, tối đa 10 GB) và giữ trong RAM
// suốt quá trình bóc — với file PDF large, fallback còn copy `string(data)` thêm lần nữa.
// Bóc chữ là best-effort: văn bản thật của một tài liệu hiếm khi vượt vài MB, nên đọc quá
// ngưỡng này cắt bớt không làm mất gì đáng kể, và một request không còn kéo theo nguy cơ tốn
// cả GB RAM. Với file .txt chỉ cần chữ đầu, đọc thấp hơn nữa (xem ExtractText).
const maxExtractBytes = 32 << 20 // 32 MiB

// maxExtractOutputBytes là trần kích thước text trả về, tính theo byte.
//
// 200 KiB rộng hơn hầu hết văn bản mà người dùng muốn xem/xoá vào TTS; chặn builder phình theo
// tài liệu (một DOCX giải nén tới 64 MB text vẫn được bóc hết chỉ để lấy 200 KiB).
const maxExtractOutputBytes = 200 << 10 // 200 KiB

// maxPdfMatches là trần số match regex xử lý khi bóc PDF.
//
// Một PDF crafted có thể chứa trăm nghìn thẻ `(..) Tj`; FindAllSubmatchIndex trả về match thành
// limiting slice — không chặn thì dựng slice vô hạn và tốn CPU ở mọi input lớn.
const maxPdfMatches = 100_000

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
	defer cleanupMultipartForm(r)

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Document file is required"})
		return
	}
	defer file.Close()

	// Đọc giới hạn theo định dạng: với .txt chỉ ghi phần đầu vào textarea, nên chỉ cần lấy đủ
	// trần output — không mang bản sao hàng chục MB về cho vài trăm KB dùng được. PDF/zip thì
	// cần đủ buffer để regex scan / trình giải nén chạy, nên dùng trần input 32 MiB.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	readLimit := int64(maxExtractBytes)
	if ext == ".txt" {
		readLimit = maxExtractOutputBytes
	}

	content, err := io.ReadAll(io.LimitReader(file, readLimit))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read file content"})
		return
	}

	extractedText := ""

	switch ext {
	case ".txt":
		extractedText = string(content)

	case ".pdf":
		extractedText, err = extractPDFText(content)
		if err != nil {
			// Lỗi của thư viện phân tích chỉ vào log: nó mang đường dẫn và chi tiết nội bộ,
			// còn người tải tệp lên thì chỉ cần biết tệp này không đọc được.
			log.Printf("Không đọc được tệp PDF: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Không đọc được nội dung tệp PDF"})
			return
		}

	case ".docx":
		extractedText, err = extractDOCXText(content)
		if err != nil {
			// Lỗi của thư viện phân tích chỉ vào log: nó mang đường dẫn và chi tiết nội bộ,
			// còn người tải tệp lên thì chỉ cần biết tệp này không đọc được.
			log.Printf("Không đọc được tệp DOCX: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Không đọc được nội dung tệp DOCX"})
			return
		}

	case ".odt":
		extractedText, err = extractODTText(content)
		if err != nil {
			// Lỗi của thư viện phân tích chỉ vào log: nó mang đường dẫn và chi tiết nội bộ,
			// còn người tải tệp lên thì chỉ cần biết tệp này không đọc được.
			log.Printf("Không đọc được tệp ODT: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Không đọc được nội dung tệp ODT"})
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
		"text": strings.TrimSpace(truncateText(extractedText, maxExtractOutputBytes)),
	})
}

// truncateText cắt chuỗi về tối đa maxBytes, dừng ở biên giới ký tự UTF-8.
//
// Plain `s[:maxBytes]` có thể cắt giữa một rune → cuối text là một ký tự hỏng. JSON encoder xử
// lý được nhưng người dùng thấy một ô vuông lạ ở cuối bài; lùi về biên giới rune thì đẹp hơn.
func truncateText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
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

	// FindAllSubmatchIndex trả về cặp chỉ số vào data thay vì thuộc cấp phát []byte cho từng
	// match như FindAllSubmatch, và tham số thứ hai chặn số match — với input đã bị cắt nhưng
	// vẫn có thể chứa trăm nghìn thẻ, một slice match vô hạn là cách tốn cả RAM lẫn CPU.
	matches := rePDFTextObj.FindAllSubmatchIndex(data, maxPdfMatches)

	for _, m := range matches {
		// m là dãy cặp [start,end] liên tiếp: [0:2] toàn khớp, [2:4] nhóm 1 `(...) Tj`,
		// [4:6] nhóm 2 `[...] TJ`. Nhóm nào không tham gia thì chỉ số của nó là -1.
		switch {
		case len(m) >= 4 && m[2] >= 0:
			sb.Write(data[m[2]:m[3]])
			sb.WriteByte(' ')
		case len(m) >= 6 && m[4] >= 0:
			array := data[m[4]:m[5]]
			// Trong mảng `[...]` mỗi phần tử lại là `(text)`, và đó là những gì người dùng cần.
			subs := rePDFStringInArray.FindAllSubmatchIndex(array, maxPdfMatches)
			for _, sm := range subs {
				if len(sm) >= 4 && sm[2] >= 0 {
					sb.Write(array[sm[2]:sm[3]])
				}
			}
			sb.WriteByte(' ')
		}

		if sb.Len() >= maxExtractOutputBytes {
			break
		}
	}

	res := sb.String()
	if strings.TrimSpace(res) == "" {
		res = extractPDFReadableRun(data)
	}

	return res, nil
}

// extractPDFReadableRun là fallback quét text đọc được bằng regex khi không tìm thấy thẻ chữ.
//
// Chạy trên []byte trực tiếp thay vì `string(data)`: bản cũ copy nguyên input thành string rồi
// mới quét — với file vừa chạm trần input, đó là thêm ~32 MiB RAM không cần thiết.
func extractPDFReadableRun(data []byte) string {
	var sb strings.Builder
	sb.Grow(maxExtractOutputBytes)

	limit := maxExtractOutputBytes
	for _, m := range rePDFReadableRun.FindAll(data, maxPdfMatches) {
		if len(m) >= limit {
			sb.Write(m[:limit])
			break
		}
		sb.Write(m)
		sb.WriteByte('\n')
		limit -= len(m) + 1
		if limit <= 0 {
			break
		}
	}
	return sb.String()
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
