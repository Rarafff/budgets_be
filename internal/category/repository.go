package category

import (
	"context"
	"database/sql"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) List(ctx context.Context, userID string) ([]Category, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, user_id::text, name, type, created_at
FROM categories
WHERE user_id = $1
ORDER BY type ASC, LOWER(name) ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Category{}
	for rows.Next() {
		var item Category
		if err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Type, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) Create(ctx context.Context, userID string, req SaveRequest) (Category, error) {
	var item Category
	err := r.DB.QueryRowContext(ctx, `
INSERT INTO categories (user_id, name, type)
VALUES ($1, $2, $3)
RETURNING id::text, user_id::text, name, type, created_at
`, userID, req.Name, req.Type).Scan(&item.ID, &item.UserID, &item.Name, &item.Type, &item.CreatedAt)
	return item, err
}

func (r PostgresRepository) Delete(ctx context.Context, userID, categoryID string) error {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM categories WHERE user_id = $1 AND id = $2`, userID, categoryID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
