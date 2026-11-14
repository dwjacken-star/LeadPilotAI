package batch_mail

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

func DeliveryLinks(ctx context.Context, recipient string, templateID, taskID, groupID int) (string, string, error) {
	if groupID <= 0 {
		return "", "", fmt.Errorf("select a recipient group for subscription links")
	}
	group, e := g.DB().Model("bm_contact_groups").Ctx(ctx).Where("id", groupID).One()
	if e != nil {
		return "", "", e
	}
	if group.IsEmpty() {
		return "", "", fmt.Errorf("recipient group not found")
	}
	groupToken := group["token"].String()
	if groupToken == "" {
		groupToken = strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
		if _, e = g.DB().Model("bm_contact_groups").Ctx(ctx).Where("id", groupID).Data("token", groupToken).Update(); e != nil {
			return "", "", e
		}
	}
	token, e := GenerateUnsubscribeJWT(recipient, templateID, taskID, groupID)
	if e != nil {
		return "", "", e
	}
	base := strings.TrimRight(domains.GetBaseURL(), "/")
	return base + "/unsubscribe_new.html?jwt=" + url.QueryEscape(token), base + "/subscribe_form.html?token=" + url.QueryEscape(groupToken), nil
}

func AppendDeliveryLinks(content, unsubscribe, subscribe string) string {
	if unsubscribe == "" {
		return content
	}
	if strings.Contains(content, unsubscribe) {
		if strings.Contains(content, subscribe) {
			return content
		}
		return public.AppendEmailHTML(content, fmt.Sprintf(`<p><a href="%s">订阅 / Subscribe</a></p>`, subscribe))
	}
	return public.AppendEmailHTML(content, fmt.Sprintf(`<p style="font-size:12px;color:#777;margin-top:24px"><a href="%s">退订 / Unsubscribe</a> &nbsp;|&nbsp; <a href="%s">订阅 / Subscribe</a></p>`, unsubscribe, subscribe))
}

func InstrumentDelivery(ctx context.Context, content, mid, recipient string, taskID, templateID, groupID int, options TaskDeliveryOptions) (string, map[string]string, error) {
	headers := map[string]string{}
	unsubscribe, subscribe := "", ""
	var e error
	if options.Unsubscribe {
		unsubscribe, subscribe, e = DeliveryLinks(ctx, recipient, templateID, taskID, groupID)
		if e != nil {
			return "", nil, e
		}
		headers["List-Unsubscribe"] = "<" + unsubscribe + ">"
	}
	if taskID == 0 {
		content, e = maillog_stat.InstrumentWebmail(ctx, content, strings.Trim(mid, "<> "), recipient, options.TrackOpen, options.TrackClick)
		return AppendDeliveryLinks(content, unsubscribe, subscribe), headers, e
	}
	tracker := maillog_stat.NewMailTracker(content, taskID, mid, recipient, domains.GetBaseURL())
	if options.TrackOpen {
		tracker.AppendTrackingPixel()
	}
	if options.TrackClick {
		tracker.TrackLinks()
	}
	return AppendDeliveryLinks(tracker.GetHTML(), unsubscribe, subscribe), headers, nil
}
