package db

import (
	"context"
	"time"
)

type Item struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	SectionID *int      `json:"section_id"`
	Checked   bool      `json:"checked"`
	AddedAt   time.Time `json:"added_at"`
}

func (q *Queries) AddItem(ctx context.Context, text string, sectionID *int, userID []byte) (Item, error) {
	var item Item
	err := q.pool.QueryRow(ctx,
		`INSERT INTO items(text, section_id, added_by)
		 VALUES($1, $2, $3)
		 RETURNING id, text, section_id, checked, added_at`,
		text, sectionID, userID,
	).Scan(&item.ID, &item.Text, &item.SectionID, &item.Checked, &item.AddedAt)
	return item, err
}

func (q *Queries) ListUncheckedItems(ctx context.Context) ([]Item, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, text, section_id, checked, added_at
		 FROM items WHERE checked = false ORDER BY added_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Text, &item.SectionID, &item.Checked, &item.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Queries) CheckItem(ctx context.Context, id int64, checked bool, userID []byte) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE items
		 SET checked    = $2,
		     checked_at = CASE WHEN $2 THEN now() ELSE NULL END,
		     checked_by = CASE WHEN $2 THEN $3    ELSE NULL END
		 WHERE id = $1`,
		id, checked, userID,
	)
	return err
}

func (q *Queries) DeleteItem(ctx context.Context, id int64) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM items WHERE id = $1`, id)
	return err
}

func (q *Queries) ClearCheckedItems(ctx context.Context) ([]int64, error) {
	rows, err := q.pool.Query(ctx, `DELETE FROM items WHERE checked = true RETURNING id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) UpdateItemSection(ctx context.Context, id int64, sectionID int) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE items SET section_id = $2 WHERE id = $1`,
		id, sectionID,
	)
	return err
}
