package abnormal_recipient

import (
	v1 "billionmail-core/api/abnormal_recipient/v1"
	"billionmail-core/internal/service/abnormal_recipient"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var scanning atomic.Bool

func ScanLogPath() string { return public.AbsPath("logs/check_email_valid.txt") }
func (c *ControllerV1) CheckGroup(ctx context.Context, req *v1.CheckGroupReq) (res *v1.CheckGroupRes, err error) {
	res = &v1.CheckGroupRes{}
	contacts, err := g.DB().Model("bm_contacts").Ctx(ctx).Where("group_id", req.GroupId).Where("active", 1).Fields("email").All()
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		res.SetError(gerror.New("No active contacts in this group"))
		return res, nil
	}
	if !scanning.CompareAndSwap(false, true) {
		res.SetError(gerror.New("verification_scan_running"))
		return res, nil
	}
	path := ScanLogPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if err = os.WriteFile(path, []byte("DNS/MX + SMTP verification started. Unknown results will never be deleted or blocklisted.\n"), 0600); err != nil {
		scanning.Store(false)
		return nil, err
	}
	sender, err := g.DB().Model("mailbox").Ctx(ctx).Where("active", 1).Order("username").Value("username")
	if err != nil {
		scanning.Store(false)
		return nil, err
	}
	go func() {
		defer scanning.Store(false)
		background := context.Background()
		emails := []string{}
		for _, contact := range contacts {
			emails = append(emails, contact["email"].String())
		}
		emailvalidation.InvalidateRecipients(background, emails)
		var mu sync.Mutex
		var wg sync.WaitGroup
		valid, invalid, unknown := 0, 0, 0
		bad := []abnormal_recipient.RecipientDetail{}
		jobs := make(chan string)
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for email := range jobs {
					check := emailvalidation.Check(background, email, sender.String(), true)
					mu.Lock()
					switch check.Status {
					case emailvalidation.Valid:
						valid++
					case emailvalidation.Invalid:
						invalid++
						bad = append(bad, abnormal_recipient.RecipientDetail{Email: email, ErrorReason: check.Code})
					default:
						unknown++
					}
					appendLogLine(path, fmt.Sprintf("[%d/%d] %s  %s  %s", valid+invalid+unknown, len(contacts), email, check.Status, check.Code))
					mu.Unlock()
				}
			}()
		}
		for _, contact := range contacts {
			jobs <- contact["email"].String()
		}
		close(jobs)
		wg.Wait()
		if req.Oper == 2 {
			_ = abnormal_recipient.BatchUpsertAbnormalRecipientsWithDetails(background, bad, 3, "Verified invalid DNS/MX or SMTP recipient")
		}
		if req.Oper == 3 && len(bad) > 0 {
			emails := []string{}
			for _, item := range bad {
				emails = append(emails, item.Email)
			}
			_, _ = g.DB().Model("bm_contacts").Where("group_id", req.GroupId).WhereIn("email", emails).Delete()
		}
		appendLogLine(path, fmt.Sprintf("Check finished. Accepted: %d, Invalid: %d, Unconfirmed: %d. Unconfirmed contacts were retained.", valid, invalid, unknown))
	}()
	res.SetSuccess("The process is currently running in the background. Please check the log.")
	return res, nil
}
func appendLogLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}
