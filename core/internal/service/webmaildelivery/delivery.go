package webmaildelivery

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/google/uuid"
)

type Options struct {
	Unsubscribe bool `json:"unsubscribe"`
	TrackOpen   bool `json:"track_open"`
	TrackClick  bool `json:"track_click"`
	Warmup      bool `json:"warmup"`
}
type Request struct {
	Action     string   `json:"action"`
	MessageID  string   `json:"message_id"`
	Recipients []string `json:"recipients"`
	HTML       string   `json:"html"`
	Options    Options  `json:"options"`
	Data       g.Map    `json:"data"`
}

var WarmupCheck = batch_mail.CheckDeliveryWarmup

func Authenticate(ctx context.Context, username, password string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	hash := sha256.Sum256([]byte(username))
	key := "webmail-auth-failed:" + hex.EncodeToString(hash[:])
	if value, e := public.Cache.Get(ctx, key); e == nil && !value.IsEmpty() {
		return errors.New("mailbox authentication temporarily limited")
	}
	row, e := g.DB().Model("mailbox").Ctx(ctx).Where("username", username).Where("active", 1).One()
	if e != nil {
		return errors.New("mailbox authentication unavailable")
	}
	expected := ""
	if !row.IsEmpty() {
		expected, e = mail_service.PasswordPlainByEmail(ctx, username)
	}
	if e != nil || expected == "" || password == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(password)) != 1 {
		_ = public.Cache.Set(ctx, key, true, 3*time.Second)
		return errors.New("mailbox authentication failed")
	}
	return nil
}

func subscriptionGroup(ctx context.Context, mailbox string) (int, error) {
	var id int64
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614822)"); e != nil {
			return e
		}
		hash := sha256.Sum256([]byte(mailbox))
		name := "Webmail " + mailbox
		if len(name) > 30 {
			name = "Webmail " + hex.EncodeToString(hash[:10])
		}
		dept, e := tx.GetValue("SELECT min(dept_id) FROM bm_resource_grants WHERE sender=$1 AND active HAVING count(distinct dept_id)=1", mailbox)
		if e == nil && dept.IsEmpty() {
			dept, e = tx.GetValue("SELECT min(id) FROM bm_departments WHERE active HAVING count(*)=1")
		}
		if e != nil {
			return e
		}
		// A shared mailbox spanning departments needs an administrator to assign a
		// subscription list; never guess a department or expose another team's list.
		if dept.IsEmpty() {
			return errors.New("assign a dedicated subscription group before using Webmail marketing across departments")
		}
		existing, e := tx.Model("bm_contact_groups").Where("dept_id", dept.Int64()).Where("name", name).Value("id")
		if e != nil {
			return e
		}
		if !existing.IsEmpty() {
			id = existing.Int64()
			return nil
		}
		owner, e := tx.Model("account").Where("dept_id", dept.Int64()).Order("account_id").Value("account_id")
		if e != nil {
			return e
		}
		if owner.IsEmpty() {
			return errors.New("mailbox department owner unavailable")
		}
		id, e = tx.Model("bm_contact_groups").Data(g.Map{"name": name, "token": strings.ReplaceAll(uuid.NewString(), "-", "")[:24], "dept_id": dept.Int64(), "owner_id": owner.Int64()}).InsertAndGetId()
		return e
	})
	return int(id), e
}

func Prepare(ctx context.Context, mailbox string, request Request) (g.Map, error) {
	identity, e := mail_service.OutboundIdentity(mailbox)
	if e != nil {
		return nil, e
	}
	for _, recipient := range request.Recipients {
		blocked, e := deliveryprefs.IsSuppressed(ctx, identity.From, recipient)
		if e != nil {
			return nil, e
		}
		if blocked {
			return nil, errors.New("recipient has unsubscribed; message not sent")
		}
	}
	if request.Action == "config" {
		options, err := DraftOptions(ctx, mailbox, g.NewVar(request.Data["draft_key"]).String())
		if err != nil {
			return nil, err
		}
		return g.Map{"identity": identity, "options": options}, nil
	}
	if request.Action != "prepare" || len(request.Recipients) == 0 || len(request.Recipients) > 100 {
		return nil, errors.New("invalid delivery request")
	}
	if len(request.HTML) > 2*1024*1024 || len(request.MessageID) > 300 || strings.ContainsAny(request.MessageID, "\r\n") {
		return nil, errors.New("invalid message content or identifier")
	}
	if len(request.Recipients) > 1 && (request.Options.TrackClick || request.Options.TrackOpen || request.Options.Unsubscribe) {
		return nil, errors.New("Send to one recipient when using personalized subscription links or tracking; otherwise turn those options off")
	}
	for _, recipient := range request.Recipients {
		parsed, e := mail.ParseAddress(recipient)
		if e != nil || parsed.Address != recipient {
			return nil, errors.New("invalid recipient")
		}
	}
	if request.MessageID == "" {
		request.MessageID = "<" + uuid.NewString() + "@" + strings.Split(mailbox, "@")[1] + ">"
	}
	unsubscribe, subscribe := "", ""
	if request.Options.Unsubscribe {
		group, e := subscriptionGroup(ctx, mailbox)
		if e != nil {
			return nil, e
		}
		// Do not reactivate a recipient who previously opted out of this mailbox list.
		row, e := g.DB().Model("bm_contacts").Ctx(ctx).Where("group_id", group).Where("email", request.Recipients[0]).One()
		if e != nil {
			return nil, e
		}
		if !row.IsEmpty() && row["active"].Int() == 0 {
			return nil, errors.New("recipient has unsubscribed from this mailbox list")
		}
		groupRow, e := g.DB().Model("bm_contact_groups").Ctx(ctx).Where("id", group).One()
		if e != nil {
			return nil, e
		}
		if _, e = g.DB().Model("bm_contacts").Ctx(ctx).Data(g.Map{"group_id": group, "email": request.Recipients[0], "active": 1, "status": 1, "dept_id": groupRow["dept_id"], "owner_id": groupRow["owner_id"]}).InsertIgnore(); e != nil {
			return nil, e
		}
		unsubscribe, subscribe, e = batch_mail.DeliveryLinks(ctx, request.Recipients[0], 0, 0, group)
		if e != nil {
			return nil, e
		}
	}
	if request.Options.Warmup {
		if e = WarmupCheck(ctx, request.Recipients); e != nil {
			return nil, e
		}
	}
	content := request.HTML
	if len(request.Recipients) == 1 {
		content, e = maillog_stat.InstrumentWebmail(ctx, content, strings.Trim(request.MessageID, "<> "), request.Recipients[0], request.Options.TrackOpen, request.Options.TrackClick)
		if e != nil {
			return nil, e
		}
	}
	content = batch_mail.AppendDeliveryLinks(content, unsubscribe, subscribe)
	headers := g.Map{}
	if unsubscribe != "" {
		headers["List-Unsubscribe"] = "<" + unsubscribe + ">"
	}
	plain := ""
	if unsubscribe != "" {
		plain = "\n\nUnsubscribe: " + unsubscribe + "\nSubscribe: " + subscribe
	}
	return g.Map{"html": content, "plain_append": plain, "headers": headers, "message_id": request.MessageID, "identity": identity}, nil
}

// Webmail uses mailbox credentials in a server-side HTTPS Basic request. This
// endpoint does not accept an administrator token or expose mailbox credentials.
func Handler(r *ghttp.Request) {
	r.Response.Header().Set("Cache-Control", "no-store")
	fail := func(message string) { r.Response.WriteJson(g.Map{"success": false, "msg": message}) }
	if r.Method != http.MethodPost {
		r.Response.WriteStatus(http.StatusMethodNotAllowed)
		return
	}
	username, password, ok := r.Request.BasicAuth()
	if !ok {
		fail("mailbox authentication required")
		return
	}
	ctx := r.GetCtx()
	if e := Authenticate(ctx, username, password); e != nil {
		fail(e.Error())
		return
	}
	body, e := io.ReadAll(io.LimitReader(r.Body, 3*1024*1024+1))
	if e != nil || len(body) > 3*1024*1024 {
		fail("delivery request too large")
		return
	}
	var request Request
	if json.Unmarshal(body, &request) != nil {
		fail("invalid delivery request")
		return
	}
	var result g.Map
	if request.Action == "delivery_preview" {
		var options Options
		_ = g.NewVar(request.Data["options"]).Scan(&options)
		options.Warmup = false
		result, e = Prepare(ctx, strings.ToLower(username), Request{Action: "prepare", Recipients: g.NewVar(request.Data["recipients"]).Strings(), HTML: g.NewVar(request.Data["html"]).String(), Options: options})
	} else if request.Action == "draft_options_save" {
		result, e = SaveDraftOptions(ctx, strings.ToLower(username), request.Data)
	} else if strings.HasPrefix(request.Action, "workbench_") {
		result, e = Workbench(ctx, strings.ToLower(username), request)
	} else {
		result, e = Prepare(ctx, strings.ToLower(username), request)
	}
	if e != nil {
		fail(e.Error())
		return
	}
	r.Response.WriteJson(g.Map{"success": true, "data": result})
}
