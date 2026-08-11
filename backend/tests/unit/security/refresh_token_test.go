package security_test

import (
	"testing"
	"time"

	"backend/security"
)

func TestRefreshToken_IsNotAnAccessToken(t *testing.T) {
	refresh, err := security.CreateRefreshToken("user-1", "user", "jti-1", "test-secret", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	if _, err := security.ValidateAccessToken(refresh, "test-secret"); err == nil {
		t.Fatal("refresh token được chấp nhận như access token")
	}
	claims, err := security.ValidateRefreshToken(refresh, "test-secret")
	if err != nil {
		t.Fatalf("refresh token hợp lệ bị từ chối: %v", err)
	}
	if claims.Username != "user-1" || claims.TokenUse != security.TokenUseRefresh {
		t.Fatalf("claims refresh không đúng: %+v", claims)
	}
	if claims.JTI != "jti-1" {
		t.Fatalf("claims thiếu jti: %+v", claims)
	}
}

func TestAccessToken_IsNotARefreshToken(t *testing.T) {
	access, err := security.CreateAccessToken("user-1", "user", "test-secret", 10)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	if _, err := security.ValidateRefreshToken(access, "test-secret"); err == nil {
		t.Fatal("access token được chấp nhận như refresh token")
	}
	claims, err := security.ValidateAccessToken(access, "test-secret")
	if err != nil {
		t.Fatalf("access token hợp lệ bị từ chối: %v", err)
	}
	if claims.Username != "user-1" || claims.TokenUse != security.TokenUseAccess {
		t.Fatalf("claims access không đúng: %+v", claims)
	}
	// Access token không bao giờ mang jti.
	if claims.JTI != "" {
		t.Fatalf("access token mang jti: %+v", claims)
	}
}

func TestRefreshToken_Expires(t *testing.T) {
	refresh, err := security.CreateRefreshToken("user-1", "user", "jti-2", "test-secret", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := security.ValidateRefreshToken(refresh, "wrong-secret"); err == nil {
		t.Fatal("refresh token được chấp nhận bằng secret khác")
	}
}

// TestRefreshToken_ExpMatchesSession đảm bảo exp của JWT khớp đúng mốc hết hạn phiên: rotation
// dựa vào sự trùng khớp này (token mới ra đời với cùng exp, không kéo dài phiên).
func TestRefreshToken_ExpMatchesSession(t *testing.T) {
	expiresAt := time.Now().Add(90 * time.Minute).UTC().Truncate(time.Second) // JWT serialise theo giây
	refresh, err := security.CreateRefreshToken("user-1", "user", "jti-3", "test-secret", expiresAt)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	claims, err := security.ValidateRefreshToken(refresh, "test-secret")
	if err != nil {
		t.Fatalf("refresh token hợp lệ bị từ chối: %v", err)
	}
	if !claims.ExpiresAt.Time.Equal(expiresAt) {
		t.Fatalf("exp trong token=%v, muốn=%v", claims.ExpiresAt.Time, expiresAt)
	}
}
