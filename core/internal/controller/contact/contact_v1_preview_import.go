package contact

import (
	v1 "billionmail-core/api/contact/v1"
	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) PreviewImport(ctx context.Context, req *v1.PreviewImportReq) (res *v1.PreviewImportRes, err error) {
	res = &v1.PreviewImportRes{}
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		res.SetError(e)
		return res, nil
	}
	if !p.Has("contact:import") {
		res.SetError(errors.New("Import access denied"))
		return res, nil
	}
	if req.Async || req.JobID != "" {
		id := req.JobID
		if id == "" {
			id, e = contactimport.CreateJob(ctx, p, req.FileData, req.FilterKeywords, req.ValidationSender)
		}
		if e == nil && (req.DownloadInvalid || req.DownloadPending) {
			status := "invalid"
			if req.DownloadPending {
				status = "pending"
			}
			var body []byte
			body, e = contactimport.JobWorkbook(ctx, p, id, status)
			if e == nil {
				request := g.RequestFromCtx(ctx)
				request.Response.Header().Set("Content-Type", public.SpreadsheetMIME)
				request.Response.Header().Set("Content-Disposition", `attachment; filename="`+status+`-contacts.xlsx"`)
				request.Response.Write(body)
				return nil, nil
			}
		} else if e == nil {
			res.Data, e = contactimport.JobPreview(ctx, p, id, req.Status, req.Page, req.RetryUnconfirmed)
		}
		if e != nil {
			res.SetError(e)
		} else {
			res.SetSuccess("OK")
		}
		return res, nil
	}
	scan, e := contactimport.ReadForPrincipal(ctx, p, req.FileData, req.FilterKeywords, req.ValidationSender)
	if e != nil {
		res.SetError(e)
		return res, nil
	}
	if req.DownloadInvalid || req.DownloadPending {
		status := "invalid"
		if req.DownloadPending {
			status = "pending"
		}
		body, e := scan.StatusWorkbook(status)
		if e != nil {
			return nil, e
		}
		request := g.RequestFromCtx(ctx)
		request.Response.Header().Set("Content-Type", public.SpreadsheetMIME)
		request.Response.Header().Set("Content-Disposition", `attachment; filename="`+status+`-contacts.xlsx"`)
		request.Response.Write(body)
		return nil, nil
	}
	rows := []contactimport.Row{}
	for _, row := range scan.Rows {
		if req.Status == "" || req.Status == "all" || row.Status == req.Status {
			rows = append(rows, row)
		}
	}
	total := len(rows)
	page := req.Page
	if page < 1 {
		page = 1
	}
	start := total
	if page <= total/20+1 {
		start = (page - 1) * 20
	}
	if start > total {
		start = total
	}
	end := start + 20
	if end > total {
		end = total
	}
	res.Data = g.Map{"total": scan.Total, "valid": scan.Valid, "filtered": scan.Filtered, "invalid": scan.Invalid, "pending": scan.Pending, "duplicate": scan.Duplicate, "token": scan.Token, "validation_sender": scan.ValidationSender, "rows": rows[start:end], "row_total": total, "page": page, "page_size": 20}
	res.SetSuccess("OK")
	return res, nil
}
