package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

// SystemUserID is the all-zero sentinel stored by migration 00003. It is used
// as invited_by for bootstrap invitations and is excluded from user counts.
var SystemUserID = make([]byte, 16)

// User implements webauthn.User.
type User struct {
	ID          []byte
	Email       string
	DisplayName string
	Credentials []webauthn.Credential
}

func (u *User) WebAuthnID() []byte                         { return u.ID }
func (u *User) WebAuthnName() string                       { return u.Email }
func (u *User) WebAuthnDisplayName() string                { return u.DisplayName }
func (u *User) WebAuthnIcon() string                       { return "" }
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// HasUsers reports whether any real (non-system) users exist.
func (q *Queries) HasUsers(ctx context.Context) (bool, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE id != $1`, SystemUserID,
	).Scan(&n)
	return n > 0, err
}

func (q *Queries) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := q.pool.QueryRow(ctx,
		`SELECT id, email, display_name FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Credentials, err = q.getCredentials(ctx, u.ID)
	return &u, err
}

func (q *Queries) GetUserByID(ctx context.Context, id []byte) (*User, error) {
	var u User
	err := q.pool.QueryRow(ctx,
		`SELECT id, email, display_name FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Credentials, err = q.getCredentials(ctx, u.ID)
	return &u, err
}

func (q *Queries) CreateUser(ctx context.Context, email, displayName string) (*User, error) {
	id := randomBytes(16)
	_, err := q.pool.Exec(ctx,
		`INSERT INTO users(id, email, display_name) VALUES($1, $2, $3)`,
		id, email, displayName,
	)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Email: email, DisplayName: displayName}, nil
}

func (q *Queries) getCredentials(ctx context.Context, userID []byte) ([]webauthn.Credential, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT data FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []webauthn.Credential
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(raw), &cred); err != nil {
			return nil, err
		}
		creds = append(creds, cred)
	}
	return creds, rows.Err()
}

func (q *Queries) StoreCredential(ctx context.Context, userID []byte, cred webauthn.Credential) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	// Credential ID encoded as base64url for the TEXT primary key.
	credID := encodeBase64URL(cred.ID)
	_, err = q.pool.Exec(ctx,
		`INSERT INTO webauthn_credentials(id, user_id, data)
		 VALUES($1, $2, $3)
		 ON CONFLICT(id) DO UPDATE SET data = EXCLUDED.data, last_used_at = now()`,
		credID, userID, string(data),
	)
	return err
}
