package asset

import (
	"context"
	"errors"
	"strings"
	"time"
)

var validAssetTypes = map[string]bool{
	"Liquid Asset": true,
	"Fixed Asset":  true,
}

type Asset struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	AssetType     string    `json:"assetType"`
	Category      string    `json:"category"`
	Name          string    `json:"name"`
	Quantity      float64   `json:"quantity"`
	Unit          string    `json:"unit"`
	PurchasePrice float64   `json:"purchasePrice"`
	CurrentPrice  float64   `json:"currentPrice"`
	CurrentValue  float64   `json:"currentValue"`
	CostBasis     float64   `json:"costBasis"`
	GainLoss      float64   `json:"gainLoss"`
	GainLossPct   float64   `json:"gainLossPct"`
	Note          string    `json:"note"`
	AcquiredAt    *string   `json:"acquiredAt"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Repository interface {
	ListAssets(ctx context.Context, userID string) ([]Asset, error)
	CreateAsset(ctx context.Context, userID string, req SaveAssetRequest) (Asset, error)
	UpdateAsset(ctx context.Context, userID, assetID string, req SaveAssetRequest) (Asset, error)
	DeleteAsset(ctx context.Context, userID, assetID string) error
}

type Service struct {
	Repo Repository
}

type SaveAssetRequest struct {
	AssetType     string  `json:"assetType"`
	Category      string  `json:"category"`
	Name          string  `json:"name"`
	Quantity      float64 `json:"quantity"`
	Unit          string  `json:"unit"`
	PurchasePrice float64 `json:"purchasePrice"`
	CurrentPrice  float64 `json:"currentPrice"`
	CurrentValue  float64 `json:"currentValue"`
	Note          string  `json:"note"`
	AcquiredAt    *string `json:"acquiredAt"`
}

func (s Service) List(ctx context.Context, userID string) ([]Asset, error) {
	return s.Repo.ListAssets(ctx, userID)
}

func (s Service) Create(ctx context.Context, userID string, req SaveAssetRequest) (Asset, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Asset{}, err
	}
	return s.Repo.CreateAsset(ctx, userID, normalized)
}

func (s Service) Update(ctx context.Context, userID, assetID string, req SaveAssetRequest) (Asset, error) {
	if strings.TrimSpace(assetID) == "" {
		return Asset{}, errors.New("asset id is required")
	}

	normalized, err := normalizeRequest(req)
	if err != nil {
		return Asset{}, err
	}
	return s.Repo.UpdateAsset(ctx, userID, assetID, normalized)
}

func (s Service) Delete(ctx context.Context, userID, assetID string) error {
	if strings.TrimSpace(assetID) == "" {
		return errors.New("asset id is required")
	}
	return s.Repo.DeleteAsset(ctx, userID, assetID)
}

func normalizeRequest(req SaveAssetRequest) (SaveAssetRequest, error) {
	req.AssetType = strings.TrimSpace(req.AssetType)
	req.Category = strings.TrimSpace(req.Category)
	req.Name = strings.TrimSpace(req.Name)
	req.Unit = strings.TrimSpace(req.Unit)
	req.Note = strings.TrimSpace(req.Note)

	if req.AssetType == "" {
		req.AssetType = "Liquid Asset"
	}
	if !validAssetTypes[req.AssetType] {
		return SaveAssetRequest{}, errors.New("asset type is invalid")
	}
	if req.Category == "" {
		return SaveAssetRequest{}, errors.New("category is required")
	}
	if req.Name == "" {
		return SaveAssetRequest{}, errors.New("asset name is required")
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	if req.PurchasePrice < 0 {
		return SaveAssetRequest{}, errors.New("purchase price cannot be negative")
	}
	if req.CurrentPrice < 0 {
		return SaveAssetRequest{}, errors.New("current price cannot be negative")
	}
	if req.CurrentValue < 0 {
		return SaveAssetRequest{}, errors.New("current value cannot be negative")
	}
	if req.CurrentValue == 0 && req.CurrentPrice > 0 {
		req.CurrentValue = req.Quantity * req.CurrentPrice
	}
	if req.CurrentPrice == 0 && req.CurrentValue > 0 {
		req.CurrentPrice = req.CurrentValue / req.Quantity
	}
	if req.AcquiredAt != nil {
		value := strings.TrimSpace(*req.AcquiredAt)
		if value == "" {
			req.AcquiredAt = nil
		} else if _, err := time.Parse("2006-01-02", value); err != nil {
			return SaveAssetRequest{}, errors.New("acquired date must be YYYY-MM-DD")
		} else {
			req.AcquiredAt = &value
		}
	}

	return req, nil
}
