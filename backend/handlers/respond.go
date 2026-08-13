package handlers

import (
	"net/http"

	"backend/db/sqlc"
	"backend/middleware"

	"github.com/bytedance/sonic"
)

// writeJSON returns a JSON payload with a status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(payload)
}

// writeError returns an error in the exact {"detail": ...} shape that the Frontend reads.
func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

// currentUser retrieves the authenticated user, returning 401 itself if absent.
//
// Every route calling this function is already behind RequireActiveUser or RequireAdmin, so the
// !ok branch is a safety net for the case where a new route is accidentally placed in a group
// without middleware — keeping it is cheap, losing it is silent breakage. Previously nine handlers
// re-wrote this entire block, so fixing the error shape in one place would miss the other eight.
func currentUser(w http.ResponseWriter, r *http.Request) (*sqlc.User, bool) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return nil, false
	}
	return user, true
}
