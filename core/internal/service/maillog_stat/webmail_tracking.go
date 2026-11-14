package maillog_stat

import (
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/public"
	"context"
	"crypto/rand"
	"encoding/hex"
	"html"
	"image"
	"image/png"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

var webmailTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func InstrumentWebmail(ctx context.Context, content, mid, recipient string, opened, clicked bool) (string, error) {
	if !opened && !clicked {
		return content, nil
	}
	base := strings.TrimRight(domains.GetBaseURL(), "/")
	register := func(kind, target string) (string, error) {
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			return "", e
		}
		token := hex.EncodeToString(b)
		_, e := g.DB().Model("bm_webmail_tracking").Ctx(ctx).Data(g.Map{"token": token, "message_id": mid, "recipient": recipient, "event_type": kind, "target_url": target}).Insert()
		return base + "/pmta/w/" + token, e
	}
	var failure error
	if clicked {
		pattern := regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
		content = pattern.ReplaceAllStringFunc(content, func(match string) string {
			target := html.UnescapeString(pattern.FindStringSubmatch(match)[1])
			parsed, e := url.Parse(target)
			if e != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || strings.Contains(target, "/pmta/") || strings.Contains(parsed.Path, "subscribe") {
				return match
			}
			tracked, e := register("click", target)
			if e != nil {
				failure = e
				return match
			}
			return `href="` + html.EscapeString(tracked) + `"`
		})
	}
	if failure != nil {
		return "", failure
	}
	if opened {
		tracked, e := register("open", "")
		if e != nil {
			return "", e
		}
		content = public.AppendEmailHTML(content, `<img src="`+html.EscapeString(tracked)+`" width="1" height="1" alt="" style="width:1px;height:1px;border:0;opacity:0" />`)
	}
	return content, nil
}

func writeTrackingPixel(r *ghttp.Request) {
	r.Response.Header().Set("Content-Type", "image/png")
	r.Response.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	r.Response.Header().Set("Pragma", "no-cache")
	_ = png.Encode(r.Response.BufferWriter, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
}

// Opaque tokens are registered by the authenticated webmail sender. They cannot
// be manufactured by a public tracking request or applied to historical mail.
func recordWebmailEvent(ctx context.Context, token string) (string, error) {
	if !webmailTokenPattern.MatchString(token) {
		return "", nil
	}
	target := ""
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, err := tx.Model("bm_webmail_tracking").Where("token", token).LockUpdate().One()
		if err != nil || row.IsEmpty() {
			return err
		}
		if row["event_type"].String() == "click" {
			u, e := url.Parse(row["target_url"].String())
			if e != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" {
				return nil
			}
			target = u.String()
		}
		if row["recorded"].Bool() {
			return nil
		}
		queue, err := tx.Model("mailstat_message_ids").Where("message_id", row["message_id"].String()).OrderDesc("log_time_millis").Value("postfix_message_id")
		if err != nil {
			return err
		}
		postfixID := ""
		if queue != nil {
			postfixID = queue.String()
		}
		table := "mailstat_opened"
		if row["event_type"].String() == "click" {
			table = "mailstat_clicked"
		}
		data := g.Map{"campaign_id": 0, "message_id": row["message_id"].String(), "recipient": row["recipient"].String(), "postfix_message_id": postfixID, "log_time_millis": time.Now().UnixMilli()}
		if target != "" {
			data["url"] = target
		}
		_, err = tx.Model(table).Data(data).Insert()
		if err != nil {
			return err
		}
		_, err = tx.Model("bm_webmail_tracking").Where("token", token).Data("recorded", true).Update()
		return err
	})
	return target, err
}

func WebmailTrackingHandler(r *ghttp.Request, path string) {
	target, err := recordWebmailEvent(r.GetCtx(), strings.TrimPrefix(path, "w/"))
	if err != nil {
		g.Log().Error(r.GetCtx(), "Unable to record webmail tracking event", err)
	}
	if target != "" {
		r.Response.Header().Set("Cache-Control", "no-store")
		r.Response.RedirectTo(target)
		return
	}
	writeTrackingPixel(r)
}
