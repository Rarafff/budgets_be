package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const chatCompletionsURL = "https://openrouter.ai/api/v1/chat/completions"

type Config struct {
	APIKey        string
	PrimaryModel  string
	FallbackModel string
	CheapModel    string
	ReceiptModel  string
	HTTPClient    *http.Client
}

type Client struct {
	apiKey        string
	primaryModel  string
	fallbackModel string
	cheapModel    string
	receiptModel  string
	httpClient    *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GenerateRequest struct {
	Messages    []Message `json:"messages"`
	UseCheap    bool      `json:"useCheap"`
	Temperature float64   `json:"temperature"`
}

type GenerateResponse struct {
	Model   string `json:"model"`
	Content string `json:"content"`
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		apiKey:        cfg.APIKey,
		primaryModel:  cfg.PrimaryModel,
		fallbackModel: cfg.FallbackModel,
		cheapModel:    cfg.CheapModel,
		receiptModel:  cfg.ReceiptModel,
		httpClient:    httpClient,
	}
}

func (c *Client) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	models := c.modelOrder(req.UseCheap)
	var lastErr error

	for _, model := range models {
		response, err := c.generateWithModel(ctx, model, req)
		if err == nil {
			return response, nil
		}
		lastErr = err
	}

	return GenerateResponse{}, lastErr
}

func (c *Client) modelOrder(useCheap bool) []string {
	if useCheap && c.cheapModel != "" {
		return uniqueNonEmpty(c.cheapModel, c.primaryModel, c.fallbackModel)
	}
	return uniqueNonEmpty(c.primaryModel, c.fallbackModel)
}

func (c *Client) generateWithModel(ctx context.Context, model string, req GenerateRequest) (GenerateResponse, error) {
	payload := map[string]any{
		"model":       model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return GenerateResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, bytes.NewReader(body))
	if err != nil {
		return GenerateResponse{}, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost")
	httpReq.Header.Set("X-Title", "Budgets AI Advisor")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return GenerateResponse{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return GenerateResponse{}, fmt.Errorf("openrouter %s: %s", httpResp.Status, strings.TrimSpace(string(respBody)))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return GenerateResponse{}, err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return GenerateResponse{}, errors.New("openrouter returned an empty response")
	}

	return GenerateResponse{
		Model:   model,
		Content: decoded.Choices[0].Message.Content,
	}, nil
}

func (c *Client) GenerateReceiptFromImage(ctx context.Context, prompt, dataURL string) (GenerateResponse, error) {
	model := strings.TrimSpace(c.receiptModel)
	if model == "" {
		return GenerateResponse{}, errors.New("receipt model is not configured")
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": dataURL,
						},
					},
				},
			},
		},
		"temperature": 0,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return GenerateResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, bytes.NewReader(body))
	if err != nil {
		return GenerateResponse{}, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "http://localhost")
	httpReq.Header.Set("X-Title", "Budgets Receipt Scanner")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return GenerateResponse{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return GenerateResponse{}, fmt.Errorf("openrouter %s: %s", httpResp.Status, strings.TrimSpace(string(respBody)))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return GenerateResponse{}, err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return GenerateResponse{}, errors.New("openrouter returned an empty response")
	}

	return GenerateResponse{
		Model:   model,
		Content: decoded.Choices[0].Message.Content,
	}, nil
}

type chatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
