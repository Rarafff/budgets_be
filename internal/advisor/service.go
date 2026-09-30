package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"budgets_be/internal/openrouter"
	"budgets_be/internal/report"
)

type LLM interface {
	Generate(ctx context.Context, req openrouter.GenerateRequest) (openrouter.GenerateResponse, error)
}

type Service struct {
	LLM    LLM
	Report report.Service
	Store  Store
}

type AdviceRequest struct {
	MonthlyIncome float64       `json:"monthlyIncome"`
	Currency      string        `json:"currency"`
	Goals         []string      `json:"goals"`
	Transactions  []Transaction `json:"transactions"`
	Question      string        `json:"question"`
	PeriodMonth   string        `json:"periodMonth"`
	ThreadID      string        `json:"threadId"`
	UseCheapModel bool          `json:"useCheapModel"`
}

type Transaction struct {
	Date     string  `json:"date"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Type     string  `json:"type"`
	Amount   float64 `json:"amount"`
}

type AdviceResponse struct {
	Model       string         `json:"model"`
	Advice      string         `json:"advice"`
	PeriodMonth string         `json:"periodMonth"`
	ThreadID    string         `json:"threadId"`
	ThreadTitle string         `json:"threadTitle"`
	Message     *Message       `json:"message,omitempty"`
	Snapshot    *reportSummary `json:"snapshot,omitempty"`
}

type Thread struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Title       string    `json:"title"`
	PeriodMonth string    `json:"periodMonth"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Message struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store interface {
	ListThreads(ctx context.Context, userID string) ([]Thread, error)
	CreateThread(ctx context.Context, userID, title, periodMonth string) (Thread, error)
	GetThread(ctx context.Context, userID, threadID string) (Thread, error)
	TouchThread(ctx context.Context, userID, threadID string) error
	ListMessages(ctx context.Context, userID, threadID string, limit int) ([]Message, error)
	CreateMessage(ctx context.Context, userID, threadID, role, content, model string) (Message, error)
}

type advisorPayload struct {
	Question      string         `json:"question"`
	Currency      string         `json:"currency"`
	PeriodMonth   string         `json:"periodMonth"`
	Report        *reportSummary `json:"report,omitempty"`
	ManualContext AdviceRequest  `json:"manualContext"`
}

type reportSummary struct {
	Income              float64                    `json:"income"`
	Expense             float64                    `json:"expense"`
	Net                 float64                    `json:"net"`
	Saved               float64                    `json:"saved"`
	SavingsRatio        float64                    `json:"savingsRatio"`
	CashBalance         float64                    `json:"cashBalance"`
	DebtBalance         float64                    `json:"debtBalance"`
	HealthScore         report.HealthScore         `json:"healthScore"`
	TopCategories       []report.CategoryBreakdown `json:"topCategories"`
	BudgetPerformance   []report.BudgetPerformance `json:"budgetPerformance"`
	GoalProgress        []report.GoalProgress      `json:"goalProgress"`
	GoalContributions   float64                    `json:"goalContributions"`
	TransactionCount    int                        `json:"transactionCount"`
	NoSpendDays         int                        `json:"noSpendDays"`
	AverageDailyExpense float64                    `json:"averageDailyExpense"`
	HighestSpendingDay  *report.DailySpending      `json:"highestSpendingDay"`
}

func isCasualGreeting(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	normalized = strings.Trim(normalized, " .,!?:;…")
	_, ok := map[string]struct{}{
		"hi": {}, "hello": {}, "hey": {}, "halo": {}, "hai": {}, "pagi": {}, "siang": {}, "sore": {}, "malam": {},
		"selamat pagi": {}, "selamat siang": {}, "selamat sore": {}, "selamat malam": {}, "hai ai": {}, "halo ai": {}, "ai": {},
	}[normalized]
	return ok
}

func (s Service) GetAdvice(ctx context.Context, userID string, req AdviceRequest) (AdviceResponse, error) {
	if s.LLM == nil {
		return AdviceResponse{}, fmt.Errorf("advisor LLM is not configured")
	}

	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "IDR"
	}
	if strings.TrimSpace(req.PeriodMonth) == "" {
		req.PeriodMonth = time.Now().Format("2006-01")
	}
	if strings.TrimSpace(req.Question) == "" {
		req.Question = "Review my current financial condition and recommend the next actions."
	}

	thread, history, err := s.resolveThread(ctx, userID, req)
	if err != nil {
		return AdviceResponse{}, err
	}

	isGreeting := isCasualGreeting(req.Question)
	var snapshot *reportSummary
	if !isGreeting && strings.TrimSpace(userID) != "" && s.Report.Repo != nil {
		monthly, err := s.Report.Monthly(ctx, userID, req.PeriodMonth)
		if err != nil {
			return AdviceResponse{}, err
		}
		snapshot = summarizeReport(monthly)
		req.PeriodMonth = monthly.Period.Month
	}

	payload := advisorPayload{
		Question:      req.Question,
		Currency:      req.Currency,
		PeriodMonth:   req.PeriodMonth,
		Report:        snapshot,
		ManualContext: req,
	}

	userPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return AdviceResponse{}, err
	}

	messages := []openrouter.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`You are the warm, practical bear money buddy in a personal finance app.
Reply in the same language as the user's latest message. Sound kind, calm, and natural. Be friendly without being overly cute, repetitive, or theatrical.

For a greeting or casual small talk, reply with a short greeting and one helpful question about what the user would like to do. Do not mention their financial data, do not generate a report, and do not use headings or bullet points.

For a financial question, answer the question directly first. Use the report snapshot only when it helps answer that question. Keep the response short unless the user asks for a review or detailed plan. Use bullets only when they make actions easier to scan. Use headings only for a detailed review, never as a mandatory template.

Give practical budgeting guidance from the user's data. Do not claim to be a licensed financial advisor. Do not provide guaranteed investment, loan, tax, or legal advice. When data is missing, say what is missing without guessing. Use the user's currency. Use recent conversation only for follow-up context. Do not invent transactions, balances, goals, or budgets that are not in the provided data.`),
		},
		{
			Role: "user",
			Content: "Financial data snapshot for this question:\n" +
				string(userPayload),
		},
	}

	for _, message := range history {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		messages = append(messages, openrouter.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	messages = append(messages, openrouter.Message{
		Role:    "user",
		Content: req.Question,
	})

	if s.Store != nil {
		if _, err := s.Store.CreateMessage(ctx, userID, thread.ID, "user", req.Question, ""); err != nil {
			return AdviceResponse{}, err
		}
	}

	response, err := s.LLM.Generate(ctx, openrouter.GenerateRequest{
		UseCheap:    req.UseCheapModel,
		Temperature: 0.2,
		Messages:    messages,
	})
	if err != nil {
		return AdviceResponse{}, err
	}

	var assistantMessage *Message
	if s.Store != nil {
		saved, err := s.Store.CreateMessage(ctx, userID, thread.ID, "assistant", response.Content, response.Model)
		if err != nil {
			return AdviceResponse{}, err
		}
		assistantMessage = &saved
		if err := s.Store.TouchThread(ctx, userID, thread.ID); err != nil {
			return AdviceResponse{}, err
		}
	}

	return AdviceResponse{
		Model:       response.Model,
		Advice:      response.Content,
		PeriodMonth: req.PeriodMonth,
		ThreadID:    thread.ID,
		ThreadTitle: thread.Title,
		Message:     assistantMessage,
		Snapshot:    snapshot,
	}, nil
}

func (s Service) Threads(ctx context.Context, userID string) ([]Thread, error) {
	if s.Store == nil {
		return []Thread{}, nil
	}
	return s.Store.ListThreads(ctx, userID)
}

func (s Service) Messages(ctx context.Context, userID, threadID string) ([]Message, error) {
	if s.Store == nil {
		return []Message{}, nil
	}
	if strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("thread id is required")
	}
	if _, err := s.Store.GetThread(ctx, userID, threadID); err != nil {
		return nil, err
	}
	return s.Store.ListMessages(ctx, userID, threadID, 100)
}

func (s Service) resolveThread(ctx context.Context, userID string, req AdviceRequest) (Thread, []Message, error) {
	if s.Store == nil {
		return Thread{}, nil, nil
	}

	threadID := strings.TrimSpace(req.ThreadID)
	if threadID == "" {
		thread, err := s.Store.CreateThread(ctx, userID, titleFromQuestion(req.Question), req.PeriodMonth)
		return thread, nil, err
	}

	thread, err := s.Store.GetThread(ctx, userID, threadID)
	if err != nil {
		return Thread{}, nil, err
	}

	history, err := s.Store.ListMessages(ctx, userID, threadID, 12)
	return thread, history, err
}

func titleFromQuestion(question string) string {
	title := strings.TrimSpace(question)
	if title == "" {
		return "New advisor chat"
	}
	if len(title) > 60 {
		return strings.TrimSpace(title[:57]) + "..."
	}
	return title
}

func summarizeReport(monthly report.MonthlyReport) *reportSummary {
	return &reportSummary{
		Income:              monthly.Income,
		Expense:             monthly.Expense,
		Net:                 monthly.Net,
		Saved:               monthly.Saved,
		SavingsRatio:        monthly.SavingsRatio,
		CashBalance:         monthly.CashBalance,
		DebtBalance:         monthly.DebtBalance,
		HealthScore:         monthly.HealthScore,
		TopCategories:       limitSlice(monthly.TopCategories, 5),
		BudgetPerformance:   limitSlice(monthly.BudgetPerformance, 8),
		GoalProgress:        limitSlice(monthly.GoalProgress, 8),
		GoalContributions:   monthly.GoalContributions,
		TransactionCount:    monthly.TransactionCount,
		NoSpendDays:         monthly.NoSpendDays,
		AverageDailyExpense: monthly.AverageDailyExpense,
		HighestSpendingDay:  monthly.HighestSpendingDay,
	}
}

func limitSlice[T any](items []T, limit int) []T {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
