package handlers

import (
	"net/http"

	"core-backend/db/sqlc"
	"core-backend/middleware"

	"github.com/bytedance/sonic"
)

// writeJSON trả về một payload JSON kèm mã trạng thái.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(payload)
}

// writeError trả về một lỗi theo đúng hình dạng {"detail": ...} mà Frontend đọc.
func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

// currentUser lấy người dùng đã xác thực, tự trả 401 nếu không có.
//
// Mọi route gọi hàm này đều đã nằm sau RequireActiveUser hoặc RequireAdmin, nên nhánh
// !ok là lưới an toàn cho trường hợp một route mới bị mắc vào nhóm không có middleware —
// giữ lại thì rẻ, mà mất thì hỏng âm thầm. Trước đây chín handler viết lại nguyên khối
// này, nên sửa hình dạng lỗi ở một chỗ sẽ bỏ sót tám chỗ còn lại.
func currentUser(w http.ResponseWriter, r *http.Request) (*sqlc.User, bool) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return nil, false
	}
	return user, true
}
