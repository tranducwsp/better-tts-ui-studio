package manifest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"backend/types"
)

// Manifest đầy đủ được công bố trong docs/GATEWAY.md phải đi qua bộ kiểm sạch sẽ — không lỗi
// và không cảnh báo nào. Đọc thẳng block JSON trong Markdown để tài liệu là nguồn duy nhất; một
// bản JSON chép riêng sẽ sớm muộn trôi khỏi ví dụ mà AI Engineer thực sự đọc và sao chép.
//
// Đây là hàng rào hai chiều. Nếu ai đó thêm một quy tắc kiểm quá chặt, test này hỏng ngay thay
// vì để người vận hành thấy cảnh báo trên một Engine lành mạnh rồi học cách bỏ qua mọi cảnh báo.
// Nếu ví dụ trong tài liệu lỗi thời hoặc không còn là manifest hợp lệ, CI cũng báo ngay.
func TestDocumentedManifestPassesValidation(t *testing.T) {
	const path = "../../../../docs/GATEWAY.md"
	const heading = "## 💻 3. MẪU MANIFEST CHUẨN ĐẦY ĐỦ"

	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("không đọc được %s: %v", path, err)
	}

	sectionAt := bytes.Index(doc, []byte(heading))
	if sectionAt < 0 {
		t.Fatalf("%s không còn mục %q", path, heading)
	}
	section := doc[sectionAt:]

	const openFence = "```json\n"
	openAt := bytes.Index(section, []byte(openFence))
	if openAt < 0 {
		t.Fatalf("mục manifest trong %s không có block ```json", path)
	}
	payload := section[openAt+len(openFence):]

	closeAt := bytes.Index(payload, []byte("\n```"))
	if closeAt < 0 {
		t.Fatalf("block manifest trong %s không có fence đóng", path)
	}

	var m types.UniversalManifest
	if err := json.Unmarshal(payload[:closeAt], &m); err != nil {
		t.Fatalf("block manifest trong %s không phải JSON hợp lệ: %v", path, err)
	}

	errs, warnings := m.Validate()
	for _, w := range warnings {
		t.Logf("cảnh báo: %s", w)
	}
	if len(errs) > 0 {
		t.Errorf("manifest trong tài liệu bị từ chối: %v", errs)
	}
	if len(warnings) > 0 {
		t.Errorf("manifest trong tài liệu sinh %d cảnh báo; hoặc ví dụ đã lỗi thời, hoặc quy tắc kiểm quá chặt", len(warnings))
	}
}
