package advisor

import (
	"context"
	"database/sql"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListThreads(ctx context.Context, userID string) ([]Thread, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, user_id::text, title, period_month, created_at, updated_at
FROM advisor_threads
WHERE user_id = $1
ORDER BY updated_at DESC
LIMIT 50
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := []Thread{}
	for rows.Next() {
		var thread Thread
		if err := rows.Scan(&thread.ID, &thread.UserID, &thread.Title, &thread.PeriodMonth, &thread.CreatedAt, &thread.UpdatedAt); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func (r PostgresRepository) CreateThread(ctx context.Context, userID, title, periodMonth string) (Thread, error) {
	var thread Thread
	err := r.DB.QueryRowContext(ctx, `
INSERT INTO advisor_threads (user_id, title, period_month)
VALUES ($1, $2, $3)
RETURNING id::text, user_id::text, title, period_month, created_at, updated_at
`, userID, title, periodMonth).Scan(&thread.ID, &thread.UserID, &thread.Title, &thread.PeriodMonth, &thread.CreatedAt, &thread.UpdatedAt)
	return thread, err
}

func (r PostgresRepository) GetThread(ctx context.Context, userID, threadID string) (Thread, error) {
	var thread Thread
	err := r.DB.QueryRowContext(ctx, `
SELECT id::text, user_id::text, title, period_month, created_at, updated_at
FROM advisor_threads
WHERE user_id = $1 AND id = $2
`, userID, threadID).Scan(&thread.ID, &thread.UserID, &thread.Title, &thread.PeriodMonth, &thread.CreatedAt, &thread.UpdatedAt)
	return thread, err
}

func (r PostgresRepository) TouchThread(ctx context.Context, userID, threadID string) error {
	_, err := r.DB.ExecContext(ctx, `
UPDATE advisor_threads
SET updated_at = NOW()
WHERE user_id = $1 AND id = $2
`, userID, threadID)
	return err
}

func (r PostgresRepository) ListMessages(ctx context.Context, userID, threadID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, thread_id::text, user_id::text, role, content, model, created_at
FROM (
	SELECT *
	FROM advisor_messages
	WHERE user_id = $1 AND thread_id = $2
	ORDER BY created_at DESC
	LIMIT $3
) messages
ORDER BY created_at ASC
`, userID, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.ThreadID, &message.UserID, &message.Role, &message.Content, &message.Model, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r PostgresRepository) CreateMessage(ctx context.Context, userID, threadID, role, content, model string) (Message, error) {
	var message Message
	err := r.DB.QueryRowContext(ctx, `
INSERT INTO advisor_messages (user_id, thread_id, role, content, model)
VALUES ($1, $2, $3, $4, $5)
RETURNING id::text, thread_id::text, user_id::text, role, content, model, created_at
`, userID, threadID, role, content, model).Scan(
		&message.ID,
		&message.ThreadID,
		&message.UserID,
		&message.Role,
		&message.Content,
		&message.Model,
		&message.CreatedAt,
	)
	return message, err
}
