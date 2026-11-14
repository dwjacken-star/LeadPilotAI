package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/public"
	"context"
	"strings"
	"testing"
)

func TestPlainEmailPersonalizationKeepsFormatting(t *testing.T) {
	content := public.AddUnsubscribeButton("Dear {{ get .Subscriber \"name\" }},\nGreetings!\n\nProducts:\n✓ Compressors")
	contact := &entity.Contact{Email: "buyer@example.com", Attribs: map[string]string{"name": "Alex"}}
	result, err := NewTemplateEngine().RenderEmailTemplate(context.Background(), content, contact, nil, "https://example.com/unsubscribe")
	if err != nil || !strings.Contains(result, "Dear Alex,<br>Greetings!") || strings.Contains(result, "{{") || !strings.Contains(result, "https://example.com/unsubscribe") {
		t.Fatalf("plain-text formatting broke personalization or footer: %s %v", result, err)
	}
}
