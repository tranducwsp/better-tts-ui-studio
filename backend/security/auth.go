package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type JWTClaims struct {
	Username string `json:"sub"`
	Role     string `json:"role"`
	TokenUse string `json:"token_use,omitempty"`
	JTI      string `json:"jti,omitempty"` // only refresh tokens carry this; points to the auth_sessions row
	jwt.RegisteredClaims
}

const (
	TokenUseAccess  = "access"
	TokenUseRefresh = "refresh"
)

func HashPassword(password string) (string, error) {
	// bcrypt only uses the first 72 bytes of the password. GenerateFromPassword returns
	// ErrPasswordTooLong when the limit is exceeded — a correct error, but what reaches the
	// user is a 500 if the caller doesn't check. The caller (Register) must block via
	// ValidatePassword first.
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// minPasswordChars / maxPasswordBytes are the boundaries for accepted passwords.
//
// 72 bytes is bcrypt's real ceiling: exceeding it causes GenerateFromPassword to error.
// 8 characters is the lowest floor that still resists online guessing; below that an
// account is too easy to crack if the hash table leaks.
// MaxPasswordBytes is the byte ceiling bcrypt accepts; the login handler compares against it
// before hashing to return a generic 401 rather than letting the library report a distinct error.
const (
	minPasswordChars = 8
	MaxPasswordBytes = 72
)

// ValidatePassword checks the password before hashing. Returns a user-readable error message
// instead of letting bcrypt fail mid-registration.
func ValidatePassword(password string) error {
	if len([]rune(password)) < minPasswordChars {
		return fmt.Errorf("password must be at least %d characters", minPasswordChars)
	}
	if len([]byte(password)) > MaxPasswordBytes {
		return fmt.Errorf("password must not exceed %d bytes", MaxPasswordBytes)
	}
	return nil
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateAccessToken creates an access token (stateless, short-lived, no jti needed).
func CreateAccessToken(username, role, secretKey string, expireMinutes int) (string, error) {
	expiresAt := time.Now().Add(time.Duration(expireMinutes) * time.Minute)
	return buildToken(username, role, "", TokenUseAccess, secretKey, expiresAt)
}

// CreateRefreshToken creates a refresh token with a jti, expiring at the same moment as the
// auth_sessions row.
//
// Accepts a specific expiry time rather than a number of minutes: rotation must re-issue with
// the same ABSOLUTE session deadline (not extend it), so the JWT exp and the expires_at in the
// DB must match.
func CreateRefreshToken(username, role, jti, secretKey string, expiresAt time.Time) (string, error) {
	return buildToken(username, role, jti, TokenUseRefresh, secretKey, expiresAt)
}

// buildToken creates a HS256 JWT with claims distinguishing the token's purpose.
func buildToken(username, role, jti, tokenUse, secretKey string, expiresAt time.Time) (string, error) {
	claims := &JWTClaims{
		Username: username,
		Role:     role,
		TokenUse: tokenUse,
		JTI:      jti,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

func ValidateAccessToken(tokenString, secretKey string) (*JWTClaims, error) {
	claims, err := ValidateToken(tokenString, secretKey)
	if err != nil {
		return nil, err
	}
	if claims.TokenUse == TokenUseRefresh {
		return nil, errors.New("not an access token")
	}
	return claims, nil
}

func ValidateRefreshToken(tokenString, secretKey string) (*JWTClaims, error) {
	claims, err := ValidateToken(tokenString, secretKey)
	if err != nil {
		return nil, err
	}
	if claims.TokenUse != TokenUseRefresh {
		return nil, errors.New("not a refresh token")
	}
	return claims, nil
}

func ValidateToken(tokenString, secretKey string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
