package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gg582/nozik/internal/model"
)

// RecommendationRequest is passed to every AI provider strategy.
type RecommendationRequest struct {
	Message        string
	Emotion        model.Emotion
	RecentHistory  []model.PlayRecord
	DiscoveryMode  model.DiscoveryMode
	Limit          int
	SystemContext  string // extra context from the curation engine
}

// AIProviderStrategy is the strategy pattern entry point for AI providers.
type AIProviderStrategy interface {
	Name() string
	RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error)
}

// DefaultLimit is used when a request does not specify one.
const DefaultLimit = 5

// BuildPrompt returns a uniform prompt that asks for JSON output.
func BuildPrompt(req RecommendationRequest) string {
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are Nozik, a music recommendation assistant. The user is feeling %s. ", req.Emotion)
	if req.DiscoveryMode != "" {
		fmt.Fprintf(&b, "Discovery mode: %s. ", req.DiscoveryMode)
	}
	if req.SystemContext != "" {
		fmt.Fprintf(&b, "Context: %s ", req.SystemContext)
	}
	if len(req.RecentHistory) > 0 {
		b.WriteString("Recent taste history:\n")
		for i, r := range req.RecentHistory {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s by %s (%s)\n", r.Title, r.Artist, r.Emotion)
		}
	}
	fmt.Fprintf(&b, "\nUser request: %s\n", req.Message)
	fmt.Fprintf(&b, "Recommend exactly %d songs. Return ONLY a JSON array of objects with fields: title, artist, album, reason, provider_id, provider_track_id. Do not wrap in markdown.", req.Limit)
	return b.String()
}

// ParseRecommendationJSON extracts the JSON array from a model response.
func ParseRecommendationJSON(text string) ([]model.Recommendation, error) {
	text = strings.TrimSpace(text)
	// Strip markdown code fences if present.
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		start, end := 0, len(lines)-1
		if strings.HasPrefix(lines[0], "```") {
			start = 1
		}
		if strings.HasSuffix(lines[end], "```") {
			end--
		}
		text = strings.TrimSpace(strings.Join(lines[start:end+1], "\n"))
	}
	var recs []model.Recommendation
	if err := json.Unmarshal([]byte(text), &recs); err != nil {
		return nil, fmt.Errorf("parsing recommendation JSON: %w", err)
	}
	for i := range recs {
		recs[i].Title = strings.TrimSpace(recs[i].Title)
		recs[i].Artist = strings.TrimSpace(recs[i].Artist)
	}
	return recs, nil
}
