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

// OpenAIStrategy uses OpenAI-compatible chat completions.
type OpenAIStrategy struct {
	APIKey   string
	Model    string
	Endpoint string
	Client   *http.Client
}

// NewOpenAIStrategy creates an OpenAI strategy. If apiKey is empty it falls back to OPENAI_API_KEY env var.
func NewOpenAIStrategy(apiKey string) *OpenAIStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	return &OpenAIStrategy{
		APIKey:   apiKey,
		Model:    "gpt-4o-mini",
		Endpoint: "https://api.openai.com/v1/chat/completions",
		Client:   http.DefaultClient,
	}
}

func (o *OpenAIStrategy) Name() string { return "ChatGPT" }

func (o *OpenAIStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if o.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	return o.call(ctx, req)
}

func (o *OpenAIStrategy) call(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	prompt := BuildPrompt(req)
	body := map[string]any{
		"model": o.Model,
		"messages": []map[string]any{
			{"role": "system", "content": "You are Nozik, a helpful music recommendation assistant."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	raw, _ := json.Marshal(body)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := o.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai http %d: %s", resp.StatusCode, string(b))
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
