package goal

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Goal struct {
	ID               string    `json:"id"`
	UserID           string    `json:"userId"`
	Name             string    `json:"name"`
	Category         string    `json:"category"`
	TargetAmount     float64   `json:"targetAmount"`
	CurrentAmount    float64   `json:"currentAmount"`
	Progress         float64   `json:"progress"`
	LinkedWalletID   *string   `json:"linkedWalletId"`
	LinkedWalletName *string   `json:"linkedWalletName"`
	Deadline         *string   `json:"deadline"`
	Icon             string    `json:"icon"`
	IsEmergency      bool      `json:"isEmergency"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type Contribution struct {
	ID               string    `json:"id"`
	UserID           string    `json:"userId"`
	GoalID           string    `json:"goalId"`
	WalletID         string    `json:"walletId"`
	WalletName       string    `json:"walletName"`
	Amount           float64   `json:"amount"`
	Note             string    `json:"note"`
	ContributionDate string    `json:"contributionDate"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Repository interface {
	ListGoals(ctx context.Context, userID string) ([]Goal, error)
	CreateGoal(ctx context.Context, userID string, req SaveGoalRequest) (Goal, error)
	UpdateGoal(ctx context.Context, userID, goalID string, req SaveGoalRequest) (Goal, error)
	DeleteGoal(ctx context.Context, userID, goalID string) error
	CreateContribution(ctx context.Context, userID, goalID string, req ContributionRequest) (Contribution, error)
	ListContributions(ctx context.Context, userID, goalID string) ([]Contribution, error)
}

type Service struct {
	Repo Repository
}

type SaveGoalRequest struct {
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	TargetAmount   float64 `json:"targetAmount"`
	CurrentAmount  float64 `json:"currentAmount"`
	LinkedWalletID *string `json:"linkedWalletId"`
	Deadline       *string `json:"deadline"`
	Icon           string  `json:"icon"`
	IsEmergency    bool    `json:"isEmergency"`
}

type ContributionRequest struct {
	WalletID         string  `json:"walletId"`
	Amount           float64 `json:"amount"`
	Note             string  `json:"note"`
	ContributionDate string  `json:"contributionDate"`
}

func (s Service) List(ctx context.Context, userID string) ([]Goal, error) {
	return s.Repo.ListGoals(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveGoalRequest) (Goal, error) {
	normalized, err := normalizeGoal(req)
	if err != nil {
		return Goal{}, err
	}
	return s.Repo.CreateGoal(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, goalID string, req SaveGoalRequest) (Goal, error) {
	if strings.TrimSpace(goalID) == "" {
		return Goal{}, errors.New("goal id is required")
	}
	normalized, err := normalizeGoal(req)
	if err != nil {
		return Goal{}, err
	}
	return s.Repo.UpdateGoal(ctx, userID, goalID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, goalID string) error {
	if strings.TrimSpace(goalID) == "" {
		return errors.New("goal id is required")
	}
	return s.Repo.DeleteGoal(ctx, userID, goalID)
}

func (s Service) Contribute(ctx context.Context, userID, goalID string, req ContributionRequest) (Contribution, error) {
	if strings.TrimSpace(goalID) == "" {
		return Contribution{}, errors.New("goal id is required")
	}
	normalized, err := normalizeContribution(req)
	if err != nil {
		return Contribution{}, err
	}
	return s.Repo.CreateContribution(ctx, userID, goalID, normalized)
}

func (s Service) Contributions(ctx context.Context, userID, goalID string) ([]Contribution, error) {
	if strings.TrimSpace(goalID) == "" {
		return nil, errors.New("goal id is required")
	}
	return s.Repo.ListContributions(ctx, userID, goalID)
}

func normalizeGoal(req SaveGoalRequest) (SaveGoalRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Icon = strings.TrimSpace(req.Icon)
	if req.LinkedWalletID != nil {
		value := strings.TrimSpace(*req.LinkedWalletID)
		if value == "" {
			req.LinkedWalletID = nil
		} else {
			req.LinkedWalletID = &value
		}
	}
	if req.Category == "" {
		req.Category = "Savings"
	}
	if req.Name == "" {
		return SaveGoalRequest{}, errors.New("goal name is required")
	}
	if req.TargetAmount <= 0 {
		return SaveGoalRequest{}, errors.New("target amount must be greater than 0")
	}
	if req.CurrentAmount < 0 {
		return SaveGoalRequest{}, errors.New("current amount cannot be negative")
	}
	if req.Deadline != nil {
		value := strings.TrimSpace(*req.Deadline)
		if value == "" {
			req.Deadline = nil
		} else if _, err := time.Parse("2006-01-02", value); err != nil {
			return SaveGoalRequest{}, errors.New("deadline must be YYYY-MM-DD")
		} else {
			req.Deadline = &value
		}
	}
	return req, nil
}

func normalizeContribution(req ContributionRequest) (ContributionRequest, error) {
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.Note = strings.TrimSpace(req.Note)
	req.ContributionDate = strings.TrimSpace(req.ContributionDate)
	if req.WalletID == "" {
		return ContributionRequest{}, errors.New("wallet is required")
	}
	if req.Amount <= 0 {
		return ContributionRequest{}, errors.New("amount must be greater than 0")
	}
	if req.ContributionDate == "" {
		req.ContributionDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", req.ContributionDate); err != nil {
		return ContributionRequest{}, errors.New("contribution date must be YYYY-MM-DD")
	}
	return req, nil
}
