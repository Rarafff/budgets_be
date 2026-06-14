package dashboard

import (
	"context"
	"time"
)

type Summary struct {
	Period              Period              `json:"period"`
	MonthlyIncome       float64             `json:"monthlyIncome"`
	MonthlyExpense      float64             `json:"monthlyExpense"`
	MonthlyNet          float64             `json:"monthlyNet"`
	CashBalance         float64             `json:"cashBalance"`
	DebtBalance         float64             `json:"debtBalance"`
	LiquidAssets        float64             `json:"liquidAssets"`
	EmergencyFundMonths float64             `json:"emergencyFundMonths"`
	BudgetLimit         float64             `json:"budgetLimit"`
	BudgetSpent         float64             `json:"budgetSpent"`
	BudgetRemaining     float64             `json:"budgetRemaining"`
	BudgetOverSpent     float64             `json:"budgetOverSpent"`
	BudgetUsedPercent   float64             `json:"budgetUsedPercent"`
	BudgetByGroup       []BudgetGroup       `json:"budgetByGroup"`
	IncomingBills       []IncomingBill      `json:"incomingBills"`
	ExpenseByCategory   []CategorySummary   `json:"expenseByCategory"`
	RecentTransactions  []RecentTransaction `json:"recentTransactions"`
	CalendarEvents      []CalendarEvent     `json:"calendarEvents"`
	Wallets             []WalletSummary     `json:"wallets"`
}

type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type CategorySummary struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

type BudgetGroup struct {
	GroupName string  `json:"groupName"`
	Limit     float64 `json:"limit"`
	Spent     float64 `json:"spent"`
	Remaining float64 `json:"remaining"`
	OverSpent float64 `json:"overSpent"`
	Percent   float64 `json:"percent"`
}

type IncomingBill struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Provider       string  `json:"provider"`
	WalletID       *string `json:"walletId"`
	WalletName     *string `json:"walletName"`
	Amount         float64 `json:"amount"`
	DueDate        string  `json:"dueDate"`
	Status         string  `json:"status"`
	Note           string  `json:"note"`
	IsRecurring    bool    `json:"isRecurring"`
	RepeatInterval string  `json:"repeatInterval"`
}

type RecentTransaction struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	WalletName      string  `json:"walletName"`
	Amount          float64 `json:"amount"`
	TransactionDate string  `json:"transactionDate"`
}

type CalendarEvent struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Type   string  `json:"type"`
}

type WalletSummary struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
}

type Repository interface {
	GetSummary(ctx context.Context, userID string, start, end time.Time) (Summary, error)
}

type Service struct {
	Repo Repository
}

func (s Service) Summary(ctx context.Context, userID string) (Summary, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, 0)

	return s.Repo.GetSummary(ctx, userID, start, end)
}
