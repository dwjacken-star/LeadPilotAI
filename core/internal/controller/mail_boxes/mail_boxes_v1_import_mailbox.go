package mail_boxes

import (
	"billionmail-core/api/mail_boxes/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/mail_boxes"
	"billionmail-core/internal/service/public"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gvalid"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) ImportMailbox(ctx context.Context, req *v1.ImportMailboxReq) (res *v1.ImportMailboxRes, err error) {
	res = &v1.ImportMailboxRes{}
	filtered := 0
	keywords, keywordErr := public.NormalizeImportKeywords(req.FilterKeywords)
	if keywordErr != nil {
		res.SetError(keywordErr)
		return res, nil
	}

	// default  CSV
	if req.FileType == "" {
		req.FileType = "csv"
	}

	//// 1. base64
	//raw, err := base64.StdEncoding.DecodeString(req.FileData)
	//if err != nil {
	//	res.SetError(gerror.New(public.LangCtx(ctx, "文件内容解码失败")))
	//	return res, nil
	//}
	//content := string(raw)

	excelImport := req.FileType == "xlsx"
	content := req.FileData
	if req.FileType == "xlsx" {
		var decodeErr error
		content, decodeErr = mailboxWorkbookCSV(content, func(email string) bool {
			if public.MatchesImportKeywords(email, keywords) {
				filtered++
				return true
			}
			return false
		})
		if decodeErr != nil {
			res.SetError(decodeErr)
			return res, nil
		}
		req.FileType = "csv"
	}

	var mailboxes []v1.Mailbox
	switch req.FileType {
	case "json":
		if err := json.Unmarshal([]byte(content), &mailboxes); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "JSON parsing failed")))
			return res, nil
		}
	case "csv":
		r := csv.NewReader(strings.NewReader(content))
		records, err := r.ReadAll()
		if err != nil || (len(records) < 2 && filtered == 0) {
			res.SetError(gerror.New(public.LangCtx(ctx, "CSV parsing failed")))
			return res, nil
		}
		header := records[0]
		for _, row := range records[1:] {
			m := v1.Mailbox{}
			for i, col := range header {
				if i >= len(row) {
					continue
				}
				switch col {
				case "username":
					m.Username = row[i]
				case "full_name":
					m.FullName = row[i]
				case "is_admin":
					m.IsAdmin = parseInt(row[i])
				case "quota":
					m.Quota = int64(parseInt(row[i]))
				case "quota_active":
					m.QuotaActive = parseInt(row[i])
				case "local_part":
					m.LocalPart = row[i]
				case "domain":
					m.Domain = row[i]
				case "active":
					m.Active = parseInt(row[i])
				case "password":
					m.Password = row[i]
				case "password_encode":
					m.PasswordEncode = row[i]
				}
			}
			mailboxes = append(mailboxes, m)
		}
	case "txt":
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			mailboxes = append(mailboxes, v1.Mailbox{Username: line})
		}
	default:
		res.SetError(gerror.New(public.LangCtx(ctx, "Unsupported file type")))
		return res, nil
	}

	if len(mailboxes) == 0 && filtered == 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to parse valid email data")))
		return res, nil
	}

	originalTotal := len(mailboxes) + filtered
	kept := mailboxes[:0]
	for _, item := range mailboxes {
		if public.MatchesImportKeywords(item.Username, keywords) {
			filtered++
			continue
		}
		kept = append(kept, item)
	}
	mailboxes = kept
	for i := range mailboxes {
		// If FullName is empty, use LocalPart instead
		if mailboxes[i].FullName == "" && mailboxes[i].LocalPart != "" {
			mailboxes[i].FullName = mailboxes[i].LocalPart
		}

		if mailboxes[i].LocalPart == "" && mailboxes[i].Username != "" {
			if parts := strings.Split(mailboxes[i].Username, "@"); len(parts) == 2 {
				mailboxes[i].LocalPart = parts[0]
				mailboxes[i].Domain = parts[1]
			}
		}

		if mailboxes[i].Domain == "" && mailboxes[i].Username != "" {
			if parts := strings.Split(mailboxes[i].Username, "@"); len(parts) == 2 {
				mailboxes[i].Domain = parts[1]
			}
		}

		if mailboxes[i].Password == "" && mailboxes[i].PasswordEncode == "" {
			password, passwordErr := mail_boxes.ResolveMailboxPassword("")
			if passwordErr != nil {
				res.SetError(fmt.Errorf("Row %d: %w", i+2, passwordErr))
				return res, nil
			}
			mailboxes[i].Password = password
		}

		if mailboxes[i].IsAdmin == 0 {
			mailboxes[i].IsAdmin = 0
		}
		if mailboxes[i].Quota == 0 {
			mailboxes[i].Quota = 1000
		}
		if mailboxes[i].Active == 0 && !excelImport {
			mailboxes[i].Active = 1
		}

		mailboxes[i].QuotaActive = 1
		m := &mailboxes[i]
		if gvalid.New().Rules("email").Data(m.Username).Run(ctx) != nil || m.LocalPart == "" || strings.ContainsAny(m.LocalPart, "/\\") {
			res.SetError(fmt.Errorf("Row %d: invalid mailbox address", i+2))
			return res, nil
		}
		exists, e := g.DB().Model("domain").Ctx(ctx).Where("domain", m.Domain).Exist()
		if e != nil {
			return nil, e
		}
		if !exists {
			res.SetError(fmt.Errorf("Row %d: mailbox domain is not configured", i+2))
			return res, nil
		}
		if m.PasswordEncode == "" && len([]rune(m.Password)) < 8 {
			res.SetError(fmt.Errorf("Row %d: password requires at least 8 characters", i+2))
			return res, nil
		}
	}

	var failed []string
	emails := []string{}
	for _, m := range mailboxes {
		emails = append(emails, m.Username)
	}
	if e := emailvalidation.RequireDomains(ctx, emails); e != nil {
		res.SetError(e)
		return res, nil
	}
	var successCount int
	skipped := 0
	for _, m := range mailboxes {

		exists, e := g.DB().Model("mailbox").Ctx(ctx).Where("username", strings.ToLower(m.Username)).Exist()
		if e != nil {
			return nil, e
		}
		if exists {
			skipped++
			continue
		}
		if err := mail_boxes.AddImport(ctx, &m); err != nil {
			failed = append(failed, m.Username)
			continue
		}
		successCount++
	}

	importResult := map[string]interface{}{
		"total":          originalTotal,
		"filtered_count": filtered,
		"success":        successCount, "skipped": skipped,
		"failed":      len(failed),
		"failed_list": failed,
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Mailboxes,
		Log:  fmt.Sprintf("Import of email account was successful. Total: %d, Success: %d, Failed: %d", len(mailboxes), successCount, len(failed)),
		Data: importResult,
	})

	res.Data = importResult
	if len(failed) > 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "The import is complete, but {} email addresses failed to be imported", len(failed))))
	} else {
		res.SetSuccess(public.LangCtx(ctx, "Import completed. A total of {} email addresses were imported.", successCount))
	}

	return res, nil
}

// String to int conversion
func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
