package growth

import (
	"billionmail-core/internal/service/aimodels"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"billionmail-core/internal/service/governance"
	service "billionmail-core/internal/service/growth"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"billionmail-core/utility/types/api_v1"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type Controller struct{}

func New() *Controller { return &Controller{} }

type Request struct {
	g.Meta    `path:"/growth/action" method:"post" tags:"Growth"`
	Action    string `json:"action" v:"required"`
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	AssetID   string `json:"asset_id"`
	CompanyID int64  `json:"company_id"`
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
	case "model_registry":
		res.Data, e = aimodels.Settings(ctx, p)
	case "save_model_registry":
		var registry aimodels.Registry
		e = gconv.Scan(req.Data, &registry)
		if e == nil {
			e = aimodels.Save(ctx, p, registry)
		}
	case "discover_registry_models":
		var provider aimodels.Provider
		e = gconv.Scan(req.Data, &provider)
		if e == nil {
			res.Data, e = aimodels.Discover(ctx, p, provider)
		}
	case "model_catalog":
		res.Data, e = aimodels.Catalog(ctx, gconv.String(req.Data["kind"]))

	case "master":
		res.Data, e = service.MasterAction(ctx, p, req.Kind, req.ID, req.CompanyID, req.Page, req.Keyword, req.Status, req.Data)
	case "company_grants":
		if !p.Super {
			e = errors.New("administrator required")
		} else {
			res.Data, e = g.DB().Model("bm_growth_grants").Ctx(ctx).Where("company_id", req.ID).Fields("dept_id").All()
		}
	case "companies":
		res.Data, e = service.Companies(ctx, p, "company:list")
	case "save_company":
		res.Data, e = service.SaveCompany(ctx, p, req.ID, req.Data)
	case "publish_company":
		e = service.PublishCompany(ctx, p, req.ID)
	case "share_company":
		e = service.GrantCompany(ctx, p, req.ID, gconv.Int64s(req.Data["dept_ids"]))
	case "list":
		res.Data, e = service.List(ctx, p, req.Kind, req.CompanyID, req.Page, req.Keyword, req.Status)
	case "detail":
		res.Data, e = service.Detail(ctx, p, req.Kind, req.ID)
	case "product":
		res.Data, e = service.ProductDetail(ctx, p, req.ID)
	case "save_product":
		res.Data, e = service.SaveProduct(ctx, p, req.ID, req.Data)
	case "publish_product":
		e = service.PublishProduct(ctx, p, req.ID, gconv.Bool(req.Data["publish"]))
	case "asset":
		res.Data, e = service.AssetDetail(ctx, p, req.AssetID)
	case "image_preview":
		res.Data, e = service.ImagePreview(ctx, p, req.AssetID)
	case "update_asset":
		e = service.UpdateAsset(ctx, p, req.AssetID, req.Data)
	case "publish_asset":
		e = service.PublishAsset(ctx, p, req.AssetID, gconv.Bool(req.Data["publish"]))
	case "recognize":
		res.Data, e = service.RecognizeAsset(service.WithModel(ctx, gconv.String(req.Data["model"])), p, req.AssetID)
	case "apply_company":
		e = service.ApplyRecognizedCompany(ctx, p, req.AssetID)
	case "attach_image":
		e = service.AttachImage(ctx, p, req.ID, req.AssetID, gconv.String(req.Data["role"]), true)
	case "mapping":
		e = service.ManualMapping(ctx, p, req.AssetID, req.Data)
	case "import_rows":
		res.Data, e = service.ImportRows(ctx, p, req.AssetID, req.Page)
	case "resolve_row":
		e = service.ResolveImportRow(ctx, p, req.ID, req.Data)
	case "versions":
		res.Data, e = service.Versions(ctx, p, req.CompanyID, req.Kind, gconv.String(req.Data["resource_id"]))
	case "archive":
		e = service.Archive(ctx, p, req.Kind, req.ID, req.AssetID)
	case "save_plan":
		res.Data, e = service.SavePlan(ctx, p, req.ID, req.Data)
	case "plan_control":
		res.Data, e = service.PlanControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "lead_control":
		res.Data, e = service.LeadControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "message_control":
		e = service.MessageControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "regenerate_message":
		res.Data, e = service.RegenerateMessage(ctx, p, req.ID)
	case "job_control":
		e = service.JobControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "inquiry_control":
		e = service.InquiryControl(ctx, p, req.ID, gconv.String(req.Data["operation"]))
	case "save_inbox_rule":
		res.Data, e = service.SaveInboxRule(ctx, p, req.ID, req.Data)
	case "reply_draft":
		res.Data, e = service.ReplyDraft(service.WithModel(ctx, gconv.String(req.Data["model"])), p, req.ID, req.CompanyID)
	case "text_model_options":
		if !p.Has("intelligence:manage") && !p.Has("outreach:manage") {
			e = errors.New("AI model selection permission required")
		} else {
			res.Data, e = service.KeywordModelOptions(ctx, p)
		}
	case "text_provider_settings":
		res.Data, e = service.TextProviderSettings(ctx, p)
	case "text_provider_models":
		res.Data, e = service.TextProviderModels(ctx, p, gconv.String(req.Data["provider"]), gconv.String(req.Data["base_url"]), gconv.String(req.Data["api_key"]))
	case "save_text_providers":
		var cfg service.TextProviders
		e = gconv.Scan(req.Data, &cfg)
		if e == nil {
			e = service.SaveTextProviders(ctx, p, cfg)
		}
	case "test_text_provider":
		if !p.Super {
			e = errors.New("administrator required")
		} else {
			var result g.Map
			e = service.IntelligenceProvider.Extract(ctx, p, "provider_test", service.AIInput{Model: gconv.String(req.Data["model"]), MaxOutput: 256, Instruction: "Return a JSON object with ok set to true.", Data: g.Map{"test": true}, Schema: g.Map{"type": "object", "properties": g.Map{"ok": g.Map{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false}}, &result)
			res.Data = result
		}
	case "delete_record":
		e = service.DeleteRecord(ctx, p, "growth", req.Kind, req.ID)
	case "expand_keywords":
		res.Data, e = service.ExpandIntentKeywords(ctx, p, req.Data)
	case "settings":
		res.Data, e = service.Settings(ctx, p)
	case "serper_settings":
		res.Data, e = service.SerperSettings(ctx, p)
	case "save_serper_settings":
		var config service.SerperConfig
		e = gconv.Scan(req.Data, &config)
		if e == nil {
			e = service.SaveSerperSettings(ctx, p, config)
		}
	case "test_serper":
		res.Data, e = service.TestSerper(ctx, p)
	case "models":
		res.Data, e = service.Models(ctx, p, gconv.String(req.Data["base_url"]), gconv.String(req.Data["api_key"]))
	case "campaign_options":
		res.Data, e = service.CampaignOptions(ctx, p)
	case "campaign_compose":
		var input service.CampaignBrief
		e = gconv.Scan(req.Data, &input)
		if e == nil {
			res.Data, e = service.ComposeCampaign(ctx, p, input)
		}
	case "delivery_identity":
		res.Data, e = mail_service.OutboundIdentity("")
	case "delivery_suppressions":
		if !p.Super {
			e = errors.New("administrator required")
		} else {
			page := req.Page
			if page < 1 {
				page = 1
			}
			query := g.DB().Model("bm_delivery_suppressions").Ctx(ctx)
			if req.Keyword != "" {
				query = query.Where("email ILIKE ?", "%"+req.Keyword+"%")
			}
			var count int
			count, e = query.Clone().Count()
			if e == nil {
				var rows interface{}
				rows, e = query.Order("active DESC,updated_at DESC,id DESC").Page(page, 20).All()
				res.Data = g.Map{"list": rows, "total": count}
			}
		}
	case "save_settings":
		var config service.AIConfig
		e = gconv.Scan(req.Data, &config)
		if e == nil {
			e = service.SaveSettings(ctx, p, config)
		}
	case "stats":
		res.Data, e = service.Stats(ctx, p)
	default:
		e = errors.New("unknown growth action")
	}
	if e != nil {
		res.SetError(e)
	} else {
		res.SetSuccess("OK")
		if req.Action != "list" && req.Action != "detail" && req.Action != "settings" && req.Action != "models" && req.Action != "campaign_options" && req.Action != "delivery_identity" && req.Action != "companies" && req.Action != "stats" && req.Action != "asset" && req.Action != "product" && req.Action != "import_rows" && req.Action != "versions" {
			_ = governance.Audit(ctx, p, "growth:"+req.Action, req.ID, g.Map{"kind": req.Kind, "asset_id": req.AssetID})
		}
	}
	return res, nil
}

type UploadReq struct {
	g.Meta `path:"/growth/upload" method:"post" tags:"Growth"`
}

func (c *Controller) Upload(ctx context.Context, req *UploadReq) (res *Response, err error) {
	res = &Response{}
	r := g.RequestFromCtx(ctx)
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		res.SetError(e)
		return res, nil
	}
	r.Body = http.MaxBytesReader(r.Response.BufferWriter, r.Body, service.MaxUploadBytes+1024*1024)
	if e = r.ParseMultipartForm(1024 * 1024); e != nil {
		res.SetError(errors.New("upload exceeds 128 MB or invalid multipart data"))
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
	if r.FormValue("purpose") == "industry_offerings" {
		res.Data, e = service.MasterPreviewWorkbook(ctx, p, gconv.Int64(r.FormValue("company_id")), gconv.Int64(r.FormValue("pack_id")), header.Filename, file)
	} else {
		res.Data, e = service.Upload(service.WithModel(ctx, r.FormValue("model")), p, gconv.Int64(r.FormValue("company_id")), header.Filename, r.FormValue("purpose"), file, r.FormValue("ai_allowed") != "false")
	}
	if e != nil {
		res.SetError(e)
	} else {
		res.SetSuccess("OK")
	}
	return res, nil
}

type DownloadReq struct {
	g.Meta  `path:"/growth/file" method:"post" tags:"Growth"`
	AssetID string `json:"asset_id" v:"required"`
}

func (c *Controller) File(ctx context.Context, req *DownloadReq) (res *Response, err error) {
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return fail(e), nil
	}
	r, e := service.Asset(ctx, p, req.AssetID, "material:download")
	if e != nil {
		return fail(e), nil
	}
	path, e := service.BlobPath(r)
	if e != nil {
		return fail(errors.New("stored file unavailable")), nil
	}
	file, e := os.Open(path)
	if e != nil {
		return fail(errors.New("stored file unavailable")), nil
	}
	defer file.Close()
	request := g.RequestFromCtx(ctx)
	request.Response.Header().Set("Content-Type", r["mime"].String())
	request.Response.Header().Set("X-Content-Type-Options", "nosniff")
	request.Response.Header().Set("Cache-Control", "private, no-store")
	request.Response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(r["filename"].String())}))
	_, e = io.Copy(request.Response.BufferWriter, file)
	return nil, e
}

type ExportReq struct {
	g.Meta    `path:"/growth/export" method:"post" tags:"Growth"`
	Kind      string `json:"kind"`
	CompanyID int64  `json:"company_id"`
	AssetID   string `json:"asset_id"`
}

type UploadTemplateReq struct {
	g.Meta    `path:"/growth/template" method:"post" tags:"Growth"`
	Kind      string `json:"kind" v:"required|in:products,company,images"`
	CompanyID int64  `json:"company_id" v:"required|min:1"`
	Language  string `json:"language"`
}

func (c *Controller) UploadTemplate(ctx context.Context, req *UploadTemplateReq) (res *Response, err error) {
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return fail(e), nil
	}
	body, contentType, name, e := service.UploadTemplate(ctx, p, req.CompanyID, req.Kind, req.Language)
	if e != nil {
		return fail(e), nil
	}
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", contentType)
	r.Response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	r.Response.Header().Set("Cache-Control", "private, no-store")
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Write(body)
	return nil, nil
}

func (c *Controller) Export(ctx context.Context, req *ExportReq) (res *Response, err error) {
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return fail(e), nil
	}
	body, e := service.Export(ctx, p, req.Kind, req.CompanyID, req.AssetID)
	if e != nil {
		return fail(e), nil
	}
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", public.SpreadsheetMIME)
	r.Response.Header().Set("Content-Disposition", `attachment; filename="maplemail-export.xlsx"`)
	r.Response.Write(body)
	return nil, nil
}
func fail(e error) *Response { r := &Response{}; r.SetError(e); return r }
