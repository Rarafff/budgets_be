package report

import (
	"context"
	"database/sql"
	"math"
	"time"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) GetMonthly(ctx context.Context, userID string, start, end time.Time) (MonthlyReport, error) {
	report := MonthlyReport{}

	if err := r.DB.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0),
	COUNT(*)
FROM transactions
WHERE user_id = $1
	AND transaction_date >= $2
	AND transaction_date < $3
`, userID, start, end).Scan(&report.Income, &report.Expense, &report.TransactionCount); err != nil {
		return MonthlyReport{}, err
	}

	report.Net = report.Income - report.Expense
	report.Saved = math.Max(report.Net, 0)
	if report.Income > 0 {
		report.SavingsRatio = (report.Saved / report.Income) * 100
	}

	if err := r.DB.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN type IN ('Credit Card', 'Paylater') THEN balance ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN type NOT IN ('Credit Card', 'Paylater') THEN balance ELSE 0 END), 0)
FROM wallets
WHERE user_id = $1
`, userID).Scan(&report.DebtBalance, &report.CashBalance); err != nil {
		return MonthlyReport{}, err
	}

	var err error
	report.TopCategories, err = r.topCategories(ctx, userID, start, end, report.Expense)
	if err != nil {
		return MonthlyReport{}, err
	}

	report.DailySpending, err = r.dailySpending(ctx, userID, start, end)
	if err != nil {
		return MonthlyReport{}, err
	}
	report.NoSpendDays, report.AverageDailyExpense, report.HighestSpendingDay = summarizeDaily(report.DailySpending, report.Expense)

	report.BudgetPerformance, err = r.budgetPerformance(ctx, userID, start.Format("2006-01"))
	if err != nil {
		return MonthlyReport{}, err
	}

	report.GoalProgress, err = r.goalProgress(ctx, userID)
	if err != nil {
		return MonthlyReport{}, err
	}

	if err := r.DB.QueryRowContext(ctx, `
SELECT COALESCE(SUM(amount), 0)
FROM goal_contributions
WHERE user_id = $1
	AND contribution_date >= $2
	AND contribution_date < $3
`, userID, start, end).Scan(&report.GoalContributions); err != nil {
		return MonthlyReport{}, err
	}

	report.HealthScore = calculateHealth(report)
	return report, nil
}

func (r PostgresRepository) topCategories(ctx context.Context, userID string, start, end time.Time, totalExpense float64) ([]CategoryBreakdown, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT COALESCE(NULLIF(category, ''), 'Uncategorized') AS category, SUM(amount) AS amount
FROM transactions
WHERE user_id = $1
	AND type = 'expense'
	AND transaction_date >= $2
	AND transaction_date < $3
GROUP BY COALESCE(NULLIF(category, ''), 'Uncategorized')
ORDER BY amount DESC
LIMIT 10
`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []CategoryBreakdown{}
	for rows.Next() {
		var item CategoryBreakdown
		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return nil, err
		}
		if totalExpense > 0 {
			item.Percent = (item.Amount / totalExpense) * 100
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) dailySpending(ctx context.Context, userID string, start, end time.Time) ([]DailySpending, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT series.day::date::text, EXTRACT(DAY FROM series.day)::int,
	COALESCE(SUM(t.amount), 0)
FROM generate_series($2::date, ($3::date - INTERVAL '1 day'), INTERVAL '1 day') AS series(day)
LEFT JOIN transactions t ON t.user_id = $1
	AND t.type = 'expense'
	AND t.transaction_date = series.day::date
GROUP BY series.day
ORDER BY series.day ASC
`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []DailySpending{}
	for rows.Next() {
		var item DailySpending
		if err := rows.Scan(&item.Date, &item.Day, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) budgetPerformance(ctx context.Context, userID, periodMonth string) ([]BudgetPerformance, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT b.category, b.group_name, b.limit_amount,
	COALESCE(SUM(t.amount), 0) AS used,
	CASE WHEN b.limit_amount > 0 THEN LEAST((COALESCE(SUM(t.amount), 0) / b.limit_amount) * 100, 999) ELSE 0 END AS percent_used
FROM budgets b
LEFT JOIN transactions t ON t.user_id = b.user_id
	AND t.type = 'expense'
	AND t.category = b.category
	AND TO_CHAR(t.transaction_date, 'YYYY-MM') = b.period_month
WHERE b.user_id = $1 AND b.period_month = $2
GROUP BY b.id
ORDER BY percent_used DESC, b.category ASC
`, userID, periodMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []BudgetPerformance{}
	for rows.Next() {
		var item BudgetPerformance
		if err := rows.Scan(&item.Category, &item.GroupName, &item.Allocation, &item.Used, &item.PercentUsed); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) goalProgress(ctx context.Context, userID string) ([]GoalProgress, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT id::text, name, category, current_amount, target_amount,
	CASE WHEN target_amount > 0 THEN LEAST((current_amount / target_amount) * 100, 999) ELSE 0 END AS percent
FROM goals
WHERE user_id = $1
ORDER BY is_emergency DESC, percent DESC, created_at ASC
LIMIT 10
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []GoalProgress{}
	for rows.Next() {
		var item GoalProgress
		if err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.CurrentAmount, &item.TargetAmount, &item.Percent); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func summarizeDaily(days []DailySpending, totalExpense float64) (int, float64, *DailySpending) {
	if len(days) == 0 {
		return 0, 0, nil
	}

	noSpendDays := 0
	var highest *DailySpending
	for index := range days {
		if days[index].Amount == 0 {
			noSpendDays++
		}
		if highest == nil || days[index].Amount > highest.Amount {
			day := days[index]
			highest = &day
		}
	}

	return noSpendDays, totalExpense / float64(len(days)), highest
}

func calculateHealth(report MonthlyReport) HealthScore {
	budgetDiscipline := 100.0
	if len(report.BudgetPerformance) > 0 {
		ok := 0
		for _, budget := range report.BudgetPerformance {
			if budget.PercentUsed <= 100 {
				ok++
			}
		}
		budgetDiscipline = (float64(ok) / float64(len(report.BudgetPerformance))) * 100
	}

	runwayMonths := 0.0
	if report.Expense > 0 {
		runwayMonths = report.CashBalance / report.Expense
	}

	debtRatio := 0.0
	if report.CashBalance+report.DebtBalance > 0 {
		debtRatio = report.DebtBalance / (report.CashBalance + report.DebtBalance) * 100
	}

	score := 0.0
	score += clamp(report.SavingsRatio, 0, 30) / 30 * 30
	score += budgetDiscipline / 100 * 25
	score += clamp(runwayMonths, 0, 6) / 6 * 25
	score += (100 - clamp(debtRatio, 0, 100)) / 100 * 20

	rounded := int(math.Round(score))
	label := "Needs Attention"
	if rounded >= 80 {
		label = "Strong"
	} else if rounded >= 60 {
		label = "Healthy"
	} else if rounded >= 40 {
		label = "Watch"
	}

	return HealthScore{
		Score:            rounded,
		Label:            label,
		SavingsRatio:     report.SavingsRatio,
		BudgetDiscipline: budgetDiscipline,
		RunwayMonths:     runwayMonths,
		DebtRatio:        debtRatio,
	}
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
