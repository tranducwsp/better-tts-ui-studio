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
	JTI      string `json:"jti,omitempty"` // chỉ refresh token mang; trỏ tới dòng auth_sessions
	jwt.RegisteredClaims
}

const (
	TokenUseAccess  = "access"
	TokenUseRefresh = "refresh"
)

func HashPassword(password string) (string, error) {
	// bcrypt chỉ dùng 72 byte đầu của password. GenerateFromPassword trả lỗi
	// ErrPasswordTooLong khi vượt trần — lỗi đúng, nhưng thứ đến tay người dùng là 500
	// nếu caller không kiểm. Caller (Register) phải chặn từ ValidatePassword trước.
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// minPasswordChars / maxPasswordBytes là ranh giới mật khẩu chấp nhận.
//
// 72 byte là trần thật của bcrypt: vượt qua đó GenerateFromPassword lỗi. 8 ký tự là
// mức sàn thấp nhất vẫn chống đoán qua mạng; dưới đó tài khoản quá dễ bẻ khi bảng hash
// lọt ra ngoài.
// MaxPasswordBytes là trần byte mà bcrypt chấp nhận; handler đăng nhập so sánh trước khi
// băm để trả 401 chung thay vì để thư viện báo lỗi riêng.
const (
	minPasswordChars = 8
	MaxPasswordBytes = 72
)

// ValidatePassword kiểm mật khẩu trước khi băm. Trả về lỗi có thông điệp người dùng
// đọc được thay vì để bcrypt fail vào giữa luồng đăng ký.
func ValidatePassword(password string) error {
	if len([]rune(password)) < minPasswordChars {
		return fmt.Errorf("mật khẩu phải có ít nhất %d ký tự", minPasswordChars)
	}
	if len([]byte(password)) > MaxPasswordBytes {
		return fmt.Errorf("mật khẩu không được vượt quá %d byte", MaxPasswordBytes)
	}
	return nil
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateAccessToken tạo access token (stateless, ngắn hạn, không cần jti).
func CreateAccessToken(username, role, secretKey string, expireMinutes int) (string, error) {
	expiresAt := time.Now().Add(time.Duration(expireMinutes) * time.Minute)
	return buildToken(username, role, "", TokenUseAccess, secretKey, expiresAt)
}

// CreateRefreshToken tạo refresh token gắn jti, hết hạn đúng lúc dòng auth_sessions hết hạn.
//
// Nhận thời điểm hết hạn cụ thể thay vì số phút: rotation phải tái phát hành với cùng mốc
// TUYỆT ĐỐI của phiên (không bị kéo dài), nên JWT exp và expires_at trong DB phải trùng nhau.
func CreateRefreshToken(username, role, jti, secretKey string, expiresAt time.Time) (string, error) {
	return buildToken(username, role, jti, TokenUseRefresh, secretKey, expiresAt)
}

// buildToken tạo JWT HS256 với các claim phân biệt mục đích sử dụng.
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
