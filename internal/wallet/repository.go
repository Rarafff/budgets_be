package wallet

import (
	"context"
	"database/sql"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListWallets(ctx context.Context, userID string) ([]Wallet, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, user_id::text, name, type, currency, balance, account_number, credit_limit, due_day, created_at, updated_at
FROM wallets
WHERE user_id = $1
ORDER BY created_at ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	wallets := []Wallet{}
	for rows.Next() {
		wallet, err := scanWallet(rows)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, wallet)
	}

	return wallets, rows.Err()
}

func (r PostgresRepository) CreateWallet(ctx context.Context, userID string, req SaveWalletRequest) (Wallet, error) {
	return scanWalletRow(r.DB.QueryRowContext(ctx, `
INSERT INTO wallets (user_id, name, type, currency, balance, account_number, credit_limit, due_day)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id::text, user_id::text, name, type, currency, balance, account_number, credit_limit, due_day, created_at, updated_at
`, userID, req.Name, req.Type, req.Currency, req.Balance, req.AccountNumber, req.CreditLimit, req.DueDay))
}

func (r PostgresRepository) UpdateWallet(ctx context.Context, userID, walletID string, req SaveWalletRequest) (Wallet, error) {
	return scanWalletRow(r.DB.QueryRowContext(ctx, `
UPDATE wallets
SET name = $3,
	type = $4,
	currency = $5,
	balance = $6,
	account_number = $7,
	credit_limit = $8,
	due_day = $9,
	updated_at = NOW()
WHERE user_id = $1 AND id = $2
RETURNING id::text, user_id::text, name, type, currency, balance, account_number, credit_limit, due_day, created_at, updated_at
`, userID, walletID, req.Name, req.Type, req.Currency, req.Balance, req.AccountNumber, req.CreditLimit, req.DueDay))
}

func (r PostgresRepository) DeleteWallet(ctx context.Context, userID, walletID string) error {
	result, err := r.DB.ExecContext(ctx, `
DELETE FROM wallets
WHERE user_id = $1 AND id = $2
`, userID, walletID)
	if err != nil {
		return err
	}

	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type walletScanner interface {
	Scan(dest ...any) error
}

func scanWalletRow(row walletScanner) (Wallet, error) {
	return scanWallet(row)
}

func scanWallet(scanner walletScanner) (Wallet, error) {
	var wallet Wallet
	var dueDay sql.NullInt64
	err := scanner.Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Name,
		&wallet.Type,
		&wallet.Currency,
		&wallet.Balance,
		&wallet.AccountNumber,
		&wallet.CreditLimit,
		&dueDay,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)
	if err != nil {
		return Wallet{}, err
	}

	if dueDay.Valid {
		value := int(dueDay.Int64)
		wallet.DueDay = &value
	}

	return wallet, nil
}
