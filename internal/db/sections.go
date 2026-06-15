package db

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Section struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

func (q *Queries) ListSections(ctx context.Context) ([]Section, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, name, sort_order FROM sections ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Section
	for rows.Next() {
		var s Section
		if err := rows.Scan(&s.ID, &s.Name, &s.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (q *Queries) AddSection(ctx context.Context, name string) (Section, error) {
	name = strings.TrimSpace(name)
	var s Section
	err := q.pool.QueryRow(ctx,
		`INSERT INTO sections(name, sort_order)
		 SELECT $1, COALESCE(MAX(sort_order), 0) + 1 FROM sections
		 RETURNING id, name, sort_order`,
		name,
	).Scan(&s.ID, &s.Name, &s.SortOrder)
	return s, err
}

func (q *Queries) LookupItemSection(ctx context.Context, text string) (int, bool, error) {
	var sectionID int
	err := q.pool.QueryRow(ctx,
		`SELECT section_id FROM item_sections WHERE item_text = $1`,
		strings.ToLower(strings.TrimSpace(text)),
	).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return sectionID, err == nil, err
}

func (q *Queries) UpsertItemSection(ctx context.Context, text string, sectionID int, source string) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO item_sections(item_text, section_id, source, updated_at)
		 VALUES($1, $2, $3, now())
		 ON CONFLICT (item_text) DO UPDATE
		   SET section_id = EXCLUDED.section_id,
		       source     = EXCLUDED.source,
		       updated_at = now()
		   WHERE item_sections.source <> 'manual' OR EXCLUDED.source = 'manual'`,
		strings.ToLower(strings.TrimSpace(text)), sectionID, source,
	)
	return err
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
