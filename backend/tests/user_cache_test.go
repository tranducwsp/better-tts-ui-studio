package tests

import (
	"testing"
	"time"

	"backend/db/sqlc"
	"backend/middleware"
)

// Cache tồn tại để mỗi request đã đăng nhập không phải hỏi PostgreSQL cùng một câu, nên
// điều đáng kiểm là nó thực sự trả lại bản đã nhớ, thực sự hết hạn, và thực sự bị xoá khi
// tài khoản được duyệt — nếu mất điểm cuối, người vừa được duyệt vẫn bị chặn mà không rõ lý do.
func TestUserCacheServesThenExpires(t *testing.T) {
	middleware.ConfigureUserCache(50 * time.Millisecond)
	defer middleware.ConfigureUserCache(0)

	user := sqlc.User{ID: "u1", Username: "alice", Role: "user", IsApproved: true}
	middleware.PutUserForTest("alice", user)

	got, ok := middleware.GetUserForTest("alice")
	if !ok {
		t.Fatal("vừa ghi vào cache mà đọc không ra")
	}
	if got.ID != "u1" || !got.IsApproved {
		t.Fatalf("bản ghi trả về không khớp: %+v", got)
	}

	time.Sleep(70 * time.Millisecond)
	if _, ok := middleware.GetUserForTest("alice"); ok {
		t.Error("entry đã quá TTL mà vẫn được trả về")
	}
}

func TestUserCacheInvalidateDropsEntry(t *testing.T) {
	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("bob", sqlc.User{ID: "u2", Username: "bob"})
	if _, ok := middleware.GetUserForTest("bob"); !ok {
		t.Fatal("chuẩn bị dữ liệu thất bại")
	}

	middleware.InvalidateUser("bob")
	if _, ok := middleware.GetUserForTest("bob"); ok {
		t.Error("Invalidate không xoá được entry, nên tài khoản vừa duyệt vẫn đọc trạng thái cũ")
	}
}

// TTL 0 nghĩa là tắt hẳn: đặt được như vậy thì mới có đường quay về hành vi truy vấn từng
// request nếu cache gây rắc rối trong triển khai thật.
func TestUserCacheDisabledStoresNothing(t *testing.T) {
	middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("carol", sqlc.User{ID: "u3", Username: "carol"})
	if _, ok := middleware.GetUserForTest("carol"); ok {
		t.Error("cache đang tắt mà vẫn ghi nhớ")
	}
}
