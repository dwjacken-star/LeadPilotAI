package askai

import "strings"

func IsEmailModel(id string) bool {
	value := strings.ToLower(id)
	if strings.HasPrefix(value, "gpt-") && strings.HasSuffix(value, "-pro") {
		return false
	}
	for _, word := range []string{"embedding", "moderation", "transcribe", "whisper", "tts-", "realtime", "audio", "image", "dall-e", "codex", "search-api", "babbage", "davinci", "instruct"} {
		if strings.Contains(value, word) {
			return false
		}
	}
	return true
}
func emailModelPriority(id string) int {
	switch id {
	case "gpt-4.1-mini":
		return 0
	case "gpt-4o-mini":
		return 1
	case "gpt-5-mini":
		return 2
	}
	if strings.HasPrefix(id, "gpt-3.5") {
		return 10
	}
	return 5
}
