package contactimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/smtpidentity"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func ResolveSender(ctx context.Context, p *governance.Principal, requested string) (string, error) {
	sender := strings.ToLower(strings.TrimSpace(requested))
	if sender == "" {
		query := g.DB().Model("mailbox").Ctx(ctx).Where("active", 1)
		if !p.Super {
			query = query.Where("username IN(SELECT sender FROM bm_resource_grants WHERE dept_id=? AND active)", p.DeptID)
		}
		value, err := query.Order(gdb.Raw(smtpidentity.SenderOrder)).Value("username")
		if err != nil {
			return "", err
		}
		sender = value.String()
	}
	if sender == "" {
		return "", errors.New("import_sender_required")
	}
	if err := governance.SenderAllowed(ctx, p, sender); err != nil {
		return "", err
	}
	return sender, nil
}

func ReadForPrincipal(ctx context.Context, p *governance.Principal, data string, keywords []string, requested string) (*Result, error) {
	sender, err := ResolveSender(ctx, p, requested)
	if err != nil {
		return nil, err
	}
	scan, err := Read(ctx, data, keywords)
	if err != nil {
		return nil, err
	}
	scan.ValidationSender = sender
	emails := []string{}
	for _, row := range scan.Rows {
		if row.Status == "valid" {
			emails = append(emails, row.Email)
		}
	}
	checks := emailvalidation.Recipients(ctx, sender, emails)
	accepted := map[string]bool{}
	pending := map[string]bool{}
	for i := range scan.Rows {
		row := &scan.Rows[i]
		if row.Status != "valid" {
			continue
		}
		check := checks[row.Email]
		row.Reason = check.Code
		switch check.Status {
		case emailvalidation.Valid:
			accepted[row.Email] = true
		case emailvalidation.Invalid:
			row.Status = "invalid"
			scan.Valid--
			scan.Invalid++
		default:
			row.Status = "pending"
			pending[row.Email] = true
			scan.Valid--
			scan.Pending++
		}
	}
	contacts := scan.Contacts[:0]
	for _, contact := range scan.Contacts {
		if accepted[contact.Email] {
			contacts = append(contacts, contact)
		} else if pending[contact.Email] {
			scan.PendingContacts = append(scan.PendingContacts, contact)
		}
	}
	scan.Contacts = contacts
	proof, _ := json.Marshal(scan.Rows)
	hash := sha256.Sum256(append([]byte(scan.Token+"\x00"+sender), proof...))
	scan.Token = hex.EncodeToString(hash[:])
	if err = storePreview(ctx, p, data, keywords, scan); err != nil {
		return nil, err
	}
	return scan, nil
}
