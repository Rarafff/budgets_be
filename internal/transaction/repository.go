package transaction

import (
	"context"
	"database/sql"
	"errors"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListTransactions(ctx context.Context, userID string) ([]Transaction, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT t.id::text, t.user_id::text, t.scope, t.couple_id::text, t.wallet_id::text, w.name, t.to_wallet_id::text, tw.name,
	t.type, t.title, t.category, t.note, t.amount, t.transaction_date::text, t.created_at, t.updated_at
FROM transactions t
JOIN wallets w ON w.id = t.wallet_id
LEFT JOIN wallets tw ON tw.id = t.to_wallet_id
WHERE t.user_id = $1
ORDER BY t.transaction_date DESC, t.created_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := []Transaction{}
	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	return transactions, rows.Err()
}

func (r PostgresRepository) CreateTransaction(ctx context.Context, userID string, req SaveTransactionRequest) (Transaction, error) {
	if err := r.ensureCoupleAccess(ctx, userID, req); err != nil {
		return Transaction{}, err
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback()

	if err := applyBalance(ctx, tx, userID, req, 1); err != nil {
		return Transaction{}, err
	}

	transaction, err := scanTransactionRow(tx.QueryRowContext(ctx, `
INSERT INTO transactions (user_id, scope, couple_id, wallet_id, to_wallet_id, type, title, category, note, amount, transaction_date)
VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id::text, user_id::text, scope, couple_id::text, wallet_id::text,
	(SELECT name FROM wallets WHERE id = wallet_id),
	to_wallet_id::text,
	(SELECT name FROM wallets WHERE id = to_wallet_id),
	type, title, category, note, amount, transaction_date::text, created_at, updated_at
`, userID, req.Scope, req.CoupleID, req.WalletID, req.ToWalletID, req.Type, req.Title, req.Category, req.Note, req.Amount, req.TransactionDate))
	if err != nil {
		return Transaction{}, err
	}

	if err := tx.Commit(); err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}

func (r PostgresRepository) CreateTransactions(ctx context.Context, userID string, reqs []SaveTransactionRequest) ([]Transaction, error) {
	for _, req := range reqs {
		if err := r.ensureCoupleAccess(ctx, userID, req); err != nil {
			return nil, err
		}
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	transactions := make([]Transaction, 0, len(reqs))
	for _, req := range reqs {
		if err := applyBalance(ctx, tx, userID, req, 1); err != nil {
			return nil, err
		}

		transaction, err := insertTransaction(ctx, tx, userID, req)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return transactions, nil
}

func (r PostgresRepository) UpdateTransaction(ctx context.Context, userID, transactionID string, req SaveTransactionRequest) (Transaction, error) {
	if err := r.ensureCoupleAccess(ctx, userID, req); err != nil {
		return Transaction{}, err
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback()

	oldTransaction, err := findTransactionForUpdate(ctx, tx, userID, transactionID)
	if err != nil {
		return Transaction{}, err
	}

	if err := revertBalance(ctx, tx, userID, oldTransaction); err != nil {
		return Transaction{}, err
	}
	if err := applyBalance(ctx, tx, userID, req, 1); err != nil {
		return Transaction{}, err
	}

	transaction, err := scanTransactionRow(tx.QueryRowContext(ctx, `
UPDATE transactions
SET scope = $3,
	couple_id = NULLIF($4, '')::uuid,
	wallet_id = $5,
	to_wallet_id = $6,
	type = $7,
	title = $8,
	category = $9,
	note = $10,
	amount = $11,
	transaction_date = $12,
	updated_at = NOW()
WHERE user_id = $1 AND id = $2
RETURNING id::text, user_id::text, scope, couple_id::text, wallet_id::text,
	(SELECT name FROM wallets WHERE id = wallet_id),
	to_wallet_id::text,
	(SELECT name FROM wallets WHERE id = to_wallet_id),
	type, title, category, note, amount, transaction_date::text, created_at, updated_at
`, userID, transactionID, req.Scope, req.CoupleID, req.WalletID, req.ToWalletID, req.Type, req.Title, req.Category, req.Note, req.Amount, req.TransactionDate))
	if err != nil {
		return Transaction{}, err
	}

	if err := tx.Commit(); err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}

func insertTransaction(ctx context.Context, tx *sql.Tx, userID string, req SaveTransactionRequest) (Transaction, error) {
	return scanTransactionRow(tx.QueryRowContext(ctx, `
INSERT INTO transactions (user_id, scope, couple_id, wallet_id, to_wallet_id, type, title, category, note, amount, transaction_date)
VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id::text, user_id::text, scope, couple_id::text, wallet_id::text,
	(SELECT name FROM wallets WHERE id = wallet_id),
	to_wallet_id::text,
	(SELECT name FROM wallets WHERE id = to_wallet_id),
	type, title, category, note, amount, transaction_date::text, created_at, updated_at
`, userID, req.Scope, req.CoupleID, req.WalletID, req.ToWalletID, req.Type, req.Title, req.Category, req.Note, req.Amount, req.TransactionDate))
}

func (r PostgresRepository) DeleteTransaction(ctx context.Context, userID, transactionID string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oldTransaction, err := findTransactionForUpdate(ctx, tx, userID, transactionID)
	if err != nil {
		return err
	}

	if err := revertBalance(ctx, tx, userID, oldTransaction); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
DELETE FROM transactions
WHERE user_id = $1 AND id = $2
`, userID, transactionID)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

func (r PostgresRepository) BudgetCategoryExists(ctx context.Context, userID, category, periodMonth, scope, coupleID string) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM budgets
	WHERE category = $2
		AND period_month = $3
		AND scope = $4
		AND COALESCE(couple_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(NULLIF($5, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		AND (
			(scope = 'personal' AND user_id = $1)
			OR (
				scope = 'couple'
				AND EXISTS (
					SELECT 1
					FROM couple_members cm
					WHERE cm.user_id = $1 AND cm.couple_id = budgets.couple_id
				)
			)
		)
)
`, userID, category, periodMonth, scope, coupleID).Scan(&exists)
	return exists, err
}

func (r PostgresRepository) ensureCoupleAccess(ctx context.Context, userID string, req SaveTransactionRequest) error {
	if req.Scope != "couple" {
		return nil
	}
	var exists bool
	err := r.DB.QueryRowContext(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM couple_members
	WHERE user_id = $1 AND couple_id = NULLIF($2, '')::uuid
)
`, userID, req.CoupleID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("couple not found")
	}
	return nil
}

func findTransactionForUpdate(ctx context.Context, tx *sql.Tx, userID, transactionID string) (SaveTransactionRequest, error) {
	var req SaveTransactionRequest
	var coupleID sql.NullString
	var toWalletID sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT scope, couple_id::text, wallet_id::text, to_wallet_id::text, type, title, category, note, amount, transaction_date::text
FROM transactions
WHERE user_id = $1 AND id = $2
FOR UPDATE
`, userID, transactionID).Scan(
		&req.Scope,
		&coupleID,
		&req.WalletID,
		&toWalletID,
		&req.Type,
		&req.Title,
		&req.Category,
		&req.Note,
		&req.Amount,
		&req.TransactionDate,
	)
	if err != nil {
		return SaveTransactionRequest{}, err
	}
	if toWalletID.Valid {
		req.ToWalletID = &toWalletID.String
	}
	if coupleID.Valid {
		req.CoupleID = coupleID.String
	}
	return req, nil
}

func applyBalance(ctx context.Context, tx *sql.Tx, userID string, req SaveTransactionRequest, multiplier float64) error {
	if err := ensureWallet(ctx, tx, userID, req.WalletID); err != nil {
		return err
	}

	switch req.Type {
	case "income":
		return updateWalletBalance(ctx, tx, userID, req.WalletID, req.Amount*multiplier)
	case "expense":
		return updateWalletBalance(ctx, tx, userID, req.WalletID, -req.Amount*multiplier)
	case "transfer":
		if req.ToWalletID == nil {
			return errors.New("to wallet is required for transfer")
		}
		if err := ensureWallet(ctx, tx, userID, *req.ToWalletID); err != nil {
			return err
		}
		if err := updateWalletBalance(ctx, tx, userID, req.WalletID, -req.Amount*multiplier); err != nil {
			return err
		}
		return updateWalletBalance(ctx, tx, userID, *req.ToWalletID, req.Amount*multiplier)
	default:
		return errors.New("transaction type is invalid")
	}
}

func revertBalance(ctx context.Context, tx *sql.Tx, userID string, req SaveTransactionRequest) error {
	return applyBalance(ctx, tx, userID, req, -1)
}

func ensureWallet(ctx context.Context, tx *sql.Tx, userID, walletID string) error {
	var exists bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM wallets WHERE user_id = $1 AND id = $2)
`, userID, walletID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("wallet not found")
	}
	return nil
}

func updateWalletBalance(ctx context.Context, tx *sql.Tx, userID, walletID string, delta float64) error {
	result, err := tx.ExecContext(ctx, `
UPDATE wallets
SET balance = balance + $3,
	updated_at = NOW()
WHERE user_id = $1 AND id = $2
`, userID, walletID, delta)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type transactionScanner interface {
	Scan(dest ...any) error
}

func scanTransactionRow(row transactionScanner) (Transaction, error) {
	return scanTransaction(row)
}

func scanTransaction(scanner transactionScanner) (Transaction, error) {
	var transaction Transaction
	var coupleID sql.NullString
	var toWalletID sql.NullString
	var toWalletName sql.NullString
	err := scanner.Scan(
		&transaction.ID,
		&transaction.UserID,
		&transaction.Scope,
		&coupleID,
		&transaction.WalletID,
		&transaction.WalletName,
		&toWalletID,
		&toWalletName,
		&transaction.Type,
		&transaction.Title,
		&transaction.Category,
		&transaction.Note,
		&transaction.Amount,
		&transaction.TransactionDate,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		return Transaction{}, err
	}
	if coupleID.Valid {
		transaction.CoupleID = &coupleID.String
	}
	if toWalletID.Valid {
		transaction.ToWalletID = &toWalletID.String
	}
	if toWalletName.Valid {
		transaction.ToWalletName = &toWalletName.String
	}
	return transaction, nil
}
