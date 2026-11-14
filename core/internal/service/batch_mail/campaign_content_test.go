package batch_mail

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseCampaignContent(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	ctx = context.WithValue(ctx, "accountId", int64(1))
	original, e := g.DB().Model("email_templates").Where("id", 1).Value("content")
	if e != nil {
		t.Fatal(e)
	}
	html := `<div lang="en"><p style="margin:0 0 16px">Draft first paragraph.</p><p>Second <strong>formatted</strong> paragraph.</p></div>`
	for _, template := range []int{0, 1} {
		id, e := CreateTaskWithRecipients(ctx, &v1.CreateTaskReq{Subject: "Inline draft", FullName: "BESTMAPLE", Addresser: "sender@example.com", GroupId: 1, TemplateId: template, HTMLContent: &html, StartTime: int(time.Now().Add(48 * time.Hour).Unix()), Unsubscribe: 1}, 0)
		if e != nil {
			t.Fatal("inline create", e)
		}
		task, e := GetTaskInfo(ctx, id)
		if e != nil || task.SnapshotContent != html || task.TemplateId != template {
			t.Fatal("snapshot not round-tripped", task, e)
		}
	}
	library, e := g.DB().Model("email_templates").Where("id", 1).Value("content")
	if e != nil || library.String() != original.String() {
		t.Fatal("draft modified shared template", e)
	}
	for _, bad := range []string{" ", strings.Repeat("x", 1024*1024+1)} {
		if _, e := ResolveCampaignContent(ctx, p, 0, &bad); e == nil {
			t.Fatal("invalid content accepted")
		}
	}
	if _, e := ResolveCampaignContent(ctx, p, 0, nil); e == nil {
		t.Fatal("missing template and body accepted")
	}
	if _, e := ResolveCampaignContent(ctx, p, 999999, &html); e == nil {
		t.Fatal("template authorization bypassed by body override")
	}
}
