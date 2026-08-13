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

// maxExtractBytes is the cap on bytes read from an uploaded file into RAM for text extraction.
//
// Previously ExtractText read the ENTIRE upload (up to MAX_UPLOAD_SIZE_MB, max 10 GB) and
// held it in RAM throughout extraction — for large PDF files, the fallback also copied
// `string(data)` an additional time. Text extraction is best-effort: the real text of a
// document rarely exceeds a few MB, so reading beyond this threshold and truncating loses
// nothing significant, and a single request no longer risks consuming gigabytes of RAM.
// For .txt files only the beginning is needed, reading even lower (see ExtractText).
const maxExtractBytes = 32 << 20 // 32 MiB

// maxExtractOutputBytes is the cap on returned text size, measured in bytes.
//
// 200 KiB is wider than most text a user would want to view/delete into TTS; prevents the
// builder from ballooning with the document (a DOCX decompressing to 64 MB of text is still
// fully extracted only to take 200 KiB).
const maxExtractOutputBytes = 200 << 10 // 200 KiB

// maxPdfMatches is the cap on regex matches processed during PDF extraction.
//
// A crafted PDF can contain hundreds of thousands of `(..) Tj` tags; FindAllSubmatchIndex
// returns matches into a limiting slice — without the cap it builds an unbounded slice and
// burns CPU on every large input.
const maxPdfMatches = 100_000

// UtilsHandler handles utility APIs for reading and extracting text from uploaded document files.
type UtilsHandler struct{}

// NewUtilsHandler initializes a UtilsHandler.
func NewUtilsHandler() *UtilsHandler {
	return &UtilsHandler{}
}

// ExtractText extracts raw text from uploaded document formats (.txt, .pdf, .docx, .odt).
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

	// Read limit varies by format: for .txt, only the beginning goes into the textarea, so
	// just fetch enough for the output cap — no need to bring tens of MB for a few hundred KB
	// of usable text. PDF/zip need enough buffer for regex scanning / the decompressor to run,
	// so use the 32 MiB input cap.
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
			// Parser library errors go to the log only: they carry paths and internal details,
			// while the file uploader only needs to know that this file could not be read.
			log.Printf("Failed to read PDF file: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read PDF file content"})
			return
		}

	case ".docx":
		extractedText, err = extractDOCXText(content)
		if err != nil {
			// Parser library errors go to the log only: they carry paths and internal details,
			// while the file uploader only needs to know that this file could not be read.
			log.Printf("Failed to read DOCX file: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read DOCX file content"})
			return
		}

	case ".odt":
		extractedText, err = extractODTText(content)
		if err != nil {
			// Parser library errors go to the log only: they carry paths and internal details,
			// while the file uploader only needs to know that this file could not be read.
			log.Printf("Failed to read ODT file: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read ODT file content"})
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

// truncateText truncates a string to at most maxBytes, stopping at a UTF-8 character boundary.
//
// Plain `s[:maxBytes]` can cut in the middle of a rune, leaving a broken character at the
// end. The JSON encoder handles it, but the user sees a strange box at the end of the text;
// backing up to a rune boundary is cleaner.
func truncateText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// maxDocumentXMLBytes is the cap for the decompressed XML portion of a DOCX/ODT file.
//
// MaxBytesReader in parseUpload only caps the COMPRESSED file size, but DOCX and ODT are both
// zip: a 597 KB file decompresses to 600 MB (measured, ~1000x amplification), so the input cap
// says nothing about how much RAM the extraction will consume. A few concurrent requests are
// enough to bring down the process, and the sender only needs a single approved account.
//
// 64 MB is many times wider than any real text document — the plain text in a thousand-page
// Word file is still a few MB — so this cap never touches real users, only files crafted
// specifically to balloon.
const maxDocumentXMLBytes = 64 << 20

// readZipEntry decompresses a zip entry with a fixed ceiling.
//
// Checks twice, because each catches a different file type: UncompressedSize64 comes from the
// zip header so it blocks before reading any bytes, but the header is written by the file
// creator and can lie. LimitReader is the real guard, measured on the decompressed data.
// Reads one extra byte past the cap to distinguish "exactly at cap" from "exceeded".
func readZipEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxDocumentXMLBytes {
		return nil, fmtError(fmt.Sprintf(
			"decompressed document content (%d MB) exceeds the %d MB limit",
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
			"decompressed document content exceeds the %d MB limit", maxDocumentXMLBytes>>20))
	}
	return data, nil
}

// extractDOCXText reads a DOCX file (Zip archive) and parses word/document.xml to extract all text.
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
		return "", fmtError("document.xml does not exist in DOCX")
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

// extractODTText reads an ODT file (OpenDocument Text) and parses content.xml to extract text.
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
		return "", fmtError("content.xml does not exist in ODT")
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

// Regular expressions for extracting text from PDFs, compiled once at package load.
//
// rePDFStringInArray was previously compiled INSIDE the match iteration loop: a PDF with
// thousands of text arrays meant recompiling the same unchanging pattern that many times.
// The other two patterns were also recompiled on every function call.
var (
	rePDFTextObj       = regexp.MustCompile(`\(([^)]*)\)\s*Tj|\[([^\]]*)\]\s*TJ`)
	rePDFStringInArray = regexp.MustCompile(`\(([^)]*)\)`)
	rePDFReadableRun   = regexp.MustCompile(`[\w\s.,!?;:\"'-]{5,}`)
)

// extractPDFText extracts raw text lines from PDF Stream Objects.
func extractPDFText(data []byte) (string, error) {
	var sb strings.Builder

	// FindAllSubmatchIndex returns index pairs into data rather than allocating []byte for each
	// match like FindAllSubmatch, and the second parameter caps the match count — with input
	// already truncated but still potentially containing hundreds of thousands of tags, an
	// unbounded match slice is a way to burn both RAM and CPU.
	matches := rePDFTextObj.FindAllSubmatchIndex(data, maxPdfMatches)

	for _, m := range matches {
		// m is a sequence of [start,end] pairs: [0:2] full match, [2:4] group 1 `(...) Tj`,
		// [4:6] group 2 `[...] TJ`. Non-participating groups have index -1.
		switch {
		case len(m) >= 4 && m[2] >= 0:
			sb.Write(data[m[2]:m[3]])
			sb.WriteByte(' ')
		case len(m) >= 6 && m[4] >= 0:
			array := data[m[4]:m[5]]
			// Inside a `[...]` array each element is `(text)`, and that is what the user needs.
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

// extractPDFReadableRun is a fallback that scans for readable text via regex when no text
// tags are found.
//
// Operates on []byte directly rather than `string(data)`: the old version copied the entire
// input into a string before scanning — for a file at the input cap, that is an extra ~32 MiB
// of unnecessary RAM.
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
