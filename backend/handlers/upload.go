package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"

	"backend/state"
)

// Mặc định khớp MAX_UPLOAD_SIZE_MB=50 trong config/settings.go để các handler được dựng
// độc lập trong test không có một trần 32MB bí mật. Bootstrap production luôn ghi đè bằng
// giá trị Config đã validate.
var maxUploadBytes int64 = 50 << 20

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
// Chỉ cần phần đầu tệp (head) chứ không phải toàn bộ: chữ ký định dạng nằm ở vài byte đầu, và
// trần kích thước so trên size của part do multipart khai, không phải độ dài của một bản copy
// trong RAM. Thuộc tính accept của trình duyệt chỉ là gợi ý cho hộp chọn tệp; client nào cũng
// gửi được thứ khác. Kiểm cả chữ ký vì đổi tên .mp3 thành .wav là việc dễ làm nhất.
func checkReferenceAudio(filename string, head []byte, size int64, modeID string) error {
	// Không có Manifest thì allowlist định dạng sẽ là mặc định của nền tảng chứ không phải
	// thứ Engine khai đọc được — một giới hạn trông như đang áp dụng nhưng nói về engine khác.
	m := state.GlobalManifestState.Get()
	if m == nil {
		return errors.New("chưa có manifest từ AI Engine, tạm thời không nhận tệp tham chiếu")
	}
	spec := m.ResolveAudioSpec(modeID)

	ext := ""
	if i := strings.LastIndex(filename, "."); i != -1 {
		ext = strings.ToLower(filename[i+1:])
	}

	accepted := spec.ReferenceAudioFormats
	if !slices.Contains(accepted, ext) {
		return fmt.Errorf("chỉ nhận %s cho âm thanh tham chiếu", strings.ToUpper(strings.Join(accepted, ", ")))
	}

	if check, known := audioSignatures[ext]; known && !check(head) {
		return fmt.Errorf("tệp không phải %s hợp lệ dù mang phần mở rộng .%s", strings.ToUpper(ext), ext)
	}

	if max := spec.MaxReferenceBytes; max > 0 && size > max {
		return fmt.Errorf("âm thanh tham chiếu vượt quá %d MB mà engine nhận được", max>>20)
	}
	return nil
}

// multipartMemoryLimit là ngưỡng dữ liệu tệp multipart được giữ trong RAM khi phân tích form.
//
// ParseMultipartForm giữ các part tệp trong bộ nhớ tới mức này; part lớn hơn được ghi ra tệp
// tạm trên đĩa. Truyền limit (trần upload) làm tham số này là sai: nó bảo Go giữ một tệp lên
// tới 256MB trong RAM chỉ để phân tích form, rồi người gọi lại đọc nó lần nữa. 32MB là vừa
// đủ cho tệp tham chiếu điển hình (một đoạn ghi âm vài giây) mà vẫn đưa được file lớn xuống đĩa.
const multipartMemoryLimit = 32 << 20 // 32 MiB

// parseUpload đọc form multipart trong giới hạn đã cấu hình.
//
// MaxBytesReader chặn ở tầng kết nối nên một tệp quá lớn bị ngắt ngay khi truyền, thay vì
// được nạp hết vào RAM rồi mới từ chối. Trả về thông báo nêu rõ giới hạn, vì "request quá
// lớn" mà không nói lớn hơn bao nhiêu thì người dùng không biết phải cắt bớt tới đâu.
func parseUpload(w http.ResponseWriter, r *http.Request) error {
	limit := uploadLimit()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
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
//
// Không giữ Data []byte như cũ: một tệp tham chiếu dài cả phút là hàng chục MB, đọc hết vào
// RAM chỉ để người gọi ghi sang nơi khác là hai bản copy. Người gọi stream thẳng từ File.

type referenceUpload struct {
	ModelID  string
	Filename string
	File     multipart.File
}

// receiveReferenceAudio chốt mode → kiểm phần đầu tệp → trả lại handle stream ở vị trí 0.
//
// Chỉ đọc vừa đủ vài byte đầu để kiểm chữ ký, rồi Seek về 0 thay vì io.ReadAll toàn tệp.
// parseUpload nằm ngoài hàm này có chủ ý: r.FormValue chỉ đọc được sau khi form được phân
// tích, và phần lớn tệp đó nằm trên đĩa (multipartMemoryLimit) — nuốt hết một tệp 100 MB vào
// RAM rồi mới phát hiện thiếu tên giọng là đúng thứ mà MaxBytesReader trong parseUpload được
// đặt ở đó để tránh.
//
// Người gọi có trách nhiệm đóng File và gọi r.MultipartForm.RemoveAll() sau khi dùng xong —
// part spill ra đĩa cần được xoá, còn tệp nào nằm trong RAM thì Close là đủ.
func receiveReferenceAudio(w http.ResponseWriter, r *http.Request) (referenceUpload, bool) {
	// Không đoán tên mode: hỏi Manifest xem Mode nào thực sự hỗ trợ cloning. Chuỗi "clone"
	// cứng trước đây khiến giọng của một Engine đặt tên mode là zero_shot_clone bị lưu dưới
	// một model_id không tồn tại, nên sau đó không mode nào liệt kê được nó.
	modelID := r.FormValue("model_id")
	if modelID == "" {
		modelID = firstCloningMode()
	}

	// model_id đi vào khoá lưu trữ bên dưới (storage.VoiceKey), nên nó phải là một Mode
	// Engine thực sự khai — không phải chuỗi bất kỳ, vì nó được dùng nguyên văn để ghép
	// đường dẫn / khoá. Trước đây model_id=../../.. đưa os.WriteFile ra ngoài thư mục storage,
	// với nội dung do người gửi kiểm soát. ResolveAudioSpec không chắn được vì nó lặng lẽ
	// quay về mặc định khi gặp mode lạ, nên chỗ chặn phải là ở đây.
	if !state.GlobalManifestState.HasMode(modelID) {
		writeError(w, http.StatusBadRequest, "model_id không phải là mode mà engine hỗ trợ")
		return referenceUpload{}, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Thiếu tệp âm thanh")
		return referenceUpload{}, false
	}

	// Đọc phần đầu đủ cho kiểm chữ ký (chữ ký của các định dạng đã biết nằm trong vài byte
	// đầu), rồi đưa con trỏ về 0 để người gọi stream được lại từ đầu. Không cần đọc toàn tệp.
	head, _ := io.ReadAll(io.LimitReader(file, signaturePeekLen))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		writeError(w, http.StatusInternalServerError, "Không đọc lại được tệp âm thanh")
		return referenceUpload{}, false
	}

	if err := checkReferenceAudio(header.Filename, head, header.Size, modelID); err != nil {
		file.Close()
		writeError(w, http.StatusBadRequest, err.Error())
		return referenceUpload{}, false
	}

	return referenceUpload{ModelID: modelID, Filename: header.Filename, File: file}, true
}

// signaturePeekLen là số byte cần đọc đầu tệp để nhận các chữ ký định dạng đã biết.
//
// Chữ ký dài nhất đang kiểm là wav (RIFF…WAVE) với 12 byte; 512 byte cho dư rất nhiều và vẫn
// là một lượng bỏ qua được so với hàng trăm MB của tệp âm thanh.
const signaturePeekLen = 512

// cleanupMultipartForm dọn part tệp do ParseMultipartForm để lại.
//
// Với multipartMemoryLimit, mọi part lớn hơn ngưỡng nằm trên đĩa dưới dạng tệp tạm; không
// gọi RemoveAll thì chúng tích tụ cho tới khi process thoát. Gọi ở cuối handler của mọi route
// dùng parseUpload, sau khi mọi luồng đã đọc xong part đó.
func cleanupMultipartForm(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}
