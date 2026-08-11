package db

import (
	"context"
	"time"

	"backend/db/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateAuthSession writes a new refresh token (jti) belonging to a login session (family).
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

// GetAuthSession returns the session row for a refresh token. The error is ErrNoRows when the
// jti never existed (fake token or already deleted) — the handler must reject with 401.
func GetAuthSession(ctx context.Context, jti string) (*sqlc.AuthSession, error) {
	s, err := Queries.GetAuthSession(ctx, jti)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// RotateRefreshSession replaces the old token with a new one in the same family, in ONE
// transaction.
//
// The two writes must go together: create the new token first, then mark the old one, so if
// either step fails the family still holds exactly one live token — no state where two tokens
// are both valid, or the old token is dead but the new one was never born.
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

// RevokeAuthSession revokes the current token (logout from that device).
func RevokeAuthSession(ctx context.Context, jti string) (int64, error) {
	return Queries.RevokeAuthSession(ctx, jti)
}

// DeleteExpiredAuthSessions deletes all expired sessions. Called opportunistically at login.
func DeleteExpiredAuthSessions(ctx context.Context) (int64, error) {
	return Queries.DeleteExpiredAuthSessions(ctx)
}