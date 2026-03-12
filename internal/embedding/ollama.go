package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

type embedRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding"`
}

func NewClient(baseURL, model string) *Client {
	return &Client{
		baseURL: baseURL,
		model:   model,
		http: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}

	body, err := json.Marshal(embedRequest{
		Model: c.model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call embedding api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("embedding api status=%d body=%s", resp.StatusCode, string(b))
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(out.Embedding) > 0 {
		return out.Embedding, nil
	}
	if len(out.Embeddings) == 0 || len(out.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("empty embedding vector")
	}
	return out.Embeddings[0], nil
}
