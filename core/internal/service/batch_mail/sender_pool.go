package batch_mail

import (
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"net/mail"
	"strings"
)

func NormalizeSenders(senders []string, legacy string) ([]string, error) {
	if len(senders) == 0 {
		senders = []string{legacy}
	}
	result := []string{}
	seen := map[string]bool{}
	for _, sender := range senders {
		sender = strings.ToLower(strings.TrimSpace(sender))
		parsed, e := mail.ParseAddress(sender)
		if e != nil || parsed.Address != sender {
			return nil, errors.New("Invalid sender address")
		}
		if !seen[sender] {
			result = append(result, sender)
			seen[sender] = true
		}
	}
	if len(result) == 0 || len(result) > 50 {
		return nil, errors.New("Select between 1 and 50 senders")
	}
	return result, nil
}
func NormalizeGroups(groups []int64, legacy int64) []int64 {
	if len(groups) == 0 && legacy > 0 {
		groups = []int64{legacy}
	}
	result := []int64{}
	seen := map[int64]bool{}
	for _, id := range groups {
		if id > 0 && !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}
func ValidCC(cc string) error {
	if cc == "" {
		return nil
	}
	address, err := mail.ParseAddress(cc)
	if err != nil || address.Address != cc || strings.ContainsAny(cc, "\r\n") {
		return errors.New("Invalid CC address")
	}
	return nil
}
func PoolContains(senders []string, legacy, sender string) bool {
	if len(senders) == 0 {
		senders = []string{legacy}
	}
	for _, candidate := range senders {
		if strings.EqualFold(candidate, sender) {
			return true
		}
	}
	return false
}
func DecodeSenders(value string) []string {
	var result []string
	_ = json.Unmarshal([]byte(value), &result)
	return result
}

func NextAPISender(ctx context.Context, id int, requested string) (string, error) {
	var sender string
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, err := tx.GetOne(`SELECT addresser,senders,sender_cursor FROM api_templates WHERE id=$1 AND active=1 FOR UPDATE`, id)
		if err != nil {
			return err
		}
		if len(row) == 0 {
			return errors.New("API is disabled")
		}
		pool, err := NormalizeSenders(DecodeSenders(row["senders"].String()), row["addresser"].String())
		if err != nil {
			return err
		}
		if requested != "" {
			if !PoolContains(pool, "", requested) {
				return errors.New("API cannot override the authorized sender pool")
			}
			sender = strings.ToLower(requested)
			return nil
		}
		cursor := row["sender_cursor"].Int64()
		sender = pool[cursor%int64(len(pool))]
		_, err = tx.Exec(`UPDATE api_templates SET sender_cursor=CASE WHEN sender_cursor>9000000000000000000 THEN 0 ELSE sender_cursor+1 END WHERE id=$1`, id)
		return err
	})
	return sender, err
}

func campaignAudience(c CampaignConfig) (string, error) {
	groups := NormalizeGroups(c.GroupIDs, c.GroupID)
	if len(groups) == 0 {
		return "", errors.New("Select at least one recipient group")
	}
	ids := []string{}
	for _, id := range groups {
		ids = append(ids, fmt.Sprint(id))
	}
	where := `c.group_id IN (` + strings.Join(ids, ",") + `) AND c.active=1 AND c.status=1`
	if len(c.TagIDs) > 0 {
		tags := []string{}
		seen := map[int64]bool{}
		for _, id := range c.TagIDs {
			if id <= 0 {
				return "", errors.New("Invalid tag")
			}
			if !seen[id] {
				tags = append(tags, fmt.Sprint(id))
				seen[id] = true
			}
		}
		sub := `SELECT lower(trim(sc.email)) FROM bm_contact_tags ct JOIN bm_contacts sc ON sc.id=ct.contact_id WHERE sc.active=1 AND sc.status=1 AND sc.group_id IN (` + strings.Join(ids, ",") + `) AND ct.tag_id IN (` + strings.Join(tags, ",") + `) GROUP BY lower(trim(sc.email))`
		if c.TagLogic == "AND" {
			sub += fmt.Sprintf(` HAVING count(DISTINCT ct.tag_id)=%d`, len(tags))
		}
		op := "IN"
		if c.TagLogic == "NOT" {
			op = "NOT IN"
		}
		where += ` AND lower(trim(c.email)) ` + op + ` (` + sub + `)`
	}
	return where, nil
}

func CheckSenderPool(ctx context.Context, p *governance.Principal, pool []string) error {
	for _, sender := range pool {
		if err := governance.SenderAllowed(ctx, p, sender); err != nil {
			return err
		}
	}
	return nil
}
func PreviewAudience(ctx context.Context, p *governance.Principal, c CampaignConfig) (int, error) {
	groups := NormalizeGroups(c.GroupIDs, c.GroupID)
	for _, id := range groups {
		if err := p.Check(ctx, "bm_contact_groups", id, "contact:list"); err != nil {
			return 0, err
		}
	}
	for _, id := range c.TagIDs {
		if err := p.Check(ctx, "bm_tags", id, "contact:list"); err != nil {
			return 0, err
		}
	}
	where, err := campaignAudience(c)
	if err != nil {
		return 0, err
	}
	value, err := g.DB().GetValue(ctx, `SELECT count(DISTINCT lower(trim(c.email))) FROM bm_contacts c WHERE `+where)
	return value.Int(), err
}
