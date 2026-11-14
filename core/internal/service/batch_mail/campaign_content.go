package batch_mail

import (
	"context"
	"errors"
	"strings"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

func ValidateAuthoring(config *entity.TaskAuthoring) error {
	if config == nil {
		return nil
	}
	if config.Mode != "template" && config.Mode != "ai" {
		return errors.New("invalid content source")
	}
	if len(config.Country) > 2 || len(config.Language) > 30 || len(config.Model) > 200 || len(config.Requirement) > 32000 {
		return errors.New("authoring settings too large")
	}
	return nil
}

// ResolveCampaignContent keeps the selected template immutable. The task owns
// its edited content, including an AI draft that has no library template.
func ResolveCampaignContent(ctx context.Context, p *governance.Principal, templateID int64, override *string) (string, error) {
	if templateID < 0 {
		return "", errors.New("invalid template")
	}
	source := ""
	if templateID > 0 {
		if e := p.Check(ctx, "email_templates", templateID, "template:list"); e != nil {
			return "", e
		}
		value, e := g.DB().Model("email_templates").Ctx(ctx).Where("id", templateID).Value("content")
		if e != nil {
			return "", e
		}
		source = value.String()
	}
	if override != nil {
		source = *override
	}
	if strings.TrimSpace(source) == "" {
		return "", errors.New("email content required")
	}
	if len(source) > 1024*1024 {
		return "", errors.New("email content exceeds 1 MB")
	}
	source = public.NormalizeEmailHTML(source)
	return source, nil
}
