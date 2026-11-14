package growth

import (
	"billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
	"strings"
	"unicode"
)

type MasterReplyRule struct {
	Questions []string `json:"questions"`
	Answer    string   `json:"answer"`
	Language  string   `json:"language"`
}

func masterQuestion(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(value))
}
func masterContractual(value string) bool {
	return masterExcluded(value, []string{"price", "quotation", "quote", "warranty", "guarantee", "payment", "credit", "contract", "refund", "discount", "报价", "价格", "质保", "账期", "合同", "保证", "赔偿", "価格", "契約", "保証", "支払", "prix", "preis", "garantie", "precio", "cotización"})
}
func masterReplyRule(c MasterConfig, body, language string, human bool) (string, bool) {
	if !c.AutoReply || human || masterContractual(body) {
		return "", false
	}
	question := masterQuestion(body)
	for _, rule := range c.ReplyRules {
		if rule.Language != language {
			continue
		}
		for _, q := range rule.Questions {
			if question != "" && question == masterQuestion(q) {
				return rule.Answer, true
			}
		}
	}
	return "", false
}

var MasterReplyMail = crm.Reply

func (x *masterExec) autoReply(event gdb.Record, analysis g.Map, language string, human bool) (int64, error) {
	if event["channel"].String() != "email" {
		return 0, nil
	}
	answer, ok := masterReplyRule(x.Snapshot.Config, event["body"].String(), language, human)
	if !ok {
		return 0, nil
	}
	if old := gconv.Int64(analysis["automatic_reply_id"]); old > 0 {
		return old, nil
	}
	if e := masterRequired(x.P, "crm:reply", "intelligence:publish"); e != nil {
		return 0, e
	}
	reviewer, e := governance.Load(x.Ctx, x.Snapshot.Config.ReplyApprovedBy)
	if e != nil || !reviewer.Has("intelligence:review") {
		return 0, errors.New("automatic reply rule reviewer permission revoked")
	}
	if reviewer.Super {
		reviewer.DeptID = x.P.DeptID
	}
	if e = reviewer.CheckOwnership(x.Ctx, "intelligence:review", x.Run["dept_id"].Int64(), x.Run["owner_id"].Int64()); e != nil {
		return 0, e
	}
	if e = x.check(); e != nil {
		return 0, e
	}
	if e = x.sourceCurrent(); e != nil {
		return 0, e
	}
	payload := jsonMap(event["payload"])
	messageID := gconv.Int64(payload["message_id"])
	if messageID < 1 {
		return 0, nil
	}
	message, e := crm.Message(x.Ctx, x.P, messageID)
	if e != nil {
		return 0, e
	}
	if strings.ToLower(message["from_address"].String()) != strings.ToLower(event["sender"].String()) || message["direction"].String() != "incoming" {
		return 0, errors.New("automatic reply must reference the actual inbound customer")
	}
	blocked, e := deliveryprefs.IsSuppressed(x.Ctx, message["mailbox"].String(), event["sender"].String())
	if e != nil {
		return 0, e
	}
	if blocked {
		return 0, nil
	}
	if _, e = x.reserve("send", 0); e != nil {
		return 0, e
	}
	key := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("maplemail-growth-reply:%d", event["id"].Int64()))).String()
	id, e := MasterReplyMail(x.Ctx, x.P, messageID, paragraphHTML(strings.Split(answer, "\n\n")), key)
	if id > 0 {
		analysis["automatic_reply_id"] = id
		analysis["automatic_reply_rule_approved_by"] = x.Snapshot.Config.ReplyApprovedBy
		status := "sent"
		if e != nil {
			status = "unknown"
		}
		saved, readErr := g.DB().Model("bm_crm_messages").Ctx(context.WithoutCancel(x.Ctx)).Where("id", id).Value("status")
		if readErr == nil && !saved.IsEmpty() {
			status = saved.String()
		}
		analysis["automatic_reply_status"] = status
		_, err := g.DB().Model("bm_gm_events").Ctx(context.WithoutCancel(x.Ctx)).Where("id", event["id"]).Data("analysis", jsonText(analysis)).Update()
		if err != nil {
			return id, err
		}
	}
	return id, e
}
