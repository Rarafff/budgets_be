package budget

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListBudgets(ctx context.Context, userID, periodMonth, scope, coupleID string) ([]Budget, error) {
	if scope == "couple" {
		ok, err := isCoupleMember(ctx, r.DB, userID, coupleID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("couple not found")
		}
	}

	rows, err := r.DB.QueryContext(ctx, `
SELECT b.id::text, b.user_id::text, b.scope, b.couple_id::text, b.group_name, b.category, b.period_month, b.limit_amount,
	COALESCE(SUM(t.amount), 0) AS spent_amount,
	CASE WHEN b.limit_amount > 0 THEN LEAST((COALESCE(SUM(t.amount), 0) / b.limit_amount) * 100, 999) ELSE 0 END AS progress,
	b.icon, b.created_at, b.updated_at
FROM budgets b
LEFT JOIN transactions t ON t.type = 'expense'
	AND t.category = b.category
	AND TO_CHAR(t.transaction_date, 'YYYY-MM') = b.period_month
	AND t.scope = b.scope
	AND COALESCE(t.couple_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(b.couple_id, '00000000-0000-0000-0000-000000000000'::uuid)
	AND (b.scope = 'couple' OR t.user_id = b.user_id)
WHERE b.period_month = $2
	AND b.scope = $3
	AND COALESCE(b.couple_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(NULLIF($4, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
	AND (b.scope = 'couple' OR b.user_id = $1)
GROUP BY b.id
ORDER BY
	CASE b.group_name WHEN 'Needs' THEN 1 WHEN 'Wants' THEN 2 ELSE 3 END,
	b.category ASC
`, userID, periodMonth, scope, coupleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	budgets := []Budget{}
	for rows.Next() {
		budget, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, budget)
	}
	return budgets, rows.Err()
}

func (r PostgresRepository) CreateBudget(ctx context.Context, userID string, req SaveBudgetRequest) (Budget, error) {
	if err := r.ensureCoupleAccess(ctx, userID, req); err != nil {
		return Budget{}, err
	}

	budget, err := scanBudgetRow(r.DB.QueryRowContext(ctx, `
WITH inserted AS (
	INSERT INTO budgets (user_id, scope, couple_id, group_name, category, period_month, limit_amount, icon)
	VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8)
	RETURNING *
)
SELECT b.id::text, b.user_id::text, b.scope, b.couple_id::text, b.group_name, b.category, b.period_month, b.limit_amount,
	0::numeric AS spent_amount,
	0::numeric AS progress,
	b.icon, b.created_at, b.updated_at
FROM inserted b
`, userID, req.Scope, req.CoupleID, req.GroupName, req.Category, req.PeriodMonth, req.LimitAmount, req.Icon))
	if isUniqueViolation(err) {
		return Budget{}, errors.New("budget category already exists for this month")
	}
	return budget, err
}

func (r PostgresRepository) UpdateBudget(ctx context.Context, userID, budgetID string, req SaveBudgetRequest) (Budget, error) {
	if err := r.ensureCoupleAccess(ctx, userID, req); err != nil {
		return Budget{}, err
	}

	budget, err := scanBudgetRow(r.DB.QueryRowContext(ctx, `
WITH updated AS (
	UPDATE budgets
	SET scope = $3,
		couple_id = NULLIF($4, '')::uuid,
		group_name = $5,
		category = $6,
		period_month = $7,
		limit_amount = $8,
		icon = $9,
		updated_at = NOW()
	WHERE id = $2
		AND (
			user_id = $1
			OR EXISTS (
				SELECT 1
				FROM couple_members cm
				WHERE cm.user_id = $1 AND cm.couple_id = budgets.couple_id
			)
		)
	RETURNING *
)
SELECT b.id::text, b.user_id::text, b.scope, b.couple_id::text, b.group_name, b.category, b.period_month, b.limit_amount,
	COALESCE(SUM(t.amount), 0) AS spent_amount,
	CASE WHEN b.limit_amount > 0 THEN LEAST((COALESCE(SUM(t.amount), 0) / b.limit_amount) * 100, 999) ELSE 0 END AS progress,
	b.icon, b.created_at, b.updated_at
FROM updated b
LEFT JOIN transactions t ON t.user_id = b.user_id
	AND t.type = 'expense'
	AND t.category = b.category
	AND TO_CHAR(t.transaction_date, 'YYYY-MM') = b.period_month
	AND t.scope = b.scope
	AND COALESCE(t.couple_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(b.couple_id, '00000000-0000-0000-0000-000000000000'::uuid)
GROUP BY b.id, b.user_id, b.scope, b.couple_id, b.group_name, b.category, b.period_month, b.limit_amount, b.icon, b.created_at, b.updated_at
`, userID, budgetID, req.Scope, req.CoupleID, req.GroupName, req.Category, req.PeriodMonth, req.LimitAmount, req.Icon))
	if isUniqueViolation(err) {
		return Budget{}, errors.New("budget category already exists for this month")
	}
	return budget, err
}

func (r PostgresRepository) DeleteBudget(ctx context.Context, userID, budgetID string) error {
	result, err := r.DB.ExecContext(ctx, `
DELETE FROM budgets
WHERE id = $2
	AND (
		user_id = $1
		OR EXISTS (
			SELECT 1
			FROM couple_members cm
			WHERE cm.user_id = $1 AND cm.couple_id = budgets.couple_id
		)
	)
`, userID, budgetID)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r PostgresRepository) ensureCoupleAccess(ctx context.Context, userID string, req SaveBudgetRequest) error {
	if req.Scope != "couple" {
		return nil
	}
	ok, err := isCoupleMember(ctx, r.DB, userID, req.CoupleID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("couple not found")
	}
	return nil
}

type coupleMemberQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func isCoupleMember(ctx context.Context, q coupleMemberQueryer, userID, coupleID string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM couple_members
	WHERE user_id = $1 AND couple_id = NULLIF($2, '')::uuid
)
`, userID, coupleID).Scan(&exists)
	return exists, err
}

type budgetScanner interface {
	Scan(dest ...any) error
}

func scanBudgetRow(row budgetScanner) (Budget, error) {
	return scanBudget(row)
}

func scanBudget(scanner budgetScanner) (Budget, error) {
	var budget Budget
	var coupleID sql.NullString
	err := scanner.Scan(
		&budget.ID,
		&budget.UserID,
		&budget.Scope,
		&coupleID,
		&budget.GroupName,
		&budget.Category,
		&budget.PeriodMonth,
		&budget.LimitAmount,
		&budget.SpentAmount,
		&budget.Progress,
		&budget.Icon,
		&budget.CreatedAt,
		&budget.UpdatedAt,
	)
	if coupleID.Valid {
		value := coupleID.String
		budget.CoupleID = &value
	}
	return budget, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
