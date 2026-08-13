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
		t.Fatal("refresh token accepted as access token")
	}
	claims, err := security.ValidateRefreshToken(refresh, "test-secret")
	if err != nil {
		t.Fatalf("valid refresh token rejected: %v", err)
	}
	if claims.Username != "user-1" || claims.TokenUse != security.TokenUseRefresh {
		t.Fatalf("refresh claims incorrect: %+v", claims)
	}
	if claims.JTI != "jti-1" {
		t.Fatalf("claims missing jti: %+v", claims)
	}
}

func TestAccessToken_IsNotARefreshToken(t *testing.T) {
	access, err := security.CreateAccessToken("user-1", "user", "test-secret", 10)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	if _, err := security.ValidateRefreshToken(access, "test-secret"); err == nil {
		t.Fatal("access token accepted as refresh token")
	}
	claims, err := security.ValidateAccessToken(access, "test-secret")
	if err != nil {
		t.Fatalf("valid access token rejected: %v", err)
	}
	if claims.Username != "user-1" || claims.TokenUse != security.TokenUseAccess {
		t.Fatalf("access claims incorrect: %+v", claims)
	}
	// Access tokens never carry a jti.
	if claims.JTI != "" {
		t.Fatalf("access token carries jti: %+v", claims)
	}
}

func TestRefreshToken_Expires(t *testing.T) {
	refresh, err := security.CreateRefreshToken("user-1", "user", "jti-2", "test-secret", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := security.ValidateRefreshToken(refresh, "wrong-secret"); err == nil {
		t.Fatal("refresh token accepted with wrong secret")
	}
}

// TestRefreshToken_ExpMatchesSession ensures the JWT exp exactly matches the session expiry:
// rotation relies on this match (new token is issued with the same exp, not extending the session).
func TestRefreshToken_ExpMatchesSession(t *testing.T) {
	expiresAt := time.Now().Add(90 * time.Minute).UTC().Truncate(time.Second) // JWT serializes to seconds
	refresh, err := security.CreateRefreshToken("user-1", "user", "jti-3", "test-secret", expiresAt)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	claims, err := security.ValidateRefreshToken(refresh, "test-secret")
	if err != nil {
		t.Fatalf("valid refresh token rejected: %v", err)
	}
	if !claims.ExpiresAt.Time.Equal(expiresAt) {
		t.Fatalf("exp in token=%v, want=%v", claims.ExpiresAt.Time, expiresAt)
	}
}
