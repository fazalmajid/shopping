package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// WebAuthn challenge sessions (5-minute TTL, single-use).

func (q *Queries) CreateWebAuthnSession(ctx context.Context, data string, expiresAt time.Time) (string, error) {
	id := randomHex(32)
	_, err := q.pool.Exec(ctx,
		`INSERT INTO webauthn_sessions(id, data, expires_at) VALUES($1, $2, $3)`,
		id, data, expiresAt,
	)
	return id, err
}

// GetAndDeleteWebAuthnSession fetches and atomically deletes the session.
func (q *Queries) GetAndDeleteWebAuthnSession(ctx context.Context, id string) (string, error) {
	var data string
	err := q.pool.QueryRow(ctx,
		`DELETE FROM webauthn_sessions WHERE id = $1 AND expires_at > now() RETURNING data`,
		id,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("session not found or expired")
	}
	return data, err
}

// Application login sessions (30-day TTL).

func (q *Queries) CreateAppSession(ctx context.Context, userID []byte, expiresAt time.Time) (string, error) {
	token := randomHex(32)
	_, err := q.pool.Exec(ctx,
		`INSERT INTO app_sessions(id, user_id, expires_at) VALUES($1, $2, $3)`,
		token, userID, expiresAt,
	)
	return token, err
}

func (q *Queries) GetSessionUser(ctx context.Context, token string) (*User, error) {
	var u User
	err := q.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name
		 FROM app_sessions s JOIN users u ON s.user_id = u.id
		 WHERE s.id = $1 AND s.expires_at > now()`,
		token,
	).Scan(&u.ID, &u.Email, &u.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) DeleteAppSession(ctx context.Context, token string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM app_sessions WHERE id = $1`, token)
	return err
}
