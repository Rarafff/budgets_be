package couple

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Couple struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"inviteCode"`
	CreatedBy  string    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Summary struct {
	CoupleID        string  `json:"coupleId"`
	CoupleName      string  `json:"coupleName"`
	BudgetLimit     float64 `json:"budgetLimit"`
	BudgetSpent     float64 `json:"budgetSpent"`
	BudgetRemaining float64 `json:"budgetRemaining"`
	MySpending      float64 `json:"mySpending"`
	PartnerSpending float64 `json:"partnerSpending"`
}

type Repository interface {
	GetMyCouple(ctx context.Context, userID string) (Couple, error)
	CreateCouple(ctx context.Context, userID string, req SaveCoupleRequest) (Couple, error)
	JoinCouple(ctx context.Context, userID, inviteCode string) (Couple, error)
	GetSummary(ctx context.Context, userID, periodMonth string) (Summary, error)
}

type Service struct {
	Repo Repository
}

type SaveCoupleRequest struct {
	Name string `json:"name"`
}

type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

func (s Service) Me(ctx context.Context, userID string) (Couple, error) {
	return s.Repo.GetMyCouple(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveCoupleRequest) (Couple, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Couple{}, errors.New("couple name is required")
	}
	return s.Repo.CreateCouple(ctx, userID, req)
}

func (s Service) Join(ctx context.Context, userID string, req JoinRequest) (Couple, error) {
	code := strings.ToUpper(strings.TrimSpace(req.InviteCode))
	if code == "" {
		return Couple{}, errors.New("invite code is required")
	}
	return s.Repo.JoinCouple(ctx, userID, code)
}

func (s Service) Summary(ctx context.Context, userID, periodMonth string) (Summary, error) {
	periodMonth = strings.TrimSpace(periodMonth)
	if periodMonth == "" {
		periodMonth = time.Now().Format("2006-01")
	}
	if _, err := time.Parse("2006-01", periodMonth); err != nil {
		return Summary{}, errors.New("period month must be YYYY-MM")
	}
	return s.Repo.GetSummary(ctx, userID, periodMonth)
}
