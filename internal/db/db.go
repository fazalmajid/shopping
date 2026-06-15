package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Queries struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func (q *Queries) CleanupSessions(ctx context.Context) {
	q.pool.Exec(ctx, `DELETE FROM webauthn_sessions WHERE expires_at < now()`)
	q.pool.Exec(ctx, `DELETE FROM app_sessions WHERE expires_at < now()`)
}
