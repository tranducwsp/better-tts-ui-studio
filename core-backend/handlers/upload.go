package handlers

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"core-backend/state"
)

// Giới hạn kích thước upload, đặt một lần lúc khởi động từ MAX_UPLOAD_SIZE_MB.
//
// Trước đây ba handler đều viết cứng `32 << 20`, nên biến môi trường được đọc, được kiểm
// tra khoảng hợp lệ, được in ra log khởi động — rồi không ai thực thi. Đặt nó thành 8 hay
// 512 đều không thay đổi hành vi.
var maxUploadBytes int64 = 32 << 20

// SetMaxUploadMB đặt trần kích thước upload cho mọi handler nhận multipart.
func SetMaxUploadMB(mb int) {
	if mb > 0 {
		maxUploadBytes = int64(mb) << 20
	}
}

// audioSignatures là chữ ký nhận dạng định dạng, để bắt tệp bị đổi tên phần mở rộng.
//
// Chỉ liệt kê những định dạng có chữ ký ổn định ở đầu tệp. Định dạng không có trong bảng
// vẫn được nhận nếu Engine khai — chỉ là nền tảng không kiểm được nội dung, và đoán bừa còn
// tệ hơn là để Engine tự từ chối.
var audioSignatures = map[string]func([]byte) bool{
	"wav":  func(b []byte) bool { return len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE" },
	"flac": func(b []byte) bool { return len(b) >= 4 && string(b[0:4]) == "fLaC" },
	"ogg":  func(b []byte) bool { return len(b) >= 4 && string(b[0:4]) == "OggS" },
	// MP3 có thể mở đầu bằng thẻ ID3 hoặc trực tiếp bằng frame sync (0xFF 0xEx/0xFx).
	"mp3": func(b []byte) bool {
		if len(b) >= 3 && string(b[0:3]) == "ID3" {
			return true
		}
		return len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0
	},
}

// checkReferenceAudio kiểm tra định dạng tệp tham chiếu theo đúng thứ Engine khai đọc được.
//
// Thuộc tính accept của trình duyệt chỉ là gợi ý cho hộp chọn tệp; client nào cũng gửi được
// thứ khác. Kiểm cả chữ ký vì đổi tên .mp3 thành .wav là việc dễ làm nhất.
func checkReferenceAudio(filename string, data []byte, modeID string) error {
	spec := state.GlobalManifestState.Get().ResolveAudioSpec(modeID)

	ext := ""
	if i := strings.LastIndex(filename, "."); i != -1 {
		ext = strings.ToLower(filename[i+1:])
	}

	accepted := spec.ReferenceAudioFormats
	if !slices.Contains(accepted, ext) {
		return fmt.Errorf("chỉ nhận %s cho âm thanh tham chiếu", strings.ToUpper(strings.Join(accepted, ", ")))
	}

	if check, known := audioSignatures[ext]; known && !check(data) {
		return fmt.Errorf("tệp không phải %s hợp lệ dù mang phần mở rộng .%s", strings.ToUpper(ext), ext)
	}

	if max := spec.MaxReferenceBytes; max > 0 && int64(len(data)) > max {
		return fmt.Errorf("âm thanh tham chiếu vượt quá %d MB mà engine nhận được", max>>20)
	}
	return nil
}

// parseUpload đọc form multipart trong giới hạn đã cấu hình.
//
// MaxBytesReader chặn ở tầng kết nối nên một tệp quá lớn bị ngắt ngay khi truyền, thay vì
// được nạp hết vào RAM rồi mới từ chối. Trả về thông báo nêu rõ giới hạn, vì "request quá
// lớn" mà không nói lớn hơn bao nhiêu thì người dùng không biết phải cắt bớt tới đâu.
func parseUpload(w http.ResponseWriter, r *http.Request) error {
	limit := uploadLimit()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		return fmt.Errorf("tệp vượt quá giới hạn %d MB", limit>>20)
	}
	return nil
}

// uploadLimit là trần cho tệp THÔ vừa tải lên, trước khi cắt.
//
// Cao hơn max_reference_bytes có chủ ý: người dùng có thể kéo cả bản ghi dài rồi chỉ lấy vài
// giây, nên chặn ngay ở bước chọn tệp bằng trần của Engine sẽ từ chối một tệp hoàn toàn hợp
// lệ để cắt. Engine khai giá trị này; MAX_UPLOAD_SIZE_MB vẫn là trần cứng của hạ tầng và
// thắng nếu chặt hơn, vì nó nói về RAM và băng thông của deployment chứ không phải về model.
func uploadLimit() int64 {
	limit := maxUploadBytes
	if m := state.GlobalManifestState.Get(); m != nil {
		if declared := m.ResolveAudioSpec("").MaxUploadBytes; declared > 0 && declared < limit {
			limit = declared
		}
	}
	return limit
}

// referenceUpload là tệp tham chiếu đã qua mọi bước kiểm, sẵn sàng để dùng.
type referenceUpload struct {
	ModelID  string
	Filename string
	Data     []byte
}

// receiveReferenceAudio chạy trọn chuỗi phân tích form → chốt mode → đọc tệp → kiểm định dạng.
//
// Hai handler upload (lưu lâu dài và dùng tạm) chỉ khác nhau ở việc làm gì SAU khi có
// tệp hợp lệ, nhưng trước đây lặp lại cả bốn bước trên — nên chúng đã trôi ra khỏi nhau:
// cùng một lỗi thiếu tệp trả về hai thông báo khác ngôn ngữ. Đã ghi phản hồi lỗi thì
// trả ok=false, người gọi chỉ cần return.
func receiveReferenceAudio(w http.ResponseWriter, r *http.Request) (referenceUpload, bool) {
	if err := parseUpload(w, r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return referenceUpload{}, false
	}

	// Không đoán tên mode: hỏi Manifest xem Mode nào thực sự hỗ trợ cloning. Chuỗi "clone"
	// cứng trước đây khiến giọng của một Engine đặt tên mode là zero_shot_clone bị lưu dưới
	// một model_id không tồn tại, nên sau đó không mode nào liệt kê được nó.
	modelID := r.FormValue("model_id")
	if modelID == "" {
		modelID = firstCloningMode()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Thiếu tệp âm thanh")
		return referenceUpload{}, false
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Không đọc được tệp âm thanh")
		return referenceUpload{}, false
	}

	if err := checkReferenceAudio(header.Filename, data, modelID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return referenceUpload{}, false
	}

	return referenceUpload{ModelID: modelID, Filename: header.Filename, Data: data}, true
}
