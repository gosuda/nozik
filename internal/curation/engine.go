package curation

import (
	"strings"

	"github.com/gg582/nozik/internal/model"
)

// DetectEmotion returns an emotion inferred from free-form text.
func DetectEmotion(text string) model.Emotion {
	t := strings.ToLower(text)
	switch {
	case containsAny(t, "depressed", "depression", "empty", "hopeless"):
		return model.EmotionDepressed
	case containsAny(t, "sad", "cry", "crying", "melancholy", "melancholic", "blue"):
		return model.EmotionSad
	case containsAny(t, "excited", "pumped", "hype", "energetic", "thrilled"):
		return model.EmotionExcited
	case containsAny(t, "happy", "glad", "joy", "cheerful"):
		return model.EmotionHappy
	case containsAny(t, "angry", "mad", "furious", "rage"):
		return model.EmotionAngry
	case containsAny(t, "calm", "relaxed", "chill", "peaceful"):
		return model.EmotionCalm
	}
	return model.EmotionUnknown
}

// ChooseMode picks a discovery mode based on emotion and user intent.
// Depressed -> conservative Deep Diver; Excited -> sometimes Pioneer.
func ChooseMode(emotion model.Emotion, userAsksForNew bool) model.DiscoveryMode {
	switch emotion {
	case model.EmotionDepressed, model.EmotionSad:
		return model.ModeDeepDiver
	case model.EmotionExcited, model.EmotionHappy:
		if userAsksForNew {
			return model.ModePioneer
		}
		return model.ModeDeepDiver
	default:
		if userAsksForNew {
			return model.ModePioneer
		}
		return model.ModeDeepDiver
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
