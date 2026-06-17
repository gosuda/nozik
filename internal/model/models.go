package model

import "time"

// Emotion captures the user's current emotional state.
type Emotion string

const (
	EmotionUnknown  Emotion = "unknown"
	EmotionDepressed Emotion = "depressed"
	EmotionSad       Emotion = "sad"
	EmotionCalm      Emotion = "calm"
	EmotionHappy     Emotion = "happy"
	EmotionExcited   Emotion = "excited"
	EmotionAngry     Emotion = "angry"
)

// DiscoveryMode selects how the curation engine searches the taste DB.
type DiscoveryMode string

const (
	ModeDeepDiver DiscoveryMode = "deep_diver"
	ModePioneer   DiscoveryMode = "pioneer"
)

// ProviderKind is a supported AI provider.
type ProviderKind string

const (
	ProviderGemini ProviderKind = "gemini"
	ProviderOpenAI ProviderKind = "openai"
	ProviderGrok   ProviderKind = "grok"
	ProviderKimi   ProviderKind = "kimi"
	ProviderOther  ProviderKind = "other"
)

// PlayRecord is one row in the taste database.
type PlayRecord struct {
	ID              int64     `json:"id,omitempty"`
	ProviderID      string    `json:"provider_id"`
	ProviderTrackID string    `json:"provider_track_id"`
	Title           string    `json:"title"`
	Artist          string    `json:"artist"`
	Album           string    `json:"album,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Emotion         Emotion   `json:"emotion"`
	Weight          float64   `json:"weight"`
	PlayCount       int       `json:"play_count"`
	LastPlayed      time.Time `json:"last_played"`
}

// Recommendation is a track proposed by an AI provider or the curation engine.
type Recommendation struct {
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album,omitempty"`
	Reason          string `json:"reason,omitempty"`
	ProviderID      string `json:"provider_id,omitempty"`
	ProviderTrackID string `json:"provider_track_id,omitempty"`
}

// QueueItem is a concrete track queued for Gozik.
type QueueItem struct {
	ProviderID      string
	ProviderTrackID string
	Title           string
	Artist          string
}
