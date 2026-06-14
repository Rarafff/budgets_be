package wallet

import (
	"context"
	"errors"
	"strings"
	"time"
)

var validWalletTypes = map[string]bool{
	"Bank":        true,
	"E-Wallet":    true,
	"Cash":        true,
	"Credit Card": true,
	"Paylater":    true,
}

type Wallet struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Currency      string    `json:"currency"`
	Balance       float64   `json:"balance"`
	AccountNumber string    `json:"accountNumber"`
	CreditLimit   float64   `json:"creditLimit"`
	DueDay        *int      `json:"dueDay"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Repository interface {
	ListWallets(ctx context.Context, userID string) ([]Wallet, error)
	CreateWallet(ctx context.Context, userID string, req SaveWalletRequest) (Wallet, error)
	UpdateWallet(ctx context.Context, userID, walletID string, req SaveWalletRequest) (Wallet, error)
	DeleteWallet(ctx context.Context, userID, walletID string) error
}

type Service struct {
	Repo Repository
}

type SaveWalletRequest struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
	AccountNumber string  `json:"accountNumber"`
	CreditLimit   float64 `json:"creditLimit"`
	DueDay        *int    `json:"dueDay"`
}

func (s Service) List(ctx context.Context, userID string) ([]Wallet, error) {
	return s.Repo.ListWallets(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveWalletRequest) (Wallet, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Wallet{}, err
	}
	return s.Repo.CreateWallet(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, walletID string, req SaveWalletRequest) (Wallet, error) {
	if strings.TrimSpace(walletID) == "" {
		return Wallet{}, errors.New("wallet id is required")
	}

	normalized, err := normalizeRequest(req)
	if err != nil {
		return Wallet{}, err
	}
	return s.Repo.UpdateWallet(ctx, userID, walletID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, walletID string) error {
	if strings.TrimSpace(walletID) == "" {
		return errors.New("wallet id is required")
	}
	return s.Repo.DeleteWallet(ctx, userID, walletID)
}

func normalizeRequest(req SaveWalletRequest) (SaveWalletRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.TrimSpace(req.Type)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)

	if req.Name == "" {
		return SaveWalletRequest{}, errors.New("wallet name is required")
	}
	if !validWalletTypes[req.Type] {
		return SaveWalletRequest{}, errors.New("wallet type is invalid")
	}
	if req.Currency == "" {
		req.Currency = "IDR"
	}
	if req.DueDay != nil && (*req.DueDay < 1 || *req.DueDay > 31) {
		return SaveWalletRequest{}, errors.New("due day must be between 1 and 31")
	}
	if req.Balance < 0 {
		return SaveWalletRequest{}, errors.New("balance cannot be negative")
	}
	if req.CreditLimit < 0 {
		return SaveWalletRequest{}, errors.New("credit limit cannot be negative")
	}

	if !isLiability(req.Type) {
		req.CreditLimit = 0
		req.DueDay = nil
	}

	return req, nil
}

func isLiability(walletType string) bool {
	return walletType == "Credit Card" || walletType == "Paylater"
}
