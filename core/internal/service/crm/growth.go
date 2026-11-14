package crm

import (
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
)

func headerJSON(row map[string]interface{}) string {
	h := map[string]string{}
	for key, value := range row {
		if strings.HasPrefix(key, "hdr.") {
			h[strings.TrimPrefix(key, "hdr.")] = strings.TrimSpace(text(map[string]interface{}{key: value}, key))
		}
	}
	b, _ := json.Marshal(h)
	return string(b)
}
func mailHeaderJSON(header mail.Header) string {
	h := map[string]string{}
	for key, values := range header {
		h[strings.ToLower(key)] = strings.Join(values, " ")
	}
	b, _ := json.Marshal(h)
	return string(b)
}

// RawMessage preserves the original MIME including attachments for forwarding.
func RawMessage(ctx context.Context, p *governance.Principal, id int64) ([]byte, error) {
	r, e := Message(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if r["direction"].String() != "incoming" {
		return nil, errors.New("incoming message required")
	}
	results, e := doveadm(ctx, "fetch", "-u", r["mailbox"].String(), "text", "mailbox", "INBOX", "guid", r["source_guid"].String())
	if e != nil {
		return nil, e
	}
	if len(results) == 0 {
		return nil, errors.New("original message unavailable")
	}
	raw := []byte(text(results[0], "text"))
	if len(raw) > 20*1024*1024 {
		return nil, errors.New("original message exceeds 20 MB")
	}
	return raw, nil
}
