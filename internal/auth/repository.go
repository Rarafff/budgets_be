package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) CreateUser(ctx context.Context, name, email, phoneNumber, passwordHash string) (User, error) {
	var user User
	err := r.DB.QueryRowContext(ctx, `
INSERT INTO users (name, email, phone_number, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING id::text, name, email, phone_number, created_at, updated_at
`, name, email, phoneNumber, passwordHash).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PhoneNumber,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if isUniqueViolation(err) {
		return User{}, errors.New("email is already registered")
	}
	return user, err
}

func (r PostgresRepository) FindUserByEmail(ctx context.Context, email string) (UserWithPassword, error) {
	var user UserWithPassword
	err := r.DB.QueryRowContext(ctx, `
SELECT id::text, name, email, phone_number, password_hash, created_at, updated_at
FROM users
WHERE email = $1
`, strings.ToLower(strings.TrimSpace(email))).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PhoneNumber,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func (r PostgresRepository) FindUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := r.DB.QueryRowContext(ctx, `
SELECT id::text, name, email, phone_number, created_at, updated_at
FROM users
WHERE id = $1
`, id).Scan(&user.ID, &user.Name, &user.Email, &user.PhoneNumber, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (r PostgresRepository) UpdateProfile(ctx context.Context, userID, name, phoneNumber string) (User, error) {
	var user User
	err := r.DB.QueryRowContext(ctx, `
UPDATE users
SET name = $2,
	phone_number = $3,
	updated_at = NOW()
WHERE id = $1
RETURNING id::text, name, email, phone_number, created_at, updated_at
`, userID, name, phoneNumber).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PhoneNumber,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func (r PostgresRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := r.DB.ExecContext(ctx, `
UPDATE users
SET password_hash = $2,
	updated_at = NOW()
WHERE id = $1
`, userID, passwordHash)
	return err
}

func (r PostgresRepository) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.DB.ExecContext(ctx, `
INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
`, userID, tokenHash, expiresAt)
	return err
}

func (r PostgresRepository) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.DB.QueryRowContext(ctx, `
SELECT user_id::text
FROM password_reset_tokens
WHERE token_hash = $1
	AND used_at IS NULL
	AND expires_at > NOW()
`, tokenHash).Scan(&userID)
	return userID, err
}

func (r PostgresRepository) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) error {
	_, err := r.DB.ExecContext(ctx, `
UPDATE password_reset_tokens
SET used_at = NOW()
WHERE token_hash = $1
`, tokenHash)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
