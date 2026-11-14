package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/service/rbac"
	service "billionmail-core/internal/service/social"
	"billionmail-core/utility/types/api_v1"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
)

type Controller struct{}

// Bind only GET/HEAD /api/social/media outside the JWT group. The signed bearer
// capability is verified by the service; there is no general public file route.
func MetaMediaFetch(r *ghttp.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		r.Response.WriteStatus(http.StatusMethodNotAllowed)
		return
	}
	file, e := service.SignedDeliveryMedia(r.Context(), r.GetQuery("delivery").Int64(), r.GetQuery("index").Int(), r.GetQuery("expires").Int64(), r.GetQuery("digest").String(), r.GetQuery("signature").String())
	r.Response.Header().Set("Cache-Control", "private, no-store")
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Header().Set("Referrer-Policy", "no-referrer")
	if e != nil {
		r.Response.WriteStatus(http.StatusNotFound)
		return
	}
	r.Response.Header().Set("Content-Type", file.MIME)
	if file.Path != "" {
		stream, e := os.Open(file.Path)
		if e != nil {
			r.Response.WriteStatus(http.StatusNotFound)
			return
		}
		defer stream.Close()
		r.Response.ServeContent(file.Filename, time.Time{}, stream)
		return
	}
	r.Response.ServeContent(file.Filename, time.Time{}, bytes.NewReader(file.Data))
}

func New() *Controller { return &Controller{} }

type Request struct {
	g.Meta    `path:"/social/action" method:"post" tags:"Social Marketing"`
	Action    string `json:"action" v:"required"`
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	PlanID    int64  `json:"plan_id"`
	Platform  string `json:"platform"`
	AccountID int64  `json:"account_id"`
	AssetID   string `json:"asset_id"`
	Page      int    `json:"page"`
	Keyword   string `json:"keyword"`
	Status    string `json:"status"`
	Data      g.Map  `json:"data"`
}
type Response struct{ api_v1.StandardRes }

func (c *Controller) Action(ctx context.Context, req *Request) (res *Response, err error) {
	res = &Response{}
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		res.SetError(e)
		return res, nil
	}
	if req.Data == nil {
		req.Data = g.Map{}
	}
	switch req.Action {
	case "meta_readiness":
		res.Data, e = service.MetaReadiness(ctx, p)
	case "delete_record":
		e = service.DeleteRecord(ctx, p, req.Kind, req.ID)
	case "settings":
		res.Data, e = service.Settings(ctx, p)
	case "save_settings":
		var config service.Config
		e = gconv.Scan(req.Data, &config)
		if e == nil {
			e = service.SaveSettings(ctx, p, config)
		}
	case "authorize":
		res.Data, e = service.BeginAuthorization(ctx, p, gconv.String(req.Data["platform"]), req.Data)
	case "authorization_status":
		res.Data, e = service.AuthorizationStatus(ctx, p, req.ID)
	case "authorization_confirm":
		res.Data, e = service.ConfirmAuthorization(ctx, p, req.ID, gconv.Strings(req.Data["external_ids"]), gconv.Bool(req.Data["allow_provider_change"]))
	case "authorization_cancel":
		e = service.CancelAuthorization(ctx, p, req.ID)
	case "create_manual_account":
		res.Data, e = service.CreateManualAccount(ctx, p, gconv.String(req.Data["name"]))
	case "manual_reminders":
		res.Data, e = service.ManualReminders(ctx, p)
	case "account_control":
		e = service.AccountControl(ctx, p, req.ID, gconv.String(req.Data["operation"]), req.Data)
	case "departments":
		res.Data, e = service.Departments(ctx, p)
	case "account_options":
		res.Data, e = service.AccountOptions(ctx, p, req.Page, req.Keyword, gconv.Int64s(req.Data["ids"]), gconv.Strings(req.Data["platforms"]))
	case "filter_accounts":
		res.Data, e = service.FilterAccountOptions(ctx, p, gconv.String(req.Data["permission"]), service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID}, req.Page, req.Keyword)
	case "sync_reports":
		res.Data, e = service.SyncReports(ctx, p, service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID})
	case "assignees":
		res.Data, e = service.Assignees(ctx, p, req.ID)
	case "models":
		res.Data, e = growth.SocialModels(ctx, p)
	case "image_models":
		res.Data, e = growth.SocialImageModels(ctx, p)
	case "post_images":
		res.Data, e = service.PostImages(ctx, p, req.ID)
	case "process_images":
		res.Data, e = service.QueuePostImages(ctx, p, req.ID, gconv.Strings(req.Data["images"]), gconv.String(req.Data["model"]), gconv.String(req.Data["prompt"]), gconv.String(req.Data["size"]))
	case "availability":
		res.Data, e = service.Availability(ctx, p)
	case "library_options":
		res.Data, e = growth.SocialLibraryOptions(ctx, p, gconv.Int64(req.Data["company_id"]), req.Kind, req.Page, req.Keyword, gconv.Strings(req.Data["ids"]))
	case "manual_variants":
		e = service.ManualVariants(ctx, p, req.ID)
	case "companies":
		res.Data, e = growth.Companies(ctx, p, "social:plan:view")
	case "list":
		res.Data, e = service.List(ctx, p, req.Kind, req.PlanID, req.Page, req.Keyword, req.Status, service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID})
	case "detail":
		res.Data, e = service.Detail(ctx, p, req.ID)
	case "save_plan":
		var config service.PlanConfig
		e = gconv.Scan(req.Data["config"], &config)
		if e == nil {
			var id int64
			id, e = service.SavePlan(ctx, p, req.ID, gconv.String(req.Data["name"]), gconv.Int64(req.Data["company_id"]), config, gconv.String(req.Data["request_key"]))
			res.Data = g.Map{"id": id}
		}
	case "plan_control":
		e = service.PlanControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "schedule":
		e = service.Schedule(ctx, p, req.ID)
	case "save_variant":
		e = service.SaveVariant(ctx, p, req.ID, req.Data)
	case "approve_variant":
		e = service.ApproveVariant(ctx, p, req.ID)
	case "material_detail":
		res.Data, e = service.MaterialDetail(ctx, p, req.AssetID, req.Page, req.Keyword)
	case "material_control":
		e = service.MaterialControl(ctx, p, req.AssetID, gconv.String(req.Data["operation"]), req.Data)
	case "delivery_control":
		e = service.DeliveryControl(ctx, p, req.ID, gconv.String(req.Data["operation"]), req.Data)
	case "delivery_detail":
		res.Data, e = service.DeliveryDetail(ctx, p, req.ID)
	case "interaction_control":
		e = service.InteractionControl(ctx, p, req.ID, gconv.String(req.Data["operation"]), req.Data)
	case "retry_job":
		e = service.RetryJob(ctx, p, req.ID)
	case "reports":
		res.Data, e = service.Reports(ctx, p, req.PlanID, service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID})
	case "calendar":
		res.Data, e = service.Calendar(ctx, p, gconv.String(req.Data["from"]), gconv.String(req.Data["to"]), service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID})
	default:
		e = errors.New("unknown social operation")
	}
	if e != nil {
		res.SetError(e)
	} else {
		res.SetSuccess("OK")
	}
	return res, nil
}

type UploadRequest struct {
	g.Meta `path:"/social/upload" method:"post" tags:"Social Marketing"`
}

func (c *Controller) Upload(ctx context.Context, req *UploadRequest) (res *Response, err error) {
	res = &Response{}
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		res.SetError(e)
		return res, nil
	}
	r := g.RequestFromCtx(ctx)
	r.Body = http.MaxBytesReader(r.Response.BufferWriter, r.Body, service.MaxUploadBytes+1024*1024)
	if e = r.ParseMultipartForm(1024 * 1024); e != nil {
		res.SetError(errors.New("invalid upload or file above 128 MB"))
		return res, nil
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, e := r.FormFile("file")
	if e != nil {
		res.SetError(errors.New("file required"))
		return res, nil
	}
	defer file.Close()
	if r.FormValue("purpose") == "post_image" {
		res.Data, e = service.UploadPostImage(ctx, p, gconv.Int64(r.FormValue("variant_id")), header.Filename, file)
	} else if r.FormValue("purpose") == "post_video" {
		res.Data, e = service.UploadPostVideo(ctx, p, gconv.Int64(r.FormValue("variant_id")), header.Filename, file, r.FormValue("ai_generated") == "true")
	} else {
		res.Data, e = service.Upload(ctx, p, gconv.Int64(r.FormValue("plan_id")), header.Filename, file, r.FormValue("ai_allowed") != "false")
	}
	if e != nil {
		res.SetError(e)
	} else {
		res.SetSuccess("OK")
	}
	return res, nil
}

type FileRequest struct {
	g.Meta `path:"/social/file" method:"post" tags:"Social Marketing"`
	ID     string `json:"id"`
	Source string `json:"source"`
}

func (c *Controller) File(ctx context.Context, req *FileRequest) (res *Response, err error) {
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return failure(e), nil
	}
	var file, name, mimeType string
	if req.Source == "library" {
		asset, e := growth.Asset(ctx, p, req.ID, "material:list")
		if e != nil {
			return failure(e), nil
		}
		file, e = growth.BlobPath(asset)
		if e != nil {
			return failure(e), nil
		}
		name = asset["filename"].String()
		mimeType = asset["mime"].String()
	} else {
		file, name, mimeType, e = service.MaterialFile(ctx, p, req.ID)
		if e != nil {
			return failure(e), nil
		}
	}
	reader, e := os.Open(file)
	if e != nil {
		return failure(errors.New("stored material unavailable")), nil
	}
	defer reader.Close()
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", mimeType)
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Header().Set("Cache-Control", "private, no-store")
	r.Response.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(name)}))
	_, e = io.Copy(r.Response.BufferWriter, reader)
	return nil, e
}

type ExportRequest struct {
	g.Meta    `path:"/social/export" method:"post" tags:"Social Marketing"`
	PlanID    int64  `json:"plan_id"`
	Platform  string `json:"platform"`
	AccountID int64  `json:"account_id"`
}

func (c *Controller) Export(ctx context.Context, req *ExportRequest) (res *Response, err error) {
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return failure(e), nil
	}
	report, e := service.Reports(ctx, p, req.PlanID, service.QueryFilter{Platform: req.Platform, AccountID: req.AccountID})
	if e != nil {
		return failure(e), nil
	}
	book := excelize.NewFile()
	defer book.Close()
	headers := []interface{}{"Plan", "Page", "Platform", "Target country", "Language", "Reactions", "Comments", "Shares", "Impressions", "Reach (per post)", "Clicks", "As of", "Availability", "Facebook views (post_media_view)"}
	book.SetSheetRow("Sheet1", "A1", &headers)
	for index, row := range gconv.Maps(report["rows"]) {
		counts, availability := g.Map{}, g.Map{}
		_ = json.Unmarshal([]byte(gconv.String(row["counts"])), &counts)
		_ = json.Unmarshal([]byte(gconv.String(row["availability"])), &availability)
		metric := func(key string) interface{} {
			if availability[key] == "available" {
				return counts[key]
			}
			return nil
		}
		values := []interface{}{gconv.String(row["plan_name"]), gconv.String(row["account_name"]), gconv.String(row["platform"]), gconv.String(row["target_country"]), gconv.String(row["language"]), metric("reactions"), metric("comments"), metric("shares"), metric("impressions"), metric("reach"), metric("clicks"), gconv.String(row["as_of"]), gconv.String(row["availability"]), metric("views")}
		cell, _ := excelize.CoordinatesToCellName(1, index+2)
		book.SetSheetRow("Sheet1", cell, &values)
	}
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.Response.Header().Set("Content-Disposition", `attachment; filename="social-aggregate-report.xlsx"`)
	_, e = book.WriteTo(r.Response.BufferWriter)
	return nil, e
}
func failure(e error) *Response { r := &Response{}; r.SetError(e); return r }
func Callback(r *ghttp.Request) {
	_, e := service.CompleteAuthorization(r.Context(), r.GetQuery("state").String(), r.GetQuery("code").String(), r.GetQuery("error").String())
	r.Response.Header().Set("Cache-Control", "no-store")
	r.Response.Header().Set("Referrer-Policy", "no-referrer")
	r.Response.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	r.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	message := "Authorization received. Return to the original management window to review the identity and confirm the connection. You may close this window."
	if e != nil {
		g.Log().Warning(r.Context(), "Social OAuth callback failed: ", e)
		message = "Authorization failed or expired. Return to the original management window and start a new connection."
	}
	r.Response.Write("<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Social authorization</title><body><p>" + message + "</p></body></html>")
}
func FacebookWebhook(r *ghttp.Request) {
	if r.Method == "GET" {
		if r.GetQuery("hub.mode").String() != "subscribe" {
			r.Response.WriteStatus(http.StatusForbidden)
			return
		}
		challenge, e := service.WebhookChallenge(r.Context(), r.GetQuery("hub.verify_token").String(), r.GetQuery("hub.challenge").String())
		if e != nil {
			r.Response.WriteStatus(http.StatusForbidden)
			return
		}
		r.Response.Write(challenge)
		return
	}
	if r.Method != "POST" {
		r.Response.WriteStatus(http.StatusMethodNotAllowed)
		return
	}
	body, e := io.ReadAll(io.LimitReader(r.Body, 1024*1024+1))
	if e != nil || len(body) > 1024*1024 {
		r.Response.WriteStatus(http.StatusRequestEntityTooLarge)
		return
	}
	if e = service.FacebookWebhook(r.Context(), r.Header.Get("X-Hub-Signature-256"), body); e != nil {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	r.Response.WriteStatus(http.StatusOK)
}
func FacebookDeauthorize(r *ghttp.Request) {
	if r.Method != "POST" {
		r.Response.WriteStatus(http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(r.Response.BufferWriter, r.Body, 1024*1024)
	if e := service.FacebookDeauthorize(r.Context(), strings.TrimSpace(r.GetForm("signed_request").String())); e != nil {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	r.Response.WriteStatus(http.StatusOK)
}
