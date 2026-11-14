package growth

import (
	"io"
	"net/http"
	"strings"

	service "billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gconv"
)

func WhatsAppWebhook(r *ghttp.Request) {
	cid := gconv.Int64(strings.TrimPrefix(r.URL.Path, "/growth/webhooks/whatsapp/"))
	if cid < 1 {
		r.Response.WriteStatus(http.StatusNotFound)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		r.Response.WriteStatus(http.StatusMethodNotAllowed)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024+1))
	if e != nil || len(raw) > 2*1024*1024 {
		r.Response.WriteStatus(http.StatusRequestEntityTooLarge)
		return
	}
	q := r.URL.Query()
	body, status := service.MasterWAWebhook(r.Context(), cid, q.Get("hub.mode"), q.Get("hub.verify_token"), q.Get("hub.challenge"), r.Header.Get("X-Hub-Signature-256"), raw)
	r.Response.WriteStatus(status, body)
}
