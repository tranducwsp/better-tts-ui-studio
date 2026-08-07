package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type JWTClaims struct {
	Username string `json:"sub"`
	Role     string `json:"role"`
	TokenUse string `json:"token_use,omitempty"`
	jwt.RegisteredClaims
}

const (
	TokenUseAccess  = "access"
	TokenUseRefresh = "refresh"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateAccessToken giữ API cũ và tạo access token có token_use=access.
func CreateAccessToken(username, role, secretKey string, expireMinutes int) (string, error) {
	return CreateToken(username, role, TokenUseAccess, secretKey, expireMinutes)
}

// CreateToken tạo token phân biệt rõ mục đích sử dụng.
func CreateToken(username, role, tokenUse, secretKey string, expireMinutes int) (string, error) {
	expirationTime := time.Now().Add(time.Duration(expireMinutes) * time.Minute)
	claims := &JWTClaims{
		Username: username,
		Role:     role,
		TokenUse: tokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
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
