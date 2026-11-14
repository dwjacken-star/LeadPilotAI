package maillog_stat

import (
	"context"
	"errors"
	"strings"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

const OverviewPermission = "overview:view"

// Scope delivery identities, never mailbox grants: a shared sender or queue can
// contain mail belonging to several departments and owners. Unattributed legacy
// and direct Webmail deliveries remain visible only to system administrators.
func overviewDeliveryScope(ctx context.Context, p *governance.Principal) (string, error) {
	if p == nil || !p.Has(OverviewPermission) {
		return "", errors.New("无权查看")
	}
	predicate, err := p.Predicate(ctx, OverviewPermission, "resource")
	if err != nil {
		return "", err
	}
	if p.Super {
		return "", nil
	}
	return `EXISTS(SELECT 1 FROM mailstat_message_ids mid
 WHERE mid.postfix_message_id=sm.postfix_message_id AND (
 EXISTS(SELECT 1 FROM recipient_info ri JOIN email_tasks resource ON resource.id=ri.task_id
 WHERE btrim(ri.message_id,'<>')=btrim(mid.message_id,'<>')
 AND lower(ri.recipient)=lower(sm.recipient) AND ` + predicate + `) OR
 EXISTS(SELECT 1 FROM api_mail_logs al JOIN api_templates resource ON resource.id=al.api_id
 WHERE btrim(al.message_id,'<>')=btrim(mid.message_id,'<>')
 AND lower(al.recipient)=lower(sm.recipient) AND ` + predicate + `) OR
 EXISTS(SELECT 1 FROM bm_crm_messages resource
 WHERE resource.direction='outgoing' AND btrim(resource.message_id,'<>')=btrim(mid.message_id,'<>')
 AND lower(resource.to_address)=lower(sm.recipient) AND ` + predicate + `)))`, nil
}

func (o *Overview) OverviewForPrincipal(ctx context.Context, p *governance.Principal, campaignID int64, domain string, startTime, endTime int64) (map[string]interface{}, error) {
	scope, err := overviewDeliveryScope(ctx, p)
	if err != nil {
		return nil, err
	}
	result, err := o.overviewContext(ctx, campaignID, domain, startTime, endTime, scope, p.Super)
	if err == nil {
		result["queue_access"] = p.Super
	}
	return result, err
}

func (o *Overview) FailedListForPrincipal(ctx context.Context, p *governance.Principal, campaignID int64, domain string, startTime, endTime int64) ([]map[string]interface{}, error) {
	scope, err := overviewDeliveryScope(ctx, p)
	if err != nil {
		return nil, err
	}
	if p.Super {
		return o.FailedList(campaignID, domain, startTime, endTime), nil
	}
	startTime, endTime = o.filterAndPrepareTimeSection(startTime, endTime)
	// Deferred records have only queue identity, not recipient identity. Joining
	// them would expose another recipient's failure on a multi-recipient queue.
	query := g.DB().Model("mailstat_send_mails sm").Ctx(ctx).
		InnerJoin("mailstat_senders s", "s.postfix_message_id=sm.postfix_message_id").
		Where(scope).Where("sm.status != ?", "sent").
		Fields("sm.postfix_message_id,s.sender,sm.recipient,sm.status,sm.dsn,sm.relay,sm.description,sm.delay,sm.delays,sm.log_time")
	if startTime > 0 {
		query = query.Where("sm.log_time_millis > ?", startTime*1000)
	}
	if endTime > 0 {
		query = query.Where("sm.log_time_millis < ?", endTime*1000)
	}
	if domain != "" {
		query = query.Where("lower(split_part(s.sender,'@',2))=lower(?)", domain)
	}
	if campaignID > 0 {
		query = query.Where(`EXISTS(SELECT 1 FROM mailstat_message_ids mid JOIN recipient_info ri ON btrim(ri.message_id,'<>')=mid.message_id
 WHERE mid.postfix_message_id=sm.postfix_message_id AND ri.task_id=? AND lower(ri.recipient)=lower(sm.recipient))`, campaignID)
	}
	rows, err := query.OrderDesc("sm.log_time_millis").All()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Map())
	}
	return result, nil
}

// Domain options come from precisely the deliveries visible in this Overview,
// not the global domain inventory or unrelated sender grants.
func (o *Overview) DomainsForPrincipal(ctx context.Context, p *governance.Principal) ([]string, error) {
	scope, err := overviewDeliveryScope(ctx, p)
	if err != nil {
		return nil, err
	}
	query := `SELECT domain FROM domain`
	if !p.Super {
		query = `SELECT DISTINCT lower(split_part(s.sender,'@',2)) AS domain
 FROM mailstat_send_mails sm JOIN mailstat_senders s ON s.postfix_message_id=sm.postfix_message_id
 WHERE ` + scope + ` ORDER BY domain`
	}
	rows, err := g.DB().GetAll(ctx, query)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		if domain := strings.TrimSpace(row["domain"].String()); domain != "" {
			result = append(result, domain)
		}
	}
	return result, nil
}
