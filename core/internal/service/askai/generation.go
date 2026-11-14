package askai

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

var generations = struct {
	sync.Mutex
	active map[string]context.CancelFunc
}{active: make(map[string]context.CancelFunc)}

func startGeneration(parent context.Context, id string) (context.Context, func(), error) {
	generations.Lock()
	defer generations.Unlock()
	if _, ok := generations.active[id]; ok {
		return nil, nil, errors.New("This chat is already generating")
	}
	ctx, cancel := context.WithTimeout(parent, 180*time.Second)
	generations.active[id] = cancel
	return ctx, func() { cancel(); generations.Lock(); delete(generations.active, id); generations.Unlock() }, nil
}

func stopGeneration(id string) error {
	generations.Lock()
	defer generations.Unlock()
	cancel, ok := generations.active[id]
	if !ok {
		return errors.New("Chat not found or already stopped")
	}
	cancel()
	return nil
}

// Parse complete HTML after assembling chunks; code fences may be split anywhere.
func extractEmailHTML(content string) string {
	content = regexp.MustCompile(`(?is)<think>.*?</think>`).ReplaceAllString(content, "")
	fenced := regexp.MustCompile("(?is)```(?:html)?\\s*(.*?)```").FindAllStringSubmatch(content, -1)
	for i := len(fenced) - 1; i >= 0; i-- {
		if looksLikeHTML(fenced[i][1]) {
			return strings.TrimSpace(fenced[i][1])
		}
	}
	lower := strings.ToLower(content)
	for _, tag := range []string{"<!doctype html", "<html"} {
		if start := strings.Index(lower, tag); start >= 0 {
			if end := strings.LastIndex(lower, "</html>"); end > start {
				return strings.TrimSpace(content[start : end+7])
			}
		}
	}
	content = strings.TrimSpace(content)
	if looksLikeHTML(content) && !strings.Contains(content, "```") {
		return content
	}
	return ""
}
func looksLikeHTML(s string) bool {
	return regexp.MustCompile(`(?i)<(?:!doctype|html|body|table|div|p|section|h[1-6])\b`).MatchString(s)
}

func publicAIError(err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "ai_cancelled"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "timeout"):
		return "ai_timeout"
	case strings.Contains(text, "no credits") || strings.Contains(text, "insufficient_quota") || strings.Contains(text, "insufficient balance"):
		return "ai_balance"
	case strings.Contains(text, "429"):
		return "ai_rate_limit"
	case strings.Contains(text, "401") || strings.Contains(text, "invalid api key"):
		return "ai_credentials"
	case strings.Contains(text, "404") || strings.Contains(text, "model_not_found"):
		return "ai_model"
	case strings.Contains(text, "400") || strings.Contains(text, "422"):
		return "ai_parameters"
	default:
		return "ai_generation_failed"
	}
}
func aiError(err error) error { return fmt.Errorf("%s", publicAIError(err)) }
