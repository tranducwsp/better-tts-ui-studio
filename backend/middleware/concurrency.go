package middleware

import "net/http"

// ConcurrencyLimit caps the number of concurrently processed requests.
//
// This is a *content-agnostic* gate: body limits (BodyLimit, MAX_UPLOAD_SIZE_MB) and the
// per-request RAM ceiling in the handler (handlers/utils.go) are measured per request, but N
// concurrent requests still multiply RAM by N. With a given max, the route group's memory
// ceiling is max * (per-request RAM ceiling), predictable and not controllable by input.
//
// Requests exceeding the limit wait in the queue instead of erroring immediately — legitimate
// users get through when a slot opens, a few seconds slower is acceptable for best-effort
// operations like text extraction. If the client disconnects while waiting, the request leaves
// the queue and does no work.
func ConcurrencyLimit(max int) func(http.Handler) http.Handler {
	sem := make(chan struct{}, max)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-r.Context().Done():
				// Client has disconnected; do not occupy a slot, do not serve.
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
