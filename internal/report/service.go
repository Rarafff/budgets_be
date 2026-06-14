package report

import (
	"context"
	"errors"
	"strings"
	"time"
)

type MonthlyReport struct {
	Period              Period              `json:"period"`
	Income              float64             `json:"income"`
	Expense             float64             `json:"expense"`
	Net                 float64             `json:"net"`
	Saved               float64             `json:"saved"`
	SavingsRatio        float64             `json:"savingsRatio"`
	CashBalance         float64             `json:"cashBalance"`
	DebtBalance         float64             `json:"debtBalance"`
	HealthScore         HealthScore         `json:"healthScore"`
	TopCategories       []CategoryBreakdown `json:"topCategories"`
	DailySpending       []DailySpending     `json:"dailySpending"`
	BudgetPerformance   []BudgetPerformance `json:"budgetPerformance"`
	GoalProgress        []GoalProgress      `json:"goalProgress"`
	GoalContributions   float64             `json:"goalContributions"`
	TransactionCount    int                 `json:"transactionCount"`
	NoSpendDays         int                 `json:"noSpendDays"`
	AverageDailyExpense float64             `json:"averageDailyExpense"`
	HighestSpendingDay  *DailySpending      `json:"highestSpendingDay"`
}

type Period struct {
	Month string `json:"month"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type HealthScore struct {
	Score            int     `json:"score"`
	Label            string  `json:"label"`
	SavingsRatio     float64 `json:"savingsRatio"`
	BudgetDiscipline float64 `json:"budgetDiscipline"`
	RunwayMonths     float64 `json:"runwayMonths"`
	DebtRatio        float64 `json:"debtRatio"`
}

type CategoryBreakdown struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Percent  float64 `json:"percent"`
}

type DailySpending struct {
	Date   string  `json:"date"`
	Day    int     `json:"day"`
	Amount float64 `json:"amount"`
}

type BudgetPerformance struct {
	Category    string  `json:"category"`
	GroupName   string  `json:"groupName"`
	Allocation  float64 `json:"allocation"`
	Used        float64 `json:"used"`
	PercentUsed float64 `json:"percentUsed"`
}

type GoalProgress struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Category      string  `json:"category"`
	CurrentAmount float64 `json:"currentAmount"`
	TargetAmount  float64 `json:"targetAmount"`
	Percent       float64 `json:"percent"`
}

type Repository interface {
	GetMonthly(ctx context.Context, userID string, start, end time.Time) (MonthlyReport, error)
}

type Service struct {
	Repo Repository
}

func (s Service) Monthly(ctx context.Context, userID, periodMonth string) (MonthlyReport, error) {
	start, end, month, err := monthRange(periodMonth)
	if err != nil {
		return MonthlyReport{}, err
	}

	report, err := s.Repo.GetMonthly(ctx, userID, start, end)
	if err != nil {
		return MonthlyReport{}, err
	}
	report.Period = Period{
		Month: month,
		Start: start.Format("2006-01-02"),
		End:   end.AddDate(0, 0, -1).Format("2006-01-02"),
	}
	return report, nil
}

func monthRange(periodMonth string) (time.Time, time.Time, string, error) {
	periodMonth = strings.TrimSpace(periodMonth)
	if periodMonth == "" {
		periodMonth = time.Now().Format("2006-01")
	}

	parsed, err := time.Parse("2006-01", periodMonth)
	if err != nil {
		return time.Time{}, time.Time{}, "", errors.New("periodMonth must be YYYY-MM")
	}

	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.Local)
	return start, start.AddDate(0, 1, 0), periodMonth, nil
}
