package transaction

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

var validTransactionTypes = map[string]bool{
	"expense":  true,
	"income":   true,
	"transfer": true,
}

type Transaction struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	Scope           string    `json:"scope"`
	CoupleID        *string   `json:"coupleId"`
	WalletID        string    `json:"walletId"`
	WalletName      string    `json:"walletName"`
	ToWalletID      *string   `json:"toWalletId"`
	ToWalletName    *string   `json:"toWalletName"`
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	Category        string    `json:"category"`
	Note            string    `json:"note"`
	Amount          float64   `json:"amount"`
	TransactionDate string    `json:"transactionDate"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Repository interface {
	ListTransactions(ctx context.Context, userID string) ([]Transaction, error)
	CreateTransaction(ctx context.Context, userID string, req SaveTransactionRequest) (Transaction, error)
	CreateTransactions(ctx context.Context, userID string, reqs []SaveTransactionRequest) ([]Transaction, error)
	UpdateTransaction(ctx context.Context, userID, transactionID string, req SaveTransactionRequest) (Transaction, error)
	DeleteTransaction(ctx context.Context, userID, transactionID string) error
	BudgetCategoryExists(ctx context.Context, userID, category, periodMonth, scope, coupleID string) (bool, error)
}

type Service struct {
	Repo Repository
}

type SaveTransactionRequest struct {
	Scope           string  `json:"scope"`
	CoupleID        string  `json:"coupleId"`
	WalletID        string  `json:"walletId"`
	ToWalletID      *string `json:"toWalletId"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Note            string  `json:"note"`
	Amount          float64 `json:"amount"`
	TransactionDate string  `json:"transactionDate"`
}

func (s Service) List(ctx context.Context, userID string) ([]Transaction, error) {
	return s.Repo.ListTransactions(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveTransactionRequest) (Transaction, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Transaction{}, err
	}
	if err := s.validateBudgetCategory(ctx, userID, normalized); err != nil {
		return Transaction{}, err
	}
	return s.Repo.CreateTransaction(ctx, userID, normalized)
}

func (s Service) CreateBulk(ctx context.Context, userID string, reqs []SaveTransactionRequest) ([]Transaction, error) {
	if len(reqs) == 0 {
		return nil, errors.New("at least one transaction is required")
	}
	if len(reqs) > 100 {
		return nil, errors.New("bulk transaction limit is 100 rows")
	}

	normalized := make([]SaveTransactionRequest, 0, len(reqs))
	for index, req := range reqs {
		normalizedReq, err := normalizeRequest(req)
		if err != nil {
			return nil, errors.New("row " + strconv.Itoa(index+1) + ": " + err.Error())
		}
		if err := s.validateBudgetCategory(ctx, userID, normalizedReq); err != nil {
			return nil, errors.New("row " + strconv.Itoa(index+1) + ": " + err.Error())
		}
		normalized = append(normalized, normalizedReq)
	}

	return s.Repo.CreateTransactions(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, transactionID string, req SaveTransactionRequest) (Transaction, error) {
	if strings.TrimSpace(transactionID) == "" {
		return Transaction{}, errors.New("transaction id is required")
	}

	normalized, err := normalizeRequest(req)
	if err != nil {
		return Transaction{}, err
	}
	if err := s.validateBudgetCategory(ctx, userID, normalized); err != nil {
		return Transaction{}, err
	}
	return s.Repo.UpdateTransaction(ctx, userID, transactionID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, transactionID string) error {
	if strings.TrimSpace(transactionID) == "" {
		return errors.New("transaction id is required")
	}
	return s.Repo.DeleteTransaction(ctx, userID, transactionID)
}

func normalizeRequest(req SaveTransactionRequest) (SaveTransactionRequest, error) {
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.Scope = strings.ToLower(strings.TrimSpace(req.Scope))
	req.CoupleID = strings.TrimSpace(req.CoupleID)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.Title = strings.TrimSpace(req.Title)
	req.Category = strings.TrimSpace(req.Category)
	req.Note = strings.TrimSpace(req.Note)
	req.TransactionDate = strings.TrimSpace(req.TransactionDate)

	if req.Scope == "" {
		req.Scope = "personal"
	}
	if req.Scope != "personal" && req.Scope != "couple" {
		return SaveTransactionRequest{}, errors.New("transaction scope is invalid")
	}
	if req.Scope == "personal" {
		req.CoupleID = ""
	}
	if req.Scope == "couple" && req.CoupleID == "" {
		return SaveTransactionRequest{}, errors.New("couple id is required")
	}
	if req.WalletID == "" {
		return SaveTransactionRequest{}, errors.New("wallet is required")
	}
	if !validTransactionTypes[req.Type] {
		return SaveTransactionRequest{}, errors.New("transaction type is invalid")
	}
	if req.Title == "" {
		return SaveTransactionRequest{}, errors.New("title is required")
	}
	if req.Amount <= 0 {
		return SaveTransactionRequest{}, errors.New("amount must be greater than 0")
	}
	if req.TransactionDate == "" {
		req.TransactionDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", req.TransactionDate); err != nil {
		return SaveTransactionRequest{}, errors.New("transaction date must be YYYY-MM-DD")
	}

	if req.ToWalletID != nil {
		value := strings.TrimSpace(*req.ToWalletID)
		if value == "" {
			req.ToWalletID = nil
		} else {
			req.ToWalletID = &value
		}
	}

	if req.Type == "transfer" {
		if req.ToWalletID == nil {
			return SaveTransactionRequest{}, errors.New("to wallet is required for transfer")
		}
		if *req.ToWalletID == req.WalletID {
			return SaveTransactionRequest{}, errors.New("to wallet must be different from wallet")
		}
		req.Category = "Transfer"
	} else {
		req.ToWalletID = nil
	}
	if req.Type == "expense" && req.Category == "" {
		return SaveTransactionRequest{}, errors.New("category is required for expense")
	}

	return req, nil
}

func (s Service) validateBudgetCategory(ctx context.Context, userID string, req SaveTransactionRequest) error {
	if req.Type != "expense" {
		return nil
	}

	periodMonth := req.TransactionDate[:7]
	exists, err := s.Repo.BudgetCategoryExists(ctx, userID, req.Category, periodMonth, req.Scope, req.CoupleID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("expense category must match a budget category for " + periodMonth)
	}
	return nil
}
