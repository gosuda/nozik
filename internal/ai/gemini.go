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

// GeminiStrategy uses the Google Generative Language API.
type GeminiStrategy struct {
	APIKey string
	Model  string
	Client *http.Client
}

// NewGeminiStrategy creates a Gemini strategy. If apiKey is empty it falls back to GEMINI_API_KEY env var.
func NewGeminiStrategy(apiKey string) *GeminiStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	return &GeminiStrategy{
		APIKey: apiKey,
		Model:  "gemini-2.0-flash",
		Client: http.DefaultClient,
	}
}

func (g *GeminiStrategy) Name() string { return "Gemini" }

func (g *GeminiStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}
	prompt := BuildPrompt(req)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.Model, g.APIKey)
	body := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"temperature": 0.7,
			"responseMimeType": "application/json",
		},
	}
	raw, _ := json.Marshal(body)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini http %d: %s", resp.StatusCode, string(b))
	}
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}
	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates returned")
	}
	text := ""
	for _, p := range geminiResp.Candidates[0].Content.Parts {
		text += p.Text
	}
	return ParseRecommendationJSON(text)
}
