package category

import (
	"context"
	"errors"
	"strings"
	"time"
)

var validTypes = map[string]bool{"expense": true, "income": true}

type Category struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Icon      string    `json:"icon"`
	CreatedAt time.Time `json:"createdAt"`
}

type SaveRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Icon string `json:"icon"`
}

type Repository interface {
	List(ctx context.Context, userID string) ([]Category, error)
	Create(ctx context.Context, userID string, req SaveRequest) (Category, error)
	Delete(ctx context.Context, userID, categoryID string) error
}

type Service struct{ Repo Repository }

func (s Service) List(ctx context.Context, userID string) ([]Category, error) {
	return s.Repo.List(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveRequest) (Category, error) {
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.Icon = strings.TrimSpace(req.Icon)
	if req.Name == "" {
		return Category{}, errors.New("category name is required")
	}
	if len([]rune(req.Name)) > 60 {
		return Category{}, errors.New("category name must be 60 characters or fewer")
	}
	if !validTypes[req.Type] {
		return Category{}, errors.New("category type is invalid")
	}
	if len([]rune(req.Icon)) > 80 {
		return Category{}, errors.New("category icon is invalid")
	}
	return s.Repo.Create(ctx, userID, req)
}

func (s Service) Delete(ctx context.Context, userID, categoryID string) error {
	if strings.TrimSpace(categoryID) == "" {
		return errors.New("category id is required")
	}
	return s.Repo.Delete(ctx, userID, categoryID)
}
