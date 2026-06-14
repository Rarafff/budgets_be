package dashboard

import (
	"context"
	"database/sql"
	"time"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) GetSummary(ctx context.Context, userID string, start, end time.Time) (Summary, error) {
	summary := Summary{
		Period: Period{
			Start: start.Format("2006-01-02"),
			End:   end.AddDate(0, 0, -1).Format("2006-01-02"),
		},
	}

	if err := r.DB.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0)
FROM transactions
WHERE user_id = $1
	AND scope = 'personal'
	AND transaction_date >= $2
	AND transaction_date < $3
`, userID, start, end).Scan(&summary.MonthlyIncome, &summary.MonthlyExpense); err != nil {
		return Summary{}, err
	}
	summary.MonthlyNet = summary.MonthlyIncome - summary.MonthlyExpense

	if err := r.DB.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN type IN ('Credit Card', 'Paylater') THEN balance ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN type NOT IN ('Credit Card', 'Paylater') THEN balance ELSE 0 END), 0)
FROM wallets
WHERE user_id = $1
`, userID).Scan(&summary.DebtBalance, &summary.CashBalance); err != nil {
		return Summary{}, err
	}
	summary.LiquidAssets = summary.CashBalance
	if summary.MonthlyExpense > 0 {
		summary.EmergencyFundMonths = summary.LiquidAssets / summary.MonthlyExpense
	}

	budgetLimit, budgetSpent, budgetGroups, err := r.budgetSummary(ctx, userID, start.Format("2006-01"))
	if err != nil {
		return Summary{}, err
	}
	summary.BudgetLimit = budgetLimit
	summary.BudgetSpent = budgetSpent
	summary.BudgetRemaining = budgetLimit - budgetSpent
	if summary.BudgetRemaining < 0 {
		summary.BudgetOverSpent = -summary.BudgetRemaining
		summary.BudgetRemaining = 0
	}
	if budgetLimit > 0 {
		summary.BudgetUsedPercent = (budgetSpent / budgetLimit) * 100
	}
	summary.BudgetByGroup = budgetGroups

	incomingBills, err := r.incomingBills(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	summary.IncomingBills = incomingBills

	categories, err := r.expenseByCategory(ctx, userID, start, end)
	if err != nil {
		return Summary{}, err
	}
	summary.ExpenseByCategory = categories

	recent, err := r.recentTransactions(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	summary.RecentTransactions = recent

	events, err := r.calendarEvents(ctx, userID, start, end)
	if err != nil {
		return Summary{}, err
	}
	summary.CalendarEvents = events

	wallets, err := r.wallets(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	summary.Wallets = wallets

	return summary, nil
}

func (r PostgresRepository) incomingBills(ctx context.Context, userID string) ([]IncomingBill, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT b.id::text, b.name, b.category, b.provider, b.wallet_id::text, w.name, b.amount, b.due_date::text,
	CASE
		WHEN b.status <> 'paid' AND b.due_date < CURRENT_DATE THEN 'overdue'
		ELSE b.status
	END AS status,
	b.note,
	b.is_recurring,
	b.repeat_interval
FROM bills b
LEFT JOIN wallets w ON w.id = b.wallet_id
WHERE b.user_id = $1
	AND b.status <> 'paid'
ORDER BY b.due_date ASC, b.created_at DESC
LIMIT 5
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []IncomingBill{}
	for rows.Next() {
		var item IncomingBill
		var walletID sql.NullString
		var walletName sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.Provider, &walletID, &walletName, &item.Amount, &item.DueDate, &item.Status, &item.Note, &item.IsRecurring, &item.RepeatInterval); err != nil {
			return nil, err
		}
		if walletID.Valid {
			value := walletID.String
			item.WalletID = &value
		}
		if walletName.Valid {
			value := walletName.String
			item.WalletName = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) budgetSummary(ctx context.Context, userID, periodMonth string) (float64, float64, []BudgetGroup, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT b.group_name,
	COALESCE(SUM(b.limit_amount), 0) AS limit_amount,
	COALESCE(SUM(spent.spent_amount), 0) AS spent_amount
FROM budgets b
LEFT JOIN LATERAL (
	SELECT COALESCE(SUM(t.amount), 0) AS spent_amount
	FROM transactions t
	WHERE t.user_id = b.user_id
		AND t.scope = 'personal'
		AND t.type = 'expense'
		AND t.category = b.category
		AND TO_CHAR(t.transaction_date, 'YYYY-MM') = b.period_month
) spent ON TRUE
WHERE b.user_id = $1 AND b.period_month = $2 AND b.scope = 'personal'
GROUP BY b.group_name
ORDER BY CASE b.group_name WHEN 'Needs' THEN 1 WHEN 'Wants' THEN 2 ELSE 3 END
`, userID, periodMonth)
	if err != nil {
		return 0, 0, nil, err
	}
	defer rows.Close()

	totalLimit := 0.0
	totalSpent := 0.0
	groups := []BudgetGroup{}
	for rows.Next() {
		var group BudgetGroup
		if err := rows.Scan(&group.GroupName, &group.Limit, &group.Spent); err != nil {
			return 0, 0, nil, err
		}
		group.Remaining = group.Limit - group.Spent
		if group.Remaining < 0 {
			group.OverSpent = -group.Remaining
			group.Remaining = 0
		}
		if group.Limit > 0 {
			group.Percent = (group.Spent / group.Limit) * 100
		}
		totalLimit += group.Limit
		totalSpent += group.Spent
		groups = append(groups, group)
	}
	return totalLimit, totalSpent, groups, rows.Err()
}

func (r PostgresRepository) expenseByCategory(ctx context.Context, userID string, start, end time.Time) ([]CategorySummary, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT COALESCE(NULLIF(category, ''), 'Uncategorized') AS category, SUM(amount) AS amount
FROM transactions
WHERE user_id = $1
	AND scope = 'personal'
	AND type = 'expense'
	AND transaction_date >= $2
	AND transaction_date < $3
GROUP BY COALESCE(NULLIF(category, ''), 'Uncategorized')
ORDER BY amount DESC
LIMIT 5
`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []CategorySummary{}
	for rows.Next() {
		var item CategorySummary
		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) recentTransactions(ctx context.Context, userID string) ([]RecentTransaction, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT t.id::text, t.type, t.title, t.category, w.name, t.amount, t.transaction_date::text
FROM transactions t
JOIN wallets w ON w.id = t.wallet_id
WHERE t.user_id = $1
	AND t.scope = 'personal'
ORDER BY t.transaction_date DESC, t.created_at DESC
LIMIT 20
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []RecentTransaction{}
	for rows.Next() {
		var item RecentTransaction
		if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Category, &item.WalletName, &item.Amount, &item.TransactionDate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) calendarEvents(ctx context.Context, userID string, start, end time.Time) ([]CalendarEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT transaction_date::text, type, SUM(amount)
FROM transactions
WHERE user_id = $1
	AND scope = 'personal'
	AND transaction_date >= $2
	AND transaction_date < $3
GROUP BY transaction_date, type
ORDER BY transaction_date ASC
`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []CalendarEvent{}
	for rows.Next() {
		var item CalendarEvent
		if err := rows.Scan(&item.Date, &item.Type, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) wallets(ctx context.Context, userID string) ([]WalletSummary, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, name, type, currency, balance
FROM wallets
WHERE user_id = $1
ORDER BY created_at ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []WalletSummary{}
	for rows.Next() {
		var item WalletSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Currency, &item.Balance); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
