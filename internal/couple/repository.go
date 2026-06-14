package couple

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) GetMyCouple(ctx context.Context, userID string) (Couple, error) {
	return scanCoupleRow(r.DB.QueryRowContext(ctx, coupleSelect()+`
JOIN couple_members cm ON cm.couple_id = c.id
WHERE cm.user_id = $1
`, userID))
}

func (r PostgresRepository) CreateCouple(ctx context.Context, userID string, req SaveCoupleRequest) (Couple, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Couple{}, err
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRowContext(ctx, `SELECT couple_id::text FROM couple_members WHERE user_id = $1`, userID).Scan(&existing)
	if err == nil {
		return Couple{}, errors.New("user is already in a couple")
	}
	if err != nil && err != sql.ErrNoRows {
		return Couple{}, err
	}

	var created Couple
	for i := 0; i < 5; i++ {
		code, err := newInviteCode()
		if err != nil {
			return Couple{}, err
		}
		created, err = scanCoupleRow(tx.QueryRowContext(ctx, `
INSERT INTO couples (name, invite_code, created_by)
VALUES ($1, $2, $3)
RETURNING id::text, name, invite_code, created_by::text, created_at, updated_at
`, req.Name, code, userID))
		if isUniqueViolation(err) {
			continue
		}
		if err != nil {
			return Couple{}, err
		}
		break
	}
	if created.ID == "" {
		return Couple{}, errors.New("failed to generate invite code")
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO couple_members (couple_id, user_id, role)
VALUES ($1, $2, 'owner')
`, created.ID, userID); err != nil {
		return Couple{}, err
	}

	if err := tx.Commit(); err != nil {
		return Couple{}, err
	}
	return created, nil
}

func (r PostgresRepository) JoinCouple(ctx context.Context, userID, inviteCode string) (Couple, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Couple{}, err
	}
	defer tx.Rollback()

	couple, err := scanCoupleRow(tx.QueryRowContext(ctx, coupleSelect()+`
WHERE c.invite_code = $1
`, strings.ToUpper(inviteCode)))
	if err != nil {
		return Couple{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO couple_members (couple_id, user_id, role)
VALUES ($1, $2, 'member')
`, couple.ID, userID)
	if isUniqueViolation(err) {
		return Couple{}, errors.New("user is already in a couple")
	}
	if err != nil {
		return Couple{}, err
	}

	if err := tx.Commit(); err != nil {
		return Couple{}, err
	}
	return couple, nil
}

func (r PostgresRepository) GetSummary(ctx context.Context, userID, periodMonth string) (Summary, error) {
	couple, err := r.GetMyCouple(ctx, userID)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	summary.CoupleID = couple.ID
	summary.CoupleName = couple.Name

	err = r.DB.QueryRowContext(ctx, `
SELECT
	COALESCE((
		SELECT SUM(b.limit_amount)
		FROM budgets b
		JOIN couple_members cm ON cm.user_id = b.user_id
		WHERE cm.couple_id = $1
			AND b.scope = 'personal'
			AND b.period_month = $2
	), 0),
	COALESCE((
		SELECT SUM(t.amount)
		FROM transactions t
		JOIN couple_members cm ON cm.user_id = t.user_id
		WHERE cm.couple_id = $1
			AND t.scope = 'personal'
			AND t.type = 'expense'
			AND TO_CHAR(t.transaction_date, 'YYYY-MM') = $2
	), 0),
	COALESCE((
		SELECT SUM(t.amount)
		FROM transactions t
		WHERE t.user_id = $3
			AND t.scope = 'personal'
			AND t.type = 'expense'
			AND TO_CHAR(t.transaction_date, 'YYYY-MM') = $2
	), 0),
	COALESCE((
		SELECT SUM(t.amount)
		FROM transactions t
		JOIN couple_members cm ON cm.user_id = t.user_id
		WHERE cm.couple_id = $1
			AND t.user_id <> $3
			AND t.scope = 'personal'
			AND t.type = 'expense'
			AND TO_CHAR(t.transaction_date, 'YYYY-MM') = $2
	), 0)
`, couple.ID, periodMonth, userID).Scan(&summary.BudgetLimit, &summary.BudgetSpent, &summary.MySpending, &summary.PartnerSpending)
	if err != nil {
		return Summary{}, err
	}
	summary.BudgetRemaining = summary.BudgetLimit - summary.BudgetSpent
	if summary.BudgetRemaining < 0 {
		summary.BudgetRemaining = 0
	}
	return summary, nil
}

func coupleSelect() string {
	return `
SELECT c.id::text, c.name, c.invite_code, c.created_by::text, c.created_at, c.updated_at
FROM couples c
`
}

type coupleScanner interface {
	Scan(dest ...any) error
}

func scanCoupleRow(scanner coupleScanner) (Couple, error) {
	var couple Couple
	err := scanner.Scan(&couple.ID, &couple.Name, &couple.InviteCode, &couple.CreatedBy, &couple.CreatedAt, &couple.UpdatedAt)
	return couple, err
}

func newInviteCode() (string, error) {
	var bytes [5]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(bytes[:]), "="), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
