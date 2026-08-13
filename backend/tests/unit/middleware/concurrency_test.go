package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend/middleware"
)

// TestConcurrencyLimit_CapsActive enforces the memory-bloat guard for /extract-text:
// N concurrent requests must not exceed max being processed at once, regardless of how many
// arrive simultaneously. Active count is measured by an atomic counter in the handler; its
// peak must be ≤ max.
func TestConcurrencyLimit_CapsActive(t *testing.T) {
	const (
		max     = 8
		workers = 50
	)

	var active, peak atomic.Int32
	handler := middleware.ConcurrencyLimit(max)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := active.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			// Hold the slot long enough for the remaining requests to queue up — if the middleware
			// were not blocking, all 50 requests would run concurrently and push the peak to 50.
			time.Sleep(2 * time.Millisecond)
			active.Add(-1)
			w.WriteHeader(http.StatusOK)
		}),
	)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > max {
		t.Fatalf("peak active request count %d exceeds limit %d", got, max)
	}
}
