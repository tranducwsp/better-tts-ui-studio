package tests

import (
	"encoding/json"
	"os"
	"testing"

	"backend/types"
)

// Manifest thật của Engine kèm theo phải đi qua bộ kiểm sạch sẽ — không lỗi và không cảnh
// báo nào.
//
// Đây là hàng rào hai chiều. Nếu ai đó thêm một quy tắc kiểm quá chặt, test này hỏng ngay
// thay vì để người vận hành thấy cảnh báo trên một Engine lành mạnh rồi học cách bỏ qua mọi
// cảnh báo. Nếu ai đó sửa Engine mà quên cập nhật ví dụ đã chụp, nó cũng hỏng.
func TestCapturedManifestPassesValidation(t *testing.T) {
	const path = "../../docs/examples/manifest-engine.json"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("không đọc được %s: %v", path, err)
	}

	var m types.UniversalManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s không phải Manifest hợp lệ: %v", path, err)
	}

	errs, warnings := m.Validate()
	for _, w := range warnings {
		t.Logf("cảnh báo: %s", w)
	}
	if len(errs) > 0 {
		t.Errorf("manifest kèm theo bị từ chối: %v", errs)
	}
	if len(warnings) > 0 {
		t.Errorf("manifest kèm theo sinh %d cảnh báo; hoặc ví dụ đã lệch khỏi Engine, hoặc quy tắc kiểm quá chặt", len(warnings))
	}
}
