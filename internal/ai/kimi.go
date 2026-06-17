package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gg582/nozik/internal/model"
)

// KimiStrategy uses the Moonshot AI API (Kimi).
type KimiStrategy struct {
	APIKey string
	Model  string
	Client *http.Client
}

// NewKimiStrategy creates a Kimi strategy. If apiKey is empty it falls back to KIMI_API_KEY env var.
func NewKimiStrategy(apiKey string) *KimiStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("KIMI_API_KEY")
	}
	return &KimiStrategy{
		APIKey: apiKey,
		Model:  "moonshot-v1-8k",
		Client: http.DefaultClient,
	}
}

func (k *KimiStrategy) Name() string { return "Kimi" }

func (k *KimiStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if k.APIKey == "" {
		return nil, fmt.Errorf("KIMI_API_KEY not set")
	}
	prompt := BuildPrompt(req)
	body := map[string]any{
		"model": k.Model,
		"messages": []map[string]any{
			{"role": "system", "content": "You are Nozik, a helpful music recommendation assistant."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	raw, _ := json.Marshal(body)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.moonshot.cn/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+k.APIKey)
	resp, err := k.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kimi http %d: %s", resp.StatusCode, string(b))
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned")
	}
	return ParseRecommendationJSON(chatResp.Choices[0].Message.Content)
}
