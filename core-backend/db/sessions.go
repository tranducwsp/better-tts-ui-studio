package db

import (
	"context"
	"time"

	"core-backend/db/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateAuthSession ghi một refresh token mới (jti) thuộc về một phiên đăng nhập (family).
func CreateAuthSession(ctx context.Context, jti, familyID, userID string, expiresAt time.Time, ua, ip string) error {
	_, err := Queries.CreateAuthSession(ctx, sqlc.CreateAuthSessionParams{
		ID:              jti,
		SessionFamilyID: familyID,
		UserID:          userID,
		ExpiresAt:       pgtype.Timestamptz{Time: expiresAt, Valid: true},
		UserAgent:       pgtype.Text{String: ua, Valid: ua != ""},
		Ip:              pgtype.Text{String: ip, Valid: ip != ""},
	})
	return err
}

// GetAuthSession trả về dòng session của một refresh token. Lỗi là ErrNoRows khi jti chưa
// từng tồn tại (token giả hoặc đã bị xoá) — handler phải từ chối 401.
func GetAuthSession(ctx context.Context, jti string) (*sqlc.AuthSession, error) {
	s, err := Queries.GetAuthSession(ctx, jti)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// RotateRefreshSession thay token cũ bằng token mới trong cùng family, trong MỘT transaction.
//
// Hai lệnh ghi phải đi cùng nhau: tạo token mới rồi mới đánh dấu token cũ, để nếu bước nào
// hỏng thì family vẫn giữ đúng một token sống — không để xảy ra trạng thái hai token cùng hợp
// lệ hoặc token cũ chết mà token mới không ra đời.
func RotateRefreshSession(ctx context.Context, oldJTI, familyID, userID, newJTI string, expiresAt time.Time, ua, ip string) error {
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	q := sqlc.New(tx)
	if _, err := q.CreateAuthSession(ctx, sqlc.CreateAuthSessionParams{
		ID:              newJTI,
		SessionFamilyID: familyID,
		UserID:          userID,
		ExpiresAt:       pgtype.Timestamptz{Time: expiresAt, Valid: true},
		UserAgent:       pgtype.Text{String: ua, Valid: ua != ""},
		Ip:              pgtype.Text{String: ip, Valid: ip != ""},
	}); err != nil {
		return err
	}
	if _, err := q.RotateAuthSession(ctx, sqlc.RotateAuthSessionParams{
		ID:         oldJTI,
		ReplacedBy: pgtype.Text{String: newJTI, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RevokeAuthSession thu hồi token đang dùng (logout của thiết bị đó).
func RevokeAuthSession(ctx context.Context, jti string) (int64, error) {
	return Queries.RevokeAuthSession(ctx, jti)
}

// DeleteExpiredAuthSessions xoá mọi phiên đã quá hạn. Gọi opportunistic tại login.
func DeleteExpiredAuthSessions(ctx context.Context) (int64, error) {
	return Queries.DeleteExpiredAuthSessions(ctx)
}
