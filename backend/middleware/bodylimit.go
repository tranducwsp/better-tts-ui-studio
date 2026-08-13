package middleware

import (
	"net/http"
	"strings"
)

// maxJSONBodyBytes is the body ceiling for JSON/text-field API payloads.
//
// Previously there was no ceiling: Sonic would read the body to EOF, and any arbitrarily large
// body would be fully loaded into RAM. 2 MiB is more than enough for every valid request
// (synthesis text <=3000 characters per manifest, uploads are intercepted separately) and
// small enough that a huge payload cannot exhaust backend memory.
const jsonBodyLimit = 2 << 20 // 2 MiB

// BodyLimit caps the request body to jsonBodyLimit.
//
// Multipart/form-data is excluded: voice uploads go up to MAX_UPLOAD_SIZE_MB (default 256MB)
// and the upload handlers already set their own MaxBytesReader — applying this 2 MiB ceiling
// on top of them would break file upload functionality.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := r.Header.Get("Content-Type"); !strings.HasPrefix(c, "multipart/") {
			r.Body = http.MaxBytesReader(w, r.Body, jsonBodyLimit)
		}
		next.ServeHTTP(w, r)
	})
}
