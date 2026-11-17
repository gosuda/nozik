package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gg582/nozik/internal/model"
)

// GeminiStrategy uses the Google Generative Language API.
type GeminiStrategy struct {
	APIKey string
	Model  string
	Client *http.Client
}

// NewGeminiStrategy creates a Gemini strategy. If apiKey is empty it falls back to GEMINI_API_KEY or GOOGLE_API_KEY.
// If model is empty it falls back to the default Gemini model.
func NewGeminiStrategy(apiKey, model string) *GeminiStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiStrategy{
		APIKey: apiKey,
		Model:  model,
		Client: http.DefaultClient,
	}
}

func (g *GeminiStrategy) Name() string { return "Gemini" }

func (g *GeminiStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY not set")
	}
	prompt := BuildPrompt(req)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.Model, g.APIKey)
	body := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"temperature":      0.7,
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

// ListModels returns the list of models available to the configured API key.
func (g *GeminiStrategy) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY not set")
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", g.APIKey)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini list models http %d: %s", resp.StatusCode, string(b))
	}
	var geminiResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}
	out := make([]model.ModelInfo, 0, len(geminiResp.Models))
	for _, m := range geminiResp.Models {
		out = append(out, model.ModelInfo{ID: strings.TrimPrefix(m.Name, "models/")})
	}
	return out, nil
}
