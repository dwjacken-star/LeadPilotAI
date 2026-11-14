package growth

import (
	"billionmail-core/internal/service/aimodels"
	"context"
	"errors"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

type CampaignBrief struct {
	Requirement string `json:"requirement"`
	Country     string `json:"country"`
	Language    string `json:"language"`
	Model       string `json:"model"`
	Target      string `json:"target"`
	FullName    string `json:"full_name"`
	Subject     string `json:"subject"`
	Content     string `json:"content"`
}
type campaignDraft struct {
	FullName   string   `json:"full_name"`
	Subject    string   `json:"subject"`
	Paragraphs []string `json:"paragraphs"`
	Language   string   `json:"language"`
}

func campaignAuthor(p *governance.Principal) error {
	if p == nil || (!p.Has("campaign:create") && !p.Has("campaign:update")) {
		return errors.New("campaign authoring permission required")
	}
	return nil
}

// Discovery uses only the administrator's saved provider. Department users
// cannot read a credential or redirect it to a different endpoint.
func CampaignOptions(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if e := campaignAuthor(p); e != nil {
		return nil, e
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	if aimodels.Active(ctx) {
		catalog, e := aimodels.Catalog(ctx, "text")
		if e != nil {
			return nil, e
		}
		_, _, err := effectiveTextConfig(ctx, "")
		catalog["enabled"] = err == nil
		return catalog, nil
	}
	options, model, warning := textModelOptions(ctx, c)
	_, _, enabledError := effectiveTextConfig(ctx, model)
	return g.Map{"enabled": enabledError == nil, "model": model, "models": options, "warning": warning}, nil
}

var campaignLanguage = regexp.MustCompile(`^[a-zA-Z]{2,3}([-_][a-zA-Z0-9]{2,8}){0,2}$`)

func ComposeCampaign(ctx context.Context, p *governance.Principal, in CampaignBrief) (g.Map, error) {
	if e := campaignAuthor(p); e != nil {
		return nil, e
	}
	in.Requirement = strings.TrimSpace(in.Requirement)
	if in.Requirement == "" || utf8.RuneCountInString(in.Requirement) > 8000 {
		return nil, errors.New("enter writing requirements within 8000 characters")
	}
	if len(in.Country) != 2 || in.Country != strings.ToUpper(in.Country) || in.Country[0] < 'A' || in.Country[0] > 'Z' || in.Country[1] < 'A' || in.Country[1] > 'Z' || !campaignLanguage.MatchString(in.Language) {
		return nil, errors.New("select target country and language")
	}
	if in.Target != "all" && in.Target != "body" && in.Target != "subject" && in.Target != "full_name" {
		return nil, errors.New("invalid generation target")
	}
	if len(in.Content) > 100000 || len(in.Subject) > 2000 || len(in.FullName) > 1000 {
		return nil, errors.New("writing context too large")
	}
	if e := validateSelectedTextModel(ctx, in.Model); e != nil {
		return nil, e
	}
	if _, _, e := effectiveTextConfig(ctx, in.Model); e != nil {
		return nil, e
	}
	var e error
	var draft campaignDraft
	maxOutput := 4000
	if in.Target == "full_name" {
		maxOutput = 512
	} else if in.Target == "subject" {
		maxOutput = 768
	}
	e = IntelligenceProvider.Extract(ctx, p, "campaign_compose", AIInput{
		Model: in.Model, MaxOutput: maxOutput,
		Instruction: "Write a concise professional B2B business development email for the selected country and strictly in the selected language. Follow writing_requirements. Use current draft only as context for refinement. Respect generation_target: when subject or full_name, only propose that field and return empty paragraphs. Return plain text paragraphs, not HTML or Markdown. A paragraph may contain line breaks. Full_name is a truthful sender display name based on supplied facts. Never invent company names, certifications, warranties, prices, customer relationships or contact details. If sender identity was not supplied, use a neutral Sales Team name in the selected language. No tracking pixels, unsubscribe URLs, or recipient addresses: the delivery pipeline adds those. Never send a message or invoke tools.",
		Data:        g.Map{"writing_requirements": in.Requirement, "country": in.Country, "language": in.Language, "generation_target": in.Target, "current_full_name": in.FullName, "current_subject": in.Subject, "current_body": in.Content},
		Schema:      schema(g.Map{"full_name": strSchema(), "subject": strSchema(), "paragraphs": stringsSchema(), "language": strSchema()}),
	}, &draft)
	if e != nil {
		return nil, e
	}
	draft.FullName = strings.TrimSpace(draft.FullName)
	draft.Subject = strings.TrimSpace(draft.Subject)
	if draft.Language != in.Language {
		return nil, errors.New("AI returned a different language; regenerate")
	}
	if strings.ContainsAny(draft.FullName+draft.Subject, "\r\n\x00") || utf8.RuneCountInString(draft.FullName) > 120 || utf8.RuneCountInString(draft.Subject) > 200 {
		return nil, errors.New("AI returned invalid email headers")
	}
	if (in.Target == "all" || in.Target == "full_name") && draft.FullName == "" {
		return nil, errors.New("AI returned an empty sender display name")
	}
	if (in.Target == "all" || in.Target == "subject") && draft.Subject == "" {
		return nil, errors.New("AI returned an empty subject")
	}
	content := ""
	if in.Target == "all" || in.Target == "body" {
		if len(draft.Paragraphs) == 0 || len(draft.Paragraphs) > 40 {
			return nil, errors.New("AI returned empty or excessive email content")
		}
		for _, paragraph := range draft.Paragraphs {
			paragraph = strings.TrimSpace(paragraph)
			if paragraph == "" {
				continue
			}
			content += `<p style="margin:0 0 16px;">` + strings.ReplaceAll(html.EscapeString(strings.ReplaceAll(paragraph, "\r\n", "\n")), "\n", "<br>") + `</p>`
		}
		if content == "" || len(content) > 100000 {
			return nil, errors.New("AI returned invalid email content")
		}
		direction := "ltr"
		if strings.HasPrefix(in.Language, "ar") || strings.HasPrefix(in.Language, "he") || strings.HasPrefix(in.Language, "fa") || strings.HasPrefix(in.Language, "ur") {
			direction = "rtl"
		}
		content = `<div lang="` + html.EscapeString(in.Language) + `" dir="` + direction + `" style="font-family:Arial,sans-serif;font-size:15px;line-height:1.7;color:#243247;">` + content + `</div>`
	}
	return g.Map{"full_name": draft.FullName, "subject": draft.Subject, "html_content": content, "country": in.Country, "language": in.Language, "model": in.Model, "target": in.Target}, nil
}
