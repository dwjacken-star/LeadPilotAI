package contactimport

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/public"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/util/gvalid"
	"github.com/xuri/excelize/v2"
	"strings"
)

type Row struct {
	Row     int      `json:"row"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Company string   `json:"company"`
	Country string   `json:"country"`
	Status  string   `json:"status"`
	Reason  string   `json:"reason"`
	Keyword string   `json:"keyword,omitempty"`
	Raw     []string `json:"-"`
}
type Result struct {
	Total            int               `json:"total"`
	Valid            int               `json:"valid"`
	Filtered         int               `json:"filtered"`
	Invalid          int               `json:"invalid"`
	Duplicate        int               `json:"duplicate"`
	Pending          int               `json:"pending"`
	ValidationSender string            `json:"validation_sender"`
	Token            string            `json:"token"`
	Rows             []Row             `json:"rows"`
	Contacts         []*entity.Contact `json:"-"`
	PendingContacts  []*entity.Contact `json:"-"`
	Headers          []string          `json:"-"`
}

func ReadLocal(ctx context.Context, data string, values []string) (*Result, error) {
	keywords, err := public.NormalizeImportKeywords(values)
	if err != nil {
		return nil, err
	}
	rows, err := public.ReadSpreadsheet(data)
	if err != nil {
		return nil, err
	}
	headers := make([]string, len(rows[0]))
	emailColumn := -1
	for i, key := range rows[0] {
		headers[i] = strings.ToLower(strings.TrimSpace(key))
		if headers[i] == "email" {
			emailColumn = i
		}
	}
	if emailColumn < 0 {
		return nil, errors.New("import_email_header")
	}
	encoded, _ := json.Marshal(keywords)
	hash := sha256.Sum256([]byte(data + "\x00" + string(encoded)))
	result := &Result{Rows: []Row{}, Contacts: []*entity.Contact{}, Headers: rows[0], Token: hex.EncodeToString(hash[:])}
	seen := map[string]bool{}
	for index, values := range rows[1:] {
		if strings.TrimSpace(strings.Join(values, "")) == "" {
			continue
		}
		get := func(i int) string {
			if i >= len(values) {
				return ""
			}
			return strings.TrimSpace(values[i])
		}
		row := Row{Row: index + 2, Email: strings.ToLower(get(emailColumn)), Status: "valid", Raw: values}
		attrs := map[string]string{}
		active := 1
		for i, key := range headers {
			switch key {
			case "name":
				row.Name = get(i)
			case "company":
				row.Company = get(i)
			case "country":
				row.Country = get(i)
			}
		}
		for _, word := range keywords {
			if strings.Contains(row.Email, word) {
				row.Status = "filtered"
				row.Reason = "filter_keyword"
				row.Keyword = word
				break
			}
		}
		if row.Status == "valid" && gvalid.New().Rules("email").Data(row.Email).Run(ctx) != nil {
			row.Status = "invalid"
			row.Reason = "invalid_email"
		}
		if row.Status == "valid" {
			if normalized, _, okay := emailvalidation.Normalize(row.Email); okay {
				row.Email = normalized
			}
		}
		if row.Status == "valid" {
			for i, key := range headers {
				value := get(i)
				if value == "" || key == "email" || key == "" {
					continue
				}
				switch key {
				case "active":
					if value != "0" && value != "1" {
						row.Status = "invalid"
						row.Reason = "invalid_active"
					} else if value == "0" {
						active = 0
					}
				case "attributes":
					var extra map[string]interface{}
					if json.Unmarshal([]byte(value), &extra) != nil || extra == nil {
						row.Status = "invalid"
						row.Reason = "invalid_attributes"
						continue
					}
					for k, v := range extra {
						if v == nil {
							attrs[k] = ""
						} else if text, ok := v.(string); ok {
							attrs[k] = text
						} else {
							raw, _ := json.Marshal(v)
							attrs[k] = string(raw)
						}
					}
				default:
					attrs[key] = value
				}
			}
		}
		if row.Status == "valid" && seen[row.Email] {
			row.Status = "duplicate"
			row.Reason = "duplicate_email"
		}
		switch row.Status {
		case "valid":
			seen[row.Email] = true
			result.Valid++
			result.Contacts = append(result.Contacts, &entity.Contact{Email: row.Email, Active: active, Attribs: attrs})
		case "filtered":
			result.Filtered++
		case "invalid":
			result.Invalid++
		case "duplicate":
			result.Duplicate++
		}
		result.Total++
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

func Read(ctx context.Context, data string, values []string) (*Result, error) {
	result, err := ReadLocal(ctx, data, values)
	if err != nil {
		return nil, err
	}
	hash, _ := hex.DecodeString(result.Token)
	// Validate each distinct domain once, after syntax/keyword/attribute checks.
	domains := []string{}
	seenDomains := map[string]bool{}
	for _, row := range result.Rows {
		if row.Status == "valid" {
			_, domain, ok := emailvalidation.Normalize(row.Email)
			if ok && !seenDomains[domain] {
				seenDomains[domain] = true
				domains = append(domains, domain)
			}
		}
	}
	checks := emailvalidation.Default.Domains(ctx, domains)
	accepted := map[string]bool{}
	pendingEmails := map[string]bool{}
	for i := range result.Rows {
		row := &result.Rows[i]
		if row.Status != "valid" {
			continue
		}
		_, domain, ok := emailvalidation.Normalize(row.Email)
		check := checks[domain]
		if !ok {
			check = emailvalidation.Result{Status: emailvalidation.Invalid, Code: "invalid_email"}
		}
		row.Reason = check.Code
		if check.Status == emailvalidation.Invalid {
			row.Status = "invalid"
			result.Valid--
			result.Invalid++
		} else if check.Status != emailvalidation.Valid {
			row.Status = "pending"
			result.Valid--
			result.Pending++
			pendingEmails[row.Email] = true
		} else {
			accepted[row.Email] = true
		}
	}
	contacts := result.Contacts[:0]
	for _, contact := range result.Contacts {
		if accepted[contact.Email] {
			contacts = append(contacts, contact)
		} else if pendingEmails[contact.Email] {
			result.PendingContacts = append(result.PendingContacts, contact)
		}
	}
	result.Contacts = contacts
	// A DNS change between preview and confirmation invalidates its proof as well.
	proof, _ := json.Marshal(result.Rows)
	finalHash := sha256.Sum256(append(hash[:], proof...))
	result.Token = hex.EncodeToString(finalHash[:])
	return result, nil
}

func (r *Result) ValidateConfirmation(token string, skipInvalid bool, keepPending ...bool) error {
	if (r.Pending > 0 || r.Invalid > 0 || skipInvalid) && token == "" {
		return errors.New("import_preview_required")
	}
	if token != "" && token != r.Token {
		return errors.New("import_preview_changed")
	}
	return nil
}

// Pending contacts require an explicit opt-in. Active verification jobs and
// invalid/filtered rows are never included in PendingContacts.
func (r *Result) ContactsForImport(keepPending bool) []*entity.Contact {
	if !keepPending {
		return r.Contacts
	}
	contacts := make([]*entity.Contact, 0, len(r.Contacts)+len(r.PendingContacts))
	contacts = append(contacts, r.Contacts...)
	return append(contacts, r.PendingContacts...)
}
func (r *Result) InvalidWorkbook() ([]byte, error) {
	return r.StatusWorkbook("invalid")
}
func (r *Result) StatusWorkbook(status string) ([]byte, error) {
	book := excelize.NewFile()
	defer book.Close()
	sheet := book.GetSheetName(0)
	header := []interface{}{"source_row"}
	for _, value := range r.Headers {
		header = append(header, value)
	}
	header = append(header, "error")
	_ = book.SetSheetRow(sheet, "A1", &header)
	line := 2
	for _, row := range r.Rows {
		if row.Status != status {
			continue
		}
		values := []interface{}{row.Row}
		for i := range r.Headers {
			value := ""
			if i < len(row.Raw) {
				value = row.Raw[i]
			}
			values = append(values, value)
		}
		values = append(values, row.Reason)
		_ = book.SetSheetRow(sheet, fmt.Sprintf("A%d", line), &values)
		line++
	}
	data, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}
