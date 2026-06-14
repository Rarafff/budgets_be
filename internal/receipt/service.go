package receipt

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"budgets_be/internal/openrouter"
)

type ImageParser interface {
	GenerateReceiptFromImage(ctx context.Context, prompt, dataURL string) (openrouter.GenerateResponse, error)
}

type Service struct {
	Parser ImageParser
}

type ReceiptItem struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

type SuggestedTransaction struct {
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Note            string  `json:"note"`
	Amount          float64 `json:"amount"`
	TransactionDate string  `json:"transactionDate"`
}

type ParseResponse struct {
	Model                string               `json:"model"`
	Merchant             string               `json:"merchant"`
	Date                 string               `json:"date"`
	Total                float64              `json:"total"`
	Category             string               `json:"category"`
	Confidence           string               `json:"confidence"`
	Items                []ReceiptItem        `json:"items"`
	Discounts            []ReceiptItem        `json:"discounts"`
	SuggestedTransaction SuggestedTransaction `json:"suggestedTransaction"`
}

func (s Service) ParseImage(ctx context.Context, dataURL string) (ParseResponse, error) {
	if s.Parser == nil {
		return ParseResponse{}, errors.New("receipt parser is not configured")
	}

	response, err := s.Parser.GenerateReceiptFromImage(ctx, receiptPrompt(), dataURL)
	if err != nil {
		return ParseResponse{}, err
	}

	var parsed ParseResponse
	if err := json.Unmarshal([]byte(extractJSONObject(response.Content)), &parsed); err != nil {
		return ParseResponse{}, err
	}

	parsed.Model = response.Model
	parsed.SuggestedTransaction.Type = "expense"
	if parsed.SuggestedTransaction.Amount == 0 {
		parsed.SuggestedTransaction.Amount = parsed.Total
	}
	if parsed.SuggestedTransaction.Title == "" {
		parsed.SuggestedTransaction.Title = parsed.Merchant
	}
	if parsed.SuggestedTransaction.Category == "" {
		parsed.SuggestedTransaction.Category = parsed.Category
	}
	if parsed.SuggestedTransaction.TransactionDate == "" {
		parsed.SuggestedTransaction.TransactionDate = parsed.Date
	}

	return parsed, nil
}

func receiptPrompt() string {
	return strings.TrimSpace(`Read this receipt image and return JSON only.
Use Indonesian Rupiah numeric values without currency symbols.
If a field is uncertain, use an empty string or 0 and set confidence to low.
Choose one broad category such as Food, Snack, Transport, Groceries, Lifestyle, Utilities, Health, Entertainment, or Uncategorized.
Return exactly this JSON shape:
{
  "merchant": "store or merchant name",
  "date": "YYYY-MM-DD",
  "total": 0,
  "category": "category",
  "confidence": "low|medium|high",
  "items": [{"name":"item name","category":"category","amount":0}],
  "discounts": [{"name":"discount name","category":"Discount","amount":0}],
  "suggestedTransaction": {
    "type": "expense",
    "title": "merchant or receipt title",
    "category": "category",
    "note": "short note",
    "amount": 0,
    "transactionDate": "YYYY-MM-DD"
  }
}`)
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end >= start {
		return content[start : end+1]
	}
	return content
}
