package bill

import (
	"context"
	"errors"
	"strings"
	"time"
)

var validStatuses = map[string]bool{
	"upcoming": true,
	"paid":     true,
	"overdue":  true,
}

var validRepeatIntervals = map[string]bool{
	"":        true,
	"monthly": true,
}

type Bill struct {
	ID                string     `json:"id"`
	UserID            string     `json:"userId"`
	WalletID          *string    `json:"walletId"`
	WalletName        *string    `json:"walletName"`
	Name              string     `json:"name"`
	Category          string     `json:"category"`
	Provider          string     `json:"provider"`
	Amount            float64    `json:"amount"`
	DueDate           string     `json:"dueDate"`
	Status            string     `json:"status"`
	Note              string     `json:"note"`
	IsRecurring       bool       `json:"isRecurring"`
	RepeatInterval    string     `json:"repeatInterval"`
	PaidTransactionID *string    `json:"paidTransactionId"`
	PaidAt            *time.Time `json:"paidAt"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type Repository interface {
	ListBills(ctx context.Context, userID string) ([]Bill, error)
	CreateBill(ctx context.Context, userID string, req SaveBillRequest) (Bill, error)
	UpdateBill(ctx context.Context, userID, billID string, req SaveBillRequest) (Bill, error)
	DeleteBill(ctx context.Context, userID, billID string) error
	PayBill(ctx context.Context, userID, billID string, req PayBillRequest) (Bill, error)
	BudgetCategoryExists(ctx context.Context, userID, category, periodMonth string) (bool, error)
}

type Service struct {
	Repo Repository
}

type SaveBillRequest struct {
	WalletID       string  `json:"walletId"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Provider       string  `json:"provider"`
	Amount         float64 `json:"amount"`
	DueDate        string  `json:"dueDate"`
	Status         string  `json:"status"`
	Note           string  `json:"note"`
	IsRecurring    bool    `json:"isRecurring"`
	RepeatInterval string  `json:"repeatInterval"`
}

type PayBillRequest struct {
	WalletID    string `json:"walletId"`
	PaymentDate string `json:"paymentDate"`
}

func (s Service) List(ctx context.Context, userID string) ([]Bill, error) {
	return s.Repo.ListBills(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveBillRequest) (Bill, error) {
	normalized, err := normalizeSave(req)
	if err != nil {
		return Bill{}, err
	}
	return s.Repo.CreateBill(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, billID string, req SaveBillRequest) (Bill, error) {
	if strings.TrimSpace(billID) == "" {
		return Bill{}, errors.New("bill id is required")
	}
	normalized, err := normalizeSave(req)
	if err != nil {
		return Bill{}, err
	}
	return s.Repo.UpdateBill(ctx, userID, billID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, billID string) error {
	if strings.TrimSpace(billID) == "" {
		return errors.New("bill id is required")
	}
	return s.Repo.DeleteBill(ctx, userID, billID)
}

func (s Service) Pay(ctx context.Context, userID, billID string, req PayBillRequest) (Bill, error) {
	if strings.TrimSpace(billID) == "" {
		return Bill{}, errors.New("bill id is required")
	}
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.PaymentDate = strings.TrimSpace(req.PaymentDate)
	if req.WalletID == "" {
		return Bill{}, errors.New("wallet is required")
	}
	if req.PaymentDate == "" {
		req.PaymentDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", req.PaymentDate); err != nil {
		return Bill{}, errors.New("payment date must be YYYY-MM-DD")
	}
	return s.Repo.PayBill(ctx, userID, billID, req)
}

func normalizeSave(req SaveBillRequest) (SaveBillRequest, error) {
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Provider = strings.TrimSpace(req.Provider)
	req.DueDate = strings.TrimSpace(req.DueDate)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.Note = strings.TrimSpace(req.Note)
	req.RepeatInterval = strings.ToLower(strings.TrimSpace(req.RepeatInterval))

	if req.Name == "" {
		return SaveBillRequest{}, errors.New("bill name is required")
	}
	if req.Category == "" {
		return SaveBillRequest{}, errors.New("category is required")
	}
	if req.Amount <= 0 {
		return SaveBillRequest{}, errors.New("amount must be greater than 0")
	}
	if req.DueDate == "" {
		return SaveBillRequest{}, errors.New("due date is required")
	}
	if _, err := time.Parse("2006-01-02", req.DueDate); err != nil {
		return SaveBillRequest{}, errors.New("due date must be YYYY-MM-DD")
	}
	if req.Status == "" {
		req.Status = "upcoming"
	}
	if !validStatuses[req.Status] {
		return SaveBillRequest{}, errors.New("bill status is invalid")
	}
	if req.IsRecurring && req.RepeatInterval == "" {
		req.RepeatInterval = "monthly"
	}
	if !req.IsRecurring {
		req.RepeatInterval = ""
	}
	if !validRepeatIntervals[req.RepeatInterval] {
		return SaveBillRequest{}, errors.New("repeat interval is invalid")
	}

	return req, nil
}
