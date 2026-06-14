package bill

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListBills(ctx context.Context, userID string) ([]Bill, error) {
	rows, err := r.DB.QueryContext(ctx, billSelect()+`
WHERE b.user_id = $1
ORDER BY
	CASE b.status WHEN 'overdue' THEN 1 WHEN 'upcoming' THEN 2 WHEN 'paid' THEN 3 ELSE 4 END,
	b.due_date ASC,
	b.created_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bills := []Bill{}
	for rows.Next() {
		bill, err := scanBill(rows)
		if err != nil {
			return nil, err
		}
		bills = append(bills, bill)
	}
	return bills, rows.Err()
}

func (r PostgresRepository) CreateBill(ctx context.Context, userID string, req SaveBillRequest) (Bill, error) {
	status := effectiveStatus(req.Status, req.DueDate)
	return scanBillRow(r.DB.QueryRowContext(ctx, `
WITH inserted AS (
	INSERT INTO bills (user_id, wallet_id, name, category, provider, amount, due_date, status, note, is_recurring, repeat_interval)
	VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	RETURNING *
)
`+billSelectFrom("inserted")+`
`, userID, req.WalletID, req.Name, req.Category, req.Provider, req.Amount, req.DueDate, status, req.Note, req.IsRecurring, req.RepeatInterval))
}

func (r PostgresRepository) UpdateBill(ctx context.Context, userID, billID string, req SaveBillRequest) (Bill, error) {
	status := effectiveStatus(req.Status, req.DueDate)
	return scanBillRow(r.DB.QueryRowContext(ctx, `
WITH updated AS (
	UPDATE bills
	SET wallet_id = NULLIF($3, '')::uuid,
		name = $4,
		category = $5,
		provider = $6,
		amount = $7,
		due_date = $8,
		status = $9,
		note = $10,
		is_recurring = $11,
		repeat_interval = $12,
		updated_at = NOW()
	WHERE user_id = $1 AND id = $2
	RETURNING *
)
`+billSelectFrom("updated")+`
`, userID, billID, req.WalletID, req.Name, req.Category, req.Provider, req.Amount, req.DueDate, status, req.Note, req.IsRecurring, req.RepeatInterval))
}

func (r PostgresRepository) DeleteBill(ctx context.Context, userID, billID string) error {
	result, err := r.DB.ExecContext(ctx, `
DELETE FROM bills
WHERE user_id = $1 AND id = $2
`, userID, billID)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r PostgresRepository) PayBill(ctx context.Context, userID, billID string, req PayBillRequest) (Bill, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback()

	existing, err := scanBillRow(tx.QueryRowContext(ctx, billSelect()+`
WHERE b.user_id = $1 AND b.id = $2
FOR UPDATE OF b
`, userID, billID))
	if err != nil {
		return Bill{}, err
	}
	if existing.Status == "paid" {
		return Bill{}, errors.New("bill is already paid")
	}

	periodMonth := req.PaymentDate[:7]
	exists, err := budgetCategoryExists(ctx, tx, userID, existing.Category, periodMonth)
	if err != nil {
		return Bill{}, err
	}
	if !exists {
		return Bill{}, errors.New("bill category must match a budget category for " + periodMonth)
	}
	if err := ensureWallet(ctx, tx, userID, req.WalletID); err != nil {
		return Bill{}, err
	}
	if err := updateWalletBalance(ctx, tx, userID, req.WalletID, -existing.Amount); err != nil {
		return Bill{}, err
	}

	var transactionID string
	err = tx.QueryRowContext(ctx, `
INSERT INTO transactions (user_id, wallet_id, type, title, category, note, amount, transaction_date)
VALUES ($1, $2, 'expense', $3, $4, $5, $6, $7)
RETURNING id::text
`, userID, req.WalletID, existing.Name, existing.Category, "Bill payment"+noteSuffix(existing.Provider), existing.Amount, req.PaymentDate).Scan(&transactionID)
	if err != nil {
		return Bill{}, err
	}

	paid, err := scanBillRow(tx.QueryRowContext(ctx, `
WITH updated AS (
	UPDATE bills
	SET wallet_id = $3,
		status = 'paid',
		paid_transaction_id = $4,
		paid_at = NOW(),
		updated_at = NOW()
	WHERE user_id = $1 AND id = $2
	RETURNING *
)
`+billSelectFrom("updated")+`
`, userID, billID, req.WalletID, transactionID))
	if err != nil {
		return Bill{}, err
	}

	if existing.IsRecurring && existing.RepeatInterval == "monthly" {
		if err := createNextRecurringBill(ctx, tx, paid); err != nil {
			return Bill{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Bill{}, err
	}
	return paid, nil
}

func (r PostgresRepository) BudgetCategoryExists(ctx context.Context, userID, category, periodMonth string) (bool, error) {
	return budgetCategoryExists(ctx, r.DB, userID, category, periodMonth)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func budgetCategoryExists(ctx context.Context, q queryer, userID, category, periodMonth string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM budgets
	WHERE user_id = $1 AND category = $2 AND period_month = $3
)
`, userID, category, periodMonth).Scan(&exists)
	return exists, err
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

func billSelect() string {
	return billSelectFrom("bills")
}

func billSelectFrom(source string) string {
	return `
SELECT b.id::text, b.user_id::text, b.wallet_id::text, w.name, b.name, b.category, b.provider,
	b.amount, b.due_date::text, b.status, b.note, b.is_recurring, b.repeat_interval, b.paid_transaction_id::text, b.paid_at,
	b.created_at, b.updated_at
FROM ` + source + ` b
LEFT JOIN wallets w ON w.id = b.wallet_id
`
}

type billScanner interface {
	Scan(dest ...any) error
}

func scanBillRow(row billScanner) (Bill, error) {
	return scanBill(row)
}

func scanBill(scanner billScanner) (Bill, error) {
	var bill Bill
	var walletID sql.NullString
	var walletName sql.NullString
	var paidTransactionID sql.NullString
	var paidAt sql.NullTime

	err := scanner.Scan(
		&bill.ID,
		&bill.UserID,
		&walletID,
		&walletName,
		&bill.Name,
		&bill.Category,
		&bill.Provider,
		&bill.Amount,
		&bill.DueDate,
		&bill.Status,
		&bill.Note,
		&bill.IsRecurring,
		&bill.RepeatInterval,
		&paidTransactionID,
		&paidAt,
		&bill.CreatedAt,
		&bill.UpdatedAt,
	)
	if err != nil {
		return Bill{}, err
	}
	if walletID.Valid {
		value := walletID.String
		bill.WalletID = &value
	}
	if walletName.Valid {
		value := walletName.String
		bill.WalletName = &value
	}
	if paidTransactionID.Valid {
		value := paidTransactionID.String
		bill.PaidTransactionID = &value
	}
	if paidAt.Valid {
		value := paidAt.Time
		bill.PaidAt = &value
	}
	return bill, nil
}

func createNextRecurringBill(ctx context.Context, tx *sql.Tx, paid Bill) error {
	nextDueDate, err := nextMonthlyDueDate(paid.DueDate)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO bills (user_id, wallet_id, name, category, provider, amount, due_date, status, note, is_recurring, repeat_interval)
SELECT $1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, TRUE, 'monthly'
WHERE NOT EXISTS (
	SELECT 1
	FROM bills
	WHERE user_id = $1
		AND name = $3
		AND category = $4
		AND provider = $5
		AND due_date = $7
		AND status <> 'paid'
)
`, paid.UserID, nullableStringValue(paid.WalletID), paid.Name, paid.Category, paid.Provider, paid.Amount, nextDueDate, effectiveStatus("upcoming", nextDueDate), paid.Note)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return nil
	}
	return nil
}

func nullableStringValue(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nextMonthlyDueDate(dueDate string) (string, error) {
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return "", err
	}

	targetYear, targetMonth := due.Year(), due.Month()+1
	if targetMonth > 12 {
		targetMonth = 1
		targetYear++
	}

	day := due.Day()
	lastDay := time.Date(targetYear, targetMonth+1, 0, 0, 0, 0, 0, due.Location()).Day()
	if day > lastDay {
		day = lastDay
	}

	return time.Date(targetYear, targetMonth, day, 0, 0, 0, 0, due.Location()).Format("2006-01-02"), nil
}

func effectiveStatus(status, dueDate string) string {
	if status == "paid" {
		return "paid"
	}
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return "upcoming"
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if due.Before(today) {
		return "overdue"
	}
	return "upcoming"
}

func noteSuffix(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}
	return ": " + provider
}
