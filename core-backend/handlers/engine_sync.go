package handlers

import (
	"io"
	"log"
	"net/http"
	"time"

	"core-backend/client"
	"core-backend/state"

	"github.com/bytedance/sonic"
)

// rebuildSignalTimeout giới hạn lượt gọi webhook sang frontend-builder.
//
// Builder chỉ cần NHẬN được tín hiệu; nó trả lời 202 ngay rồi dựng bundle ở nền, nên chỗ này
// không bao giờ phải chờ một lượt build. Vài giây là quá đủ để biết tín hiệu đã tới.
const rebuildSignalTimeout = 10 * time.Second

// rebuildClient tách khỏi http.DefaultClient vì DefaultClient KHÔNG có timeout.
//
// Trước đây webhook dùng http.Post, tức là DefaultClient: nếu builder mở cổng nhưng không trả
// lời — đúng trạng thái nó rơi vào khi đang bận build — goroutine dưới đây chờ vô hạn. Mỗi
// lần reload manifest thêm một goroutine kẹt vĩnh viễn, và không có gì trong log nói rằng
// chúng đang tích lại.
var rebuildClient = &http.Client{Timeout: rebuildSignalTimeout}

// EngineSyncHandler xử lý các yêu cầu đồng bộ cấu hình/manifest từ AI Engine nội bộ.
type EngineSyncHandler struct {
	TTSClient    *client.CoreTTSClient
	FEBuilderURL string
}

// NewEngineSyncHandler khởi tạo EngineSyncHandler với CoreTTSClient và FEBuilderURL.
func NewEngineSyncHandler(ttsClient *client.CoreTTSClient, feBuilderURL string) *EngineSyncHandler {
	return &EngineSyncHandler{
		TTSClient:    ttsClient,
		FEBuilderURL: feBuilderURL,
	}
}

// ReloadManifest xử lý yêu cầu POST /api/internal/engine/reload.
// Tự động truy vấn lại Manifest mới nhất từ Core AI Engine và kích hoạt re-build HTML tĩnh ở Frontend Builder.
func (h *EngineSyncHandler) ReloadManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	manifest, err := h.TTSClient.GetInfo()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Không thể kết nối đến Core AI Engine để đồng bộ Manifest: " + err.Error(),
		})
		return
	}

	if manifest == nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Manifest nhận được từ Core AI Engine rỗng",
		})
		return
	}

	// Cập nhật RAM Cache nội bộ. Manifest mâu thuẫn bị từ chối và bản đang dùng giữ nguyên,
	// nên một lần reload lỗi không làm hệ thống tệ hơn lúc trước khi gọi.
	if err := state.GlobalManifestState.Set(manifest); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Manifest từ Core AI Engine không hợp lệ, đã giữ lại bản đang dùng: " + err.Error(),
		})
		return
	}
	log.Printf("🚀 Đã đồng bộ lại AI Engine Manifest thành công: %s (v%s) [Max Text: %d]",
		manifest.EngineName, manifest.Version, manifest.Constraints.MaxTextLength)

	// Bắn tín hiệu bất đồng bộ sang frontend-builder container để re-prerender HTML tĩnh
	if h.FEBuilderURL != "" {
		go func(url string) {
			rebuildTarget := url + "/rebuild"

			req, err := http.NewRequest(http.MethodPost, rebuildTarget, nil)
			if err != nil {
				log.Printf("⚠️ Không dựng được yêu cầu tới FE Builder (%s): %v", rebuildTarget, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := rebuildClient.Do(req)
			if err != nil {
				log.Printf("⚠️ Tín hiệu trigger FE Builder thất bại (%s): %v", rebuildTarget, err)
				return
			}
			// Đọc cạn body trước khi đóng để kết nối được tái sử dụng thay vì bị bỏ đi.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			log.Printf("⚡ Đã bắn tín hiệu kích hoạt re-build static HTML thành công sang FE Builder (%s)", rebuildTarget)
		}(h.FEBuilderURL)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"message":     "Engine manifest synchronized successfully",
		"engine_name": manifest.EngineName,
		"version":     manifest.Version,
	})
}
