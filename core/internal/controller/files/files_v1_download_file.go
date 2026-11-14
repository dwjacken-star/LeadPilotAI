package files

import (
	"billionmail-core/api/files/v1"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"strings"
)

func (c *ControllerV1) DownloadFile(ctx context.Context, req *v1.DownloadFileReq) (res *v1.DownloadFileRes, err error) {
	name := strings.TrimPrefix(req.FilePath, "./")
	mailbox := false
	switch name {
	case "template/example_recipients.csv", "template/example_recipients.xlsx":
		name = "maplemail_contacts.xlsx"
	case "template/mailboxes.xlsx":
		name = "maplemail_mailboxes.xlsx"
		mailbox = true
	default:
		return nil, fmt.Errorf("Unsupported import template")
	}
	principal, authErr := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if authErr != nil {
		return nil, authErr
	}
	if !principal.Super && (mailbox || !principal.Has("contact:import")) {
		return nil, fmt.Errorf("Import template access denied")
	}
	data, err := public.BuildImportWorkbook(mailbox, public.GetLanguageFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	request := g.RequestFromCtx(ctx)
	if request == nil {
		return nil, fmt.Errorf("Request context is unavailable")
	}
	request.Response.Header().Set("Content-Type", public.SpreadsheetMIME)
	request.Response.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	request.Response.Write(data)
	return nil, nil
}
