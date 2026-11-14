package contactimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

type previewSnapshot struct {
	Summary         *Result           `json:"summary"`
	Contacts        []*entity.Contact `json:"contacts"`
	PendingContacts []*entity.Contact `json:"pending_contacts,omitempty"`
}

func sourceHash(data string, keywords []string) string {
	normalized, err := public.NormalizeImportKeywords(keywords)
	if err != nil {
		return ""
	}
	encoded, _ := json.Marshal(normalized)
	hash := sha256.Sum256(append([]byte(data+"\x00"), encoded...))
	return hex.EncodeToString(hash[:])
}
func storePreview(ctx context.Context, p *governance.Principal, data string, keywords []string, scan *Result) error {
	return storePreviewHash(ctx, p, sourceHash(data, keywords), scan)
}

func storePreviewHash(ctx context.Context, p *governance.Principal, hash string, scan *Result) error {
	body, err := json.Marshal(previewSnapshot{Summary: scan, Contacts: scan.Contacts, PendingContacts: scan.PendingContacts})
	if err != nil {
		return err
	}
	_, err = g.DB().Model("bm_contact_import_previews").Ctx(ctx).Data(g.Map{"token": scan.Token, "account_id": p.ID, "dept_id": p.DeptID, "source_hash": hash, "validation_sender": scan.ValidationSender, "result": string(body), "expires_at": time.Now().Add(30 * time.Minute)}).OnConflict("token,account_id,dept_id").Save()
	if err != nil {
		return err
	}
	_, err = g.DB().Exec(ctx, `DELETE FROM bm_contact_import_previews WHERE expires_at<now() OR(account_id=$1 AND source_hash=$2 AND created_at<now()-interval '2 minutes' AND token NOT IN(SELECT token FROM bm_contact_import_previews WHERE account_id=$1 AND source_hash=$2 ORDER BY created_at DESC LIMIT 5))`, p.ID, hash)
	return err
}

// Confirmation imports precisely the chosen categories in the approved preview.
// Background verification of other rows cannot silently expand or invalidate it.
func Confirmed(ctx context.Context, p *governance.Principal, data string, keywords []string, sender, token string) (*Result, error) {
	if token == "" {
		scan, err := ReadForPrincipal(ctx, p, data, keywords, sender)
		if err != nil {
			return nil, err
		}
		if err = scan.ValidateConfirmation("", false); err != nil {
			return nil, err
		}
		return scan, nil
	}
	row, err := g.DB().Model("bm_contact_import_previews").Ctx(ctx).Where("token", token).Where("account_id", p.ID).Where("dept_id", p.DeptID).Where("expires_at>?", time.Now()).One()
	if err != nil {
		return nil, err
	}
	if len(row) == 0 || row["source_hash"].String() != sourceHash(data, keywords) {
		return nil, errors.New("import_preview_changed")
	}
	selected := row["validation_sender"].String()
	if sender != "" && strings.ToLower(strings.TrimSpace(sender)) != selected {
		return nil, errors.New("import_preview_changed")
	}
	if _, err = ResolveSender(ctx, p, selected); err != nil {
		return nil, err
	}
	var snapshot previewSnapshot
	if json.Unmarshal(row["result"].Bytes(), &snapshot) != nil || snapshot.Summary == nil {
		return nil, errors.New("import_preview_changed")
	}
	snapshot.Summary.Contacts = snapshot.Contacts
	snapshot.Summary.PendingContacts = snapshot.PendingContacts
	return snapshot.Summary, nil
}
