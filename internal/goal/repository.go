package goal

import (
	"context"
	"database/sql"
	"errors"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListGoals(ctx context.Context, userID string) ([]Goal, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, user_id::text, name, category, target_amount, current_amount,
	CASE WHEN target_amount > 0 THEN LEAST((current_amount / target_amount) * 100, 999) ELSE 0 END AS progress,
	linked_wallet_id::text,
	(SELECT name FROM wallets WHERE id = goals.linked_wallet_id),
	deadline::text, icon, is_emergency, created_at, updated_at
FROM goals
WHERE goals.user_id = $1
ORDER BY is_emergency DESC, deadline ASC NULLS LAST, created_at ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := []Goal{}
	for rows.Next() {
		goal, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		goals = append(goals, goal)
	}
	return goals, rows.Err()
}

func (r PostgresRepository) CreateGoal(ctx context.Context, userID string, req SaveGoalRequest) (Goal, error) {
	if err := ensureOptionalWallet(ctx, r.DB, userID, req.LinkedWalletID); err != nil {
		return Goal{}, err
	}

	return scanGoalRow(r.DB.QueryRowContext(ctx, `
WITH inserted AS (
	INSERT INTO goals (user_id, name, category, target_amount, current_amount, linked_wallet_id, deadline, icon, is_emergency)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING *
)
SELECT g.id::text, g.user_id::text, g.name, g.category, g.target_amount, g.current_amount,
	CASE WHEN g.target_amount > 0 THEN LEAST((g.current_amount / g.target_amount) * 100, 999) ELSE 0 END,
	g.linked_wallet_id::text, w.name, g.deadline::text, g.icon, g.is_emergency, g.created_at, g.updated_at
FROM inserted g
LEFT JOIN wallets w ON w.id = g.linked_wallet_id
`, userID, req.Name, req.Category, req.TargetAmount, req.CurrentAmount, req.LinkedWalletID, req.Deadline, req.Icon, req.IsEmergency))
}

func (r PostgresRepository) UpdateGoal(ctx context.Context, userID, goalID string, req SaveGoalRequest) (Goal, error) {
	if err := ensureOptionalWallet(ctx, r.DB, userID, req.LinkedWalletID); err != nil {
		return Goal{}, err
	}

	return scanGoalRow(r.DB.QueryRowContext(ctx, `
WITH updated AS (
	UPDATE goals
	SET name = $3,
		category = $4,
		target_amount = $5,
		current_amount = $6,
		linked_wallet_id = $7,
		deadline = $8,
		icon = $9,
		is_emergency = $10,
		updated_at = NOW()
	WHERE user_id = $1 AND id = $2
	RETURNING *
)
SELECT g.id::text, g.user_id::text, g.name, g.category, g.target_amount, g.current_amount,
	CASE WHEN g.target_amount > 0 THEN LEAST((g.current_amount / g.target_amount) * 100, 999) ELSE 0 END,
	g.linked_wallet_id::text, w.name, g.deadline::text, g.icon, g.is_emergency, g.created_at, g.updated_at
FROM updated g
LEFT JOIN wallets w ON w.id = g.linked_wallet_id
`, userID, goalID, req.Name, req.Category, req.TargetAmount, req.CurrentAmount, req.LinkedWalletID, req.Deadline, req.Icon, req.IsEmergency))
}

func (r PostgresRepository) DeleteGoal(ctx context.Context, userID, goalID string) error {
	result, err := r.DB.ExecContext(ctx, `
DELETE FROM goals
WHERE user_id = $1 AND id = $2
`, userID, goalID)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r PostgresRepository) CreateContribution(ctx context.Context, userID, goalID string, req ContributionRequest) (Contribution, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Contribution{}, err
	}
	defer tx.Rollback()

	if err := ensureGoal(ctx, tx, userID, goalID); err != nil {
		return Contribution{}, err
	}
	if err := updateWalletBalance(ctx, tx, userID, req.WalletID, -req.Amount); err != nil {
		return Contribution{}, err
	}
	if err := updateGoalAmount(ctx, tx, userID, goalID, req.Amount); err != nil {
		return Contribution{}, err
	}

	contribution, err := scanContributionRow(tx.QueryRowContext(ctx, `
INSERT INTO goal_contributions (user_id, goal_id, wallet_id, amount, note, contribution_date)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, user_id::text, goal_id::text, wallet_id::text,
	(SELECT name FROM wallets WHERE id = wallet_id),
	amount, note, contribution_date::text, created_at
`, userID, goalID, req.WalletID, req.Amount, req.Note, req.ContributionDate))
	if err != nil {
		return Contribution{}, err
	}

	if err := tx.Commit(); err != nil {
		return Contribution{}, err
	}
	return contribution, nil
}

func (r PostgresRepository) ListContributions(ctx context.Context, userID, goalID string) ([]Contribution, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT c.id::text, c.user_id::text, c.goal_id::text, c.wallet_id::text, w.name,
	c.amount, c.note, c.contribution_date::text, c.created_at
FROM goal_contributions c
JOIN wallets w ON w.id = c.wallet_id
WHERE c.user_id = $1 AND c.goal_id = $2
ORDER BY c.contribution_date DESC, c.created_at DESC
`, userID, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contributions := []Contribution{}
	for rows.Next() {
		contribution, err := scanContribution(rows)
		if err != nil {
			return nil, err
		}
		contributions = append(contributions, contribution)
	}
	return contributions, rows.Err()
}

func ensureGoal(ctx context.Context, tx *sql.Tx, userID, goalID string) error {
	var exists bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM goals WHERE user_id = $1 AND id = $2)
`, userID, goalID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("goal not found")
	}
	return nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ensureOptionalWallet(ctx context.Context, db queryer, userID string, walletID *string) error {
	if walletID == nil {
		return nil
	}

	var exists bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM wallets WHERE user_id = $1 AND id = $2)
`, userID, *walletID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("linked wallet not found")
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
		return errors.New("wallet not found")
	}
	return nil
}

func updateGoalAmount(ctx context.Context, tx *sql.Tx, userID, goalID string, delta float64) error {
	result, err := tx.ExecContext(ctx, `
UPDATE goals
SET current_amount = current_amount + $3,
	updated_at = NOW()
WHERE user_id = $1 AND id = $2
`, userID, goalID, delta)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type goalScanner interface {
	Scan(dest ...any) error
}

func scanGoalRow(row goalScanner) (Goal, error) {
	return scanGoal(row)
}

func scanGoal(scanner goalScanner) (Goal, error) {
	var goal Goal
	var deadline sql.NullString
	var linkedWalletID sql.NullString
	var linkedWalletName sql.NullString
	err := scanner.Scan(
		&goal.ID,
		&goal.UserID,
		&goal.Name,
		&goal.Category,
		&goal.TargetAmount,
		&goal.CurrentAmount,
		&goal.Progress,
		&linkedWalletID,
		&linkedWalletName,
		&deadline,
		&goal.Icon,
		&goal.IsEmergency,
		&goal.CreatedAt,
		&goal.UpdatedAt,
	)
	if err != nil {
		return Goal{}, err
	}
	if deadline.Valid {
		goal.Deadline = &deadline.String
	}
	if linkedWalletID.Valid {
		goal.LinkedWalletID = &linkedWalletID.String
	}
	if linkedWalletName.Valid {
		goal.LinkedWalletName = &linkedWalletName.String
	}
	return goal, nil
}

type contributionScanner interface {
	Scan(dest ...any) error
}

func scanContributionRow(row contributionScanner) (Contribution, error) {
	return scanContribution(row)
}

func scanContribution(scanner contributionScanner) (Contribution, error) {
	var contribution Contribution
	err := scanner.Scan(
		&contribution.ID,
		&contribution.UserID,
		&contribution.GoalID,
		&contribution.WalletID,
		&contribution.WalletName,
		&contribution.Amount,
		&contribution.Note,
		&contribution.ContributionDate,
		&contribution.CreatedAt,
	)
	return contribution, err
}
