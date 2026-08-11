package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/db/sqlc"
	"backend/handlers"
	"backend/state"
	"backend/storage"

	"github.com/go-chi/chi/v5"
)

// TestLocalStore_IsNotPresigner là bất biến mà đường phục vụ âm thanh dựa vào.
//
// Handler quyết định chuyển hướng hay ghi bytes bằng một type assertion sang Presigner, chứ
// không so sánh STORAGE_BACKEND với chuỗi "s3". Nếu LocalStore bỗng thoả interface này thì
// deployment dùng đĩa cục bộ sẽ phát ra URL mà không gì phục vụ được.
func TestLocalStore_IsNotPresigner(t *testing.T) {
	s, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	if _, ok := any(s).(storage.Presigner); ok {
		t.Error("LocalStore không được thoả Presigner")
	}
}

// TestS3Store_IsPresigner giữ chiều ngược lại: nếu chữ ký PresignGet lệch khỏi interface,
// assertion trong handler lặng lẽ trả false và mọi lượt tải quay về đi qua backend — chậm hơn
// nhưng vẫn đúng, nên không có gì hỏng để mà phát hiện.
func TestS3Store_IsPresigner(t *testing.T) {
	if _, ok := any(&storage.S3Store{}).(storage.Presigner); !ok {
		t.Error("S3Store phải thoả Presigner")
	}
}

// fakePresigner là một Store phát URL, để kiểm nhánh chuyển hướng mà không cần S3 thật.
type fakePresigner struct {
	storage.Store
	url    string
	signed []string
}

func (f *fakePresigner) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	f.signed = append(f.signed, key)
	return f.url + "?key=" + key, nil
}

// TestGetTaskAudio_RedirectsWhenStoreCanPresign kiểm tra redirect khi store hỗ trợ presign.
func TestGetTaskAudio_RedirectsWhenStoreCanPresign(t *testing.T) {
	prev := storage.Global
	defer func() { storage.Global = prev }()

	base, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	fake := &fakePresigner{Store: base, url: "https://example.invalid/obj"}
	storage.Global = fake

	const taskID = "presign-task-1"
	if err := base.Put(context.Background(), storage.AudioKey(taskID, "wav"), bytes.NewReader([]byte("RIFF....WAVE"))); err != nil {
		t.Fatalf("ghi đối tượng: %v", err)
	}

	task := state.GlobalTaskManager.GetOrCreate(taskID)
	task.SetOwner("owner-1")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")

	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID+"/audio", nil)
	req = asUser(req, &sqlc.User{ID: "owner-1", Username: "owner", IsApproved: true})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("kho phát được URL thì phải trả 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.invalid/obj") {
		t.Errorf("Location không trỏ tới kho: %q", loc)
	}
	// http.Redirect tự ghi một trang HTML ngắn kèm liên kết — hành vi chuẩn của net/http, và
	// client theo redirect không bao giờ đọc tới nó. Điều cần khẳng định là phản hồi KHÔNG
	// mang âm thanh, nên kiểm theo trần kích thước thay vì đòi body rỗng.
	if rec.Body.Len() > 512 {
		t.Errorf("phản hồi chuyển hướng mang %d byte — có vẻ vẫn kèm âm thanh", rec.Body.Len())
	}
	if strings.Contains(rec.Body.String(), "RIFF") {
		t.Error("phản hồi chuyển hướng không được mang bytes âm thanh")
	}
	if len(fake.signed) == 0 {
		t.Error("PresignGet lẽ ra phải được gọi")
	}
}

// TestGetTaskAudio_ChecksOwnershipBeforeSigning là nửa bảo mật của cùng thay đổi.
//
// URL đã ký không đi qua ownsTask ở lần dùng lại, nên phép kiểm quyền phải xảy ra TRƯỚC khi
// ký. Nếu thứ tự bị đảo, người không sở hữu task vẫn nhận được một URL dùng được — và 403 sau
// đó chẳng còn ý nghĩa gì.
func TestGetTaskAudio_ChecksOwnershipBeforeSigning(t *testing.T) {
	prev := storage.Global
	defer func() { storage.Global = prev }()

	base, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	fake := &fakePresigner{Store: base, url: "https://example.invalid/obj"}
	storage.Global = fake

	const taskID = "presign-task-2"
	if err := base.Put(context.Background(), storage.AudioKey(taskID, "wav"), bytes.NewReader([]byte("RIFF....WAVE"))); err != nil {
		t.Fatalf("ghi đối tượng: %v", err)
	}

	task := state.GlobalTaskManager.GetOrCreate(taskID)
	task.SetOwner("owner-2")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")

	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID+"/audio", nil)
	req = asUser(req, &sqlc.User{ID: "intruder-2", Username: "intruder", IsApproved: true})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("người không sở hữu phải nhận 403, got %d", rec.Code)
	}
	if len(fake.signed) != 0 {
		t.Errorf("không được ký URL cho người không sở hữu, đã ký %v", fake.signed)
	}
}
