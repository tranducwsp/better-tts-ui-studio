package middleware_test

import (
	"testing"
	"time"

	"backend/db/sqlc"
	"backend/middleware"
)

// The cache exists so each authenticated request does not need to ask PostgreSQL the same
// question, so what matters is that it truly returns the cached record, truly expires, and
// truly gets evicted when an account is approved — if the last part is missing, a newly
// approved user is still blocked with no clear reason.

func TestUserCacheServesThenExpires(t *testing.T) {
	middleware.ConfigureUserCache(50 * time.Millisecond)
	defer middleware.ConfigureUserCache(0)

	user := sqlc.User{ID: "u1", Username: "alice", Role: "user", IsApproved: true}
	middleware.PutUserForTest("alice", user)

	got, ok := middleware.GetUserForTest("alice")
	if !ok {
		t.Fatal("just wrote to cache but cannot read it back")
	}
	if got.ID != "u1" || !got.IsApproved {
		t.Fatalf("returned record does not match: %+v", got)
	}

	time.Sleep(70 * time.Millisecond)
	if _, ok := middleware.GetUserForTest("alice"); ok {
		t.Error("entry past TTL is still being returned")
	}
}

func TestUserCacheInvalidateDropsEntry(t *testing.T) {
	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("bob", sqlc.User{ID: "u2", Username: "bob"})
	if _, ok := middleware.GetUserForTest("bob"); !ok {
		t.Fatal("data preparation failed")
	}

	middleware.InvalidateUser("bob")
	if _, ok := middleware.GetUserForTest("bob"); ok {
		t.Error("Invalidate failed to remove the entry, so a just-approved account still reads the old state")
	}
}

// TTL 0 means fully disabled: being able to set it this way provides a path back to
// per-request query behavior if the cache causes trouble in production.
func TestUserCacheDisabledStoresNothing(t *testing.T) {
	middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("carol", sqlc.User{ID: "u3", Username: "carol"})
	if _, ok := middleware.GetUserForTest("carol"); ok {
		t.Error("cache is disabled but still storing entries")
	}
}
