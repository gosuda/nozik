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
	APIKey   string
	Model    string
	Endpoint string
	Client   *http.Client
}

// NewKimiStrategy creates a Kimi strategy. If apiKey is empty it falls back to KIMI_API_KEY env var.
// If model is empty it falls back to the default Kimi model.
// The endpoint defaults to https://api.moonshot.ai/v1 (international); pass baseURL
// (or set MOONSHOT_BASE_URL) to https://api.moonshot.cn/v1 if you have a China-region API key.
func NewKimiStrategy(apiKey, model, baseURL string) *KimiStrategy {
	if apiKey == "" {
		apiKey = os.Getenv("KIMI_API_KEY")
	}
	if model == "" {
		model = "kimi-k2.5"
	}
	endpoint := baseURL
	if endpoint == "" {
		endpoint = os.Getenv("MOONSHOT_BASE_URL")
	}
	if endpoint == "" {
		endpoint = "https://api.moonshot.ai/v1"
	}
	return &KimiStrategy{
		APIKey:   apiKey,
		Model:    model,
		Endpoint: endpoint,
		Client:   http.DefaultClient,
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
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, k.Endpoint+"/chat/completions", bytes.NewReader(raw))
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

// ListModels returns the list of models available to the configured API key.
func (k *KimiStrategy) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	if k.APIKey == "" {
		return nil, fmt.Errorf("KIMI_API_KEY not set")
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, k.Endpoint+"/models", nil)
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", "Bearer "+k.APIKey)
	resp, err := k.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kimi list models http %d: %s", resp.StatusCode, string(b))
	}
	var kimiResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&kimiResp); err != nil {
		return nil, err
	}
	out := make([]model.ModelInfo, 0, len(kimiResp.Data))
	for _, m := range kimiResp.Data {
		out = append(out, model.ModelInfo{ID: m.ID})
	}
	return out, nil
}
