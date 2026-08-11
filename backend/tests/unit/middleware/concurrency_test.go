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

// TestConcurrencyLimit_CapsActive yêu cầu chốt chặn độ phình bộ nhớ của /extract-text:
// N request đồng thời không được vượt quá max đang xử lý một lúc, bất kể có bao nhiêu
// tới cùng lúc. Số active đo bằng atomic counter trong handler; đỉnh của nó phải ≤ max.
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
			// Giữ chỗ đủ lâu để các request còn lại kịp xếp hàng — nếu middleware không
			// chặn, 50 request này cùng lúc sẽ nâng đỉnh lên 50.
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
		t.Fatalf("đỉnh số request đang xử lý %d vượt hạn mức %d", got, max)
	}
}
