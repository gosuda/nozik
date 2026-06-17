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

// GrokStrategy uses the xAI Grok API (OpenAI-compatible).
type GrokStrategy struct {
	APIKey   string
	Model    string
	Endpoint string
	Client   *http.Client
}

// NewGrokStrategy creates a Grok strategy. If apiKey is empty it falls back to GROK_API_KEY env var.
func NewGrokStrategy(apiKey string) *GrokStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("GROK_API_KEY")
	}
	return &GrokStrategy{
		APIKey:   apiKey,
		Model:    "grok-2-latest",
		Endpoint: "https://api.x.ai/v1/chat/completions",
		Client:   http.DefaultClient,
	}
}

func (g *GrokStrategy) Name() string { return "Grok" }

func (g *GrokStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("GROK_API_KEY not set")
	}
	prompt := BuildPrompt(req)
	body := map[string]any{
		"model": g.Model,
		"messages": []map[string]any{
			{"role": "system", "content": "You are Nozik, a helpful music recommendation assistant."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	raw, _ := json.Marshal(body)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+g.APIKey)
	resp, err := g.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok http %d: %s", resp.StatusCode, string(b))
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
