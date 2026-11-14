package batch_mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	v1 "billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/warmup"
	"github.com/gogf/gf/v2/frame/g"
)

var previewSender = mail_service.NewEmailSenderWithLocal

func CheckDeliveryWarmup(ctx context.Context, recipients []string) error {
	ip, e := public.GetServerIP()
	if e != nil {
		return errors.New("cannot determine sender IP")
	}
	if _, e = warmup.SenderIpWarmup().InitializeOrGetWarmupStatus(ctx, ip); e != nil {
		return e
	}
	for _, recipient := range recipients {
		allow, seconds, e := warmup.RateLimiter().Allow(ctx, ip, recipient)
		if e != nil {
			return errors.New("IP warmup status unavailable")
		}
		if !allow {
			return fmt.Errorf("IP warmup quota unavailable; retry after %d seconds", seconds)
		}
	}
	return nil
}

func SendPreview(ctx context.Context, p *governance.Principal, req *v1.SendTestEmailReq) error {
	if !p.Has("campaign:publish") {
		return errors.New("sending permission required")
	}
	if e := governance.SenderAllowed(ctx, p, req.Addresser); e != nil {
		return e
	}
	sourceContent, e := ResolveCampaignContent(ctx, p, int64(req.TemplateId), req.HTMLContent)
	if e != nil {
		return e
	}
	if strings.ContainsAny(req.Subject+req.FullName, "\r\n") {
		return errors.New("invalid email headers")
	}
	recipient, e := mail.ParseAddress(req.Recipient)
	if e != nil || recipient.Address != req.Recipient {
		return errors.New("invalid test recipient")
	}
	group := req.GroupID
	if len(req.GroupIDs) > 0 {
		group = req.GroupIDs[0]
	}
	if group > 0 {
		if e = p.Check(ctx, "bm_contact_groups", int64(group), "contact:list"); e != nil {
			return e
		}
	}
	contact := entity.Contact{Email: req.Recipient, GroupId: group}
	if group > 0 {
		if e = g.DB().Model("bm_contacts").Ctx(ctx).Where("email", req.Recipient).Where("group_id", group).Scan(&contact); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		contact.Email = req.Recipient
		contact.GroupId = group
	}
	unsubscribe := ""
	if req.Unsubscribe == 1 {
		unsubscribe, _, e = DeliveryLinks(ctx, req.Recipient, req.TemplateId, 0, group)
		if e != nil {
			return e
		}
	}
	source := strings.ReplaceAll(sourceContent, "__UNSUBSCRIBE_URL__", "{{ UnsubscribeURL . }}")
	content, e := GetTemplateEngine().RenderEmailTemplate(ctx, source, &contact, nil, unsubscribe)
	if e != nil {
		return e
	}
	subject, e := GetTemplateEngine().RenderEmailTemplate(ctx, req.Subject, &contact, nil, unsubscribe)
	if e != nil {
		return e
	}
	sender, e := previewSender(req.Addresser)
	if e != nil {
		return e
	}
	defer sender.Close()
	mid := sender.GenerateMessageID()
	content, headers, e := InstrumentDelivery(ctx, content, mid, req.Recipient, 0, req.TemplateId, group, TaskDeliveryOptions{Unsubscribe: req.Unsubscribe == 1, TrackOpen: req.TrackOpen == 1, TrackClick: req.TrackClick == 1})
	if e != nil {
		return e
	}
	if req.Warmup == 1 {
		if e = CheckDeliveryWarmup(ctx, []string{req.Recipient}); e != nil {
			return e
		}
	}
	message := mail_service.NewMessage(subject, content)
	message.SetRealName(req.FullName)
	message.SetMessageID(mid)
	message.SetHeaders(headers)
	return sender.SendOnce(message, []string{req.Recipient})
}
