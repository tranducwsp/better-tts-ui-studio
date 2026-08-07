package tests

import (
	"testing"

	"core-backend/security"
)

func TestRefreshToken_IsNotAnAccessToken(t *testing.T) {
	refresh, err := security.CreateToken("user-1", "user", security.TokenUseRefresh, "test-secret", 10)
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
}

func TestRefreshToken_Expires(t *testing.T) {
	refresh, err := security.CreateToken("user-1", "user", security.TokenUseRefresh, "test-secret", 1)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := security.ValidateRefreshToken(refresh, "wrong-secret"); err == nil {
		t.Fatal("refresh token được chấp nhận bằng secret khác")
	}
}
