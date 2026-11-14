package growth

import "context"

type selectionKey struct{}

func WithModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, selectionKey{}, model)
}
func selectedModel(ctx context.Context) string { s, _ := ctx.Value(selectionKey{}).(string); return s }
