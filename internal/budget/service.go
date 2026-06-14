package budget

import (
	"context"
	"errors"
	"strings"
	"time"
)

var validGroups = map[string]bool{
	"Needs": true,
	"Wants": true,
}

type Budget struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Scope       string    `json:"scope"`
	CoupleID    *string   `json:"coupleId"`
	GroupName   string    `json:"groupName"`
	Category    string    `json:"category"`
	PeriodMonth string    `json:"periodMonth"`
	LimitAmount float64   `json:"limitAmount"`
	SpentAmount float64   `json:"spentAmount"`
	Progress    float64   `json:"progress"`
	Icon        string    `json:"icon"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Repository interface {
	ListBudgets(ctx context.Context, userID, periodMonth, scope, coupleID string) ([]Budget, error)
	CreateBudget(ctx context.Context, userID string, req SaveBudgetRequest) (Budget, error)
	UpdateBudget(ctx context.Context, userID, budgetID string, req SaveBudgetRequest) (Budget, error)
	DeleteBudget(ctx context.Context, userID, budgetID string) error
}

type Service struct {
	Repo Repository
}

type SaveBudgetRequest struct {
	Scope       string  `json:"scope"`
	CoupleID    string  `json:"coupleId"`
	GroupName   string  `json:"groupName"`
	Category    string  `json:"category"`
	PeriodMonth string  `json:"periodMonth"`
	LimitAmount float64 `json:"limitAmount"`
	Icon        string  `json:"icon"`
}

func (s Service) List(ctx context.Context, userID, periodMonth, scope, coupleID string) ([]Budget, error) {
	periodMonth = normalizePeriodMonth(periodMonth)
	scope = strings.ToLower(strings.TrimSpace(scope))
	coupleID = strings.TrimSpace(coupleID)
	if scope == "" {
		scope = "personal"
	}
	if scope == "personal" {
		coupleID = ""
	}
	if scope != "personal" && scope != "couple" {
		return nil, errors.New("budget scope is invalid")
	}
	if scope == "couple" && coupleID == "" {
		return nil, errors.New("couple id is required")
	}
	return s.Repo.ListBudgets(ctx, userID, periodMonth, scope, coupleID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveBudgetRequest) (Budget, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Budget{}, err
	}
	return s.Repo.CreateBudget(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, budgetID string, req SaveBudgetRequest) (Budget, error) {
	if strings.TrimSpace(budgetID) == "" {
		return Budget{}, errors.New("budget id is required")
	}

	normalized, err := normalizeRequest(req)
	if err != nil {
		return Budget{}, err
	}
	return s.Repo.UpdateBudget(ctx, userID, budgetID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, budgetID string) error {
	if strings.TrimSpace(budgetID) == "" {
		return errors.New("budget id is required")
	}
	return s.Repo.DeleteBudget(ctx, userID, budgetID)
}

func normalizeRequest(req SaveBudgetRequest) (SaveBudgetRequest, error) {
	req.GroupName = strings.TrimSpace(req.GroupName)
	req.Scope = strings.ToLower(strings.TrimSpace(req.Scope))
	req.CoupleID = strings.TrimSpace(req.CoupleID)
	req.Category = strings.TrimSpace(req.Category)
	req.PeriodMonth = normalizePeriodMonth(req.PeriodMonth)
	req.Icon = strings.TrimSpace(req.Icon)

	if req.Scope == "" {
		req.Scope = "personal"
	}
	if req.Scope != "personal" && req.Scope != "couple" {
		return SaveBudgetRequest{}, errors.New("budget scope is invalid")
	}
	if req.Scope == "personal" {
		req.CoupleID = ""
	}
	if req.Scope == "couple" && req.CoupleID == "" {
		return SaveBudgetRequest{}, errors.New("couple id is required")
	}
	if req.GroupName == "" {
		req.GroupName = "Needs"
	}
	if !validGroups[req.GroupName] {
		return SaveBudgetRequest{}, errors.New("budget group is invalid; use Needs or Wants")
	}
	if req.Category == "" {
		return SaveBudgetRequest{}, errors.New("category is required")
	}
	if req.PeriodMonth == "" {
		return SaveBudgetRequest{}, errors.New("period month is required")
	}
	if _, err := time.Parse("2006-01", req.PeriodMonth); err != nil {
		return SaveBudgetRequest{}, errors.New("period month must be YYYY-MM")
	}
	if req.LimitAmount <= 0 {
		return SaveBudgetRequest{}, errors.New("limit amount must be greater than 0")
	}

	return req, nil
}

func normalizePeriodMonth(periodMonth string) string {
	periodMonth = strings.TrimSpace(periodMonth)
	if periodMonth != "" {
		return periodMonth
	}
	return time.Now().Format("2006-01")
}
