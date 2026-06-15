package db

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Invitation struct {
	Token     string
	Email     string
	ExpiresAt time.Time
}

func encodeBase64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func (q *Queries) CreateInvitation(ctx context.Context, email string, invitedBy []byte) (string, error) {
	token := encodeBase64URL(randomBytes(32))
	_, err := q.pool.Exec(ctx,
		`INSERT INTO invitations(token, email, invited_by, expires_at)
		 VALUES($1, $2, $3, now() + interval '48 hours')`,
		token, email, invitedBy,
	)
	return token, err
}

func (q *Queries) GetValidInvitation(ctx context.Context, token string) (*Invitation, error) {
	var inv Invitation
	err := q.pool.QueryRow(ctx,
		`SELECT token, email, expires_at FROM invitations
		 WHERE token = $1 AND used_at IS NULL AND expires_at > now()`,
		token,
	).Scan(&inv.Token, &inv.Email, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &inv, err
}

func (q *Queries) UseInvitation(ctx context.Context, token string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE invitations SET used_at = now() WHERE token = $1`, token)
	return err
}
