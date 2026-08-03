package handlers

import (
	"log"
	"net/http"

	"core-backend/client"
	"core-backend/state"

	"github.com/bytedance/sonic"
)

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

	// Cập nhật RAM Cache nội bộ
	state.GlobalManifestState.Set(manifest)
	log.Printf("🚀 Đã đồng bộ lại AI Engine Manifest thành công: %s (v%s) [Max Text: %d]",
		manifest.EngineName, manifest.Version, manifest.Constraints.MaxTextLength)

	// Bắn tín hiệu bất đồng bộ sang frontend-builder container để re-prerender HTML tĩnh
	if h.FEBuilderURL != "" {
		go func(url string) {
			rebuildTarget := url + "/rebuild"
			resp, err := http.Post(rebuildTarget, "application/json", nil)
			if err != nil {
				log.Printf("⚠️ Tín hiệu trigger FE Builder thất bại (%s): %v", rebuildTarget, err)
				return
			}
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
