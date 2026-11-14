package contactimport

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
)

func TestPersistentImportJobs(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, err := governance.Load(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	previous := emailvalidation.Default
	engine := emailvalidation.New(importResolver{})
	var probes atomic.Int32
	engine.Probe = func(_ context.Context, _ emailvalidation.Result, email, sender string) emailvalidation.Result {
		probes.Add(1)
		status, code := emailvalidation.Valid, "smtp_accepted"
		if strings.HasPrefix(email, "missing@") {
			status, code = emailvalidation.Invalid, "recipient_not_found"
		}
		if strings.HasPrefix(email, "catch@") {
			status, code = emailvalidation.Unknown, "catch_all"
		}
		if strings.HasPrefix(email, "policy@") {
			status, code = emailvalidation.Unknown, "smtp_policy"
		}
		return emailvalidation.Result{Status: status, Code: code, Stage: "smtp", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute)}
	}
	emailvalidation.Default = engine
	defer func() { emailvalidation.Default = previous }()
	book := excelize.NewFile()
	defer book.Close()
	_ = book.SetSheetRow("Sheet1", "A1", &[]interface{}{"email", "name"})
	emails := []string{}
	for i := 0; i < 25; i++ {
		emails = append(emails, fmt.Sprintf("buyer-%02d@route.test", i))
	}
	emails = append(emails, "missing@route.test", "catch@catch.test", "policy@policy.test", "later@temporary.test", "spam@route.test", "not-an-email", emails[0])
	for i, email := range emails {
		_ = book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &[]interface{}{email, fmt.Sprintf("Name %d", i)})
	}
	buffer, _ := book.WriteToBuffer()
	file := base64.StdEncoding.EncodeToString(buffer.Bytes())
	id, err := CreateJob(ctx, p, file, []string{"spam"}, "")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := JobPreview(ctx, p, id, "all", 1, false)
	if err != nil || probes.Load() != 0 || gconv.Int(initial["queued"]) != 29 || gconv.Int(initial["filtered"]) != 1 || gconv.Int(initial["duplicate"]) != 1 || len(gconv.SliceAny(initial["rows"])) != 20 {
		t.Fatal("initial preview must only parse/filter", initial, err, probes.Load())
	}
	duplicate, err := CreateJob(ctx, p, file, []string{"spam"}, "")
	if err != nil || duplicate != id {
		t.Fatal("file parsed as a second job", duplicate, err)
	}
	body, err := JobWorkbook(ctx, p, id, "pending")
	if err != nil {
		t.Fatal(err)
	}
	export, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := export.GetRows(export.GetSheetName(0))
	_ = export.Close()
	if len(rows) != 30 || rows[1][1] != emails[0] || rows[1][2] != "Name 0" || probes.Load() != 0 {
		t.Fatal("download reparsed/probed or lost original columns", rows)
	}
	first, err := claimImportRow(ctx)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	checking, err := JobPreview(ctx, p, id, "checking", 1, false)
	if err != nil || gconv.Int(checking["checking"]) != 1 {
		t.Fatal(checking, err)
	}
	processImportRow(ctx, first)
	approved, err := JobPreview(ctx, p, id, "all", 1, false)
	if err != nil || gconv.Int(approved["valid"]) != 1 {
		t.Fatal(approved, err)
	}
	token := gconv.String(approved["token"])
	for {
		job, e := claimImportRow(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if job == nil {
			break
		}
		processImportRow(ctx, job)
	}
	final, err := JobPreview(ctx, p, id, "unconfirmed", 1, false)
	if err != nil || !gconv.Bool(final["finished"]) || gconv.Int(final["valid"]) != 25 || gconv.Int(final["invalid"]) != 2 || gconv.Int(final["unconfirmed"]) != 3 || gconv.Int(final["active"]) != 0 || gconv.Int(final["row_total"]) != 3 {
		t.Fatal("terminal classification", final, err)
	}
	scan, err := Confirmed(ctx, p, file, []string{"spam"}, "", token)
	if err != nil || len(scan.ContactsForImport(true)) != 1 {
		t.Fatal("background verification changed the approved contact set", scan, err)
	}
	complete, err := Confirmed(ctx, p, file, []string{"spam"}, "", gconv.String(final["token"]))
	if err != nil || len(complete.ContactsForImport(false)) != 25 || len(complete.ContactsForImport(true)) != 28 || len(complete.PendingContacts) != 3 {
		t.Fatal("pending opt-in did not preserve the complete reviewed set", complete, err)
	}
	if _, err = Confirmed(ctx, p, file, []string{"different"}, "", token); err == nil {
		t.Fatal("changed keywords accepted")
	}
	count := probes.Load()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, e := JobPreview(ctx, p, id, "valid", 2, false)
			if e != nil || gconv.Int(page["row_total"]) != 25 || len(gconv.SliceAny(page["rows"])) != 5 {
				t.Error(page, e)
			}
		}()
	}
	wg.Wait()
	if probes.Load() != count {
		t.Fatal("pagination/polling restarted validation")
	}
	blocked, err := JobPreview(ctx, p, id, "all", 1, true)
	if err != nil || gconv.Int(blocked["retried"]) != 0 {
		t.Fatal("retry bypassed backoff", blocked, err)
	}
	for _, other := range []*governance.Principal{{ID: p.ID, DeptID: 999, Super: true}, {ID: 999, DeptID: p.DeptID, Super: true}} {
		if _, err = JobPreview(ctx, other, id, "all", 1, false); err == nil {
			t.Fatal("job escaped account/department scope")
		}
		if _, err = JobWorkbook(ctx, other, id, "pending"); err == nil {
			t.Fatal("download escaped account/department scope")
		}
	}
	_, err = g.DB().Model("bm_contact_import_rows").Where("job_id", id).Where("status", "unconfirmed").Data(g.Map{"retry_after": time.Now().Add(-time.Minute)}).Update()
	if err != nil {
		t.Fatal(err)
	}
	retry, err := JobPreview(ctx, p, id, "queued", 1, true)
	if err != nil || gconv.Int(retry["retried"]) != 3 || gconv.Int(retry["queued"]) != 3 || probes.Load() != count {
		t.Fatal("explicit retry did not only enqueue uncertain addresses", retry, err)
	}
	seen := map[int64]bool{}
	claimed := make(chan *importJob, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, e := claimImportRow(ctx)
			if e != nil {
				t.Error(e)
			}
			if job != nil {
				claimed <- job
			}
		}()
	}
	wg.Wait()
	close(claimed)
	for job := range claimed {
		if seen[job.ID] {
			t.Fatal("row claimed twice")
		}
		seen[job.ID] = true
	}
	if len(seen) != 3 {
		t.Fatal("parallel claims lost work", seen)
	}
	_, err = g.DB().Model("bm_contact_import_rows").Where("job_id", id).Where("status", "checking").Data(g.Map{"lease_until": time.Now().Add(-time.Minute)}).Update()
	if err != nil {
		t.Fatal(err)
	}
	if err = recoverImportRows(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := JobPreview(ctx, p, id, "unconfirmed", 1, false)
	if err != nil || gconv.Int(recovered["unconfirmed"]) != 3 || gconv.Int(recovered["queued"]) != 0 {
		t.Fatal("interrupted retries must stop for confirmation", recovered, err)
	}
	_, err = g.DB().Model("bm_contact_import_rows").Where("job_id", id).Where("status", "unconfirmed").Data(g.Map{"status": "queued"}).Update()
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateJob(ctx, p, file, []string{"spam", "unused"}, "")
	if err != nil || second == id {
		t.Fatal(second, err)
	}
	next, err := claimImportRow(ctx)
	if err != nil || next == nil {
		t.Fatal(next, err)
	}
	owner, err := g.DB().Model("bm_contact_import_rows").Where("id", next.ID).Value("job_id")
	if err != nil || owner.String() != second {
		t.Fatal("a large old import starved a new job", owner, err)
	}
	_, err = g.DB().Model("bm_contact_import_rows").Where("id", next.ID).Data(g.Map{"lease_until": time.Now().Add(-time.Minute)}).Update()
	if err != nil {
		t.Fatal(err)
	}
	if err = recoverImportRows(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := g.DB().Model("bm_contact_import_rows").Where("id", next.ID).Value("status")
	if err != nil || state.String() != "queued" {
		t.Fatal("first interrupted attempt did not resume", state, err)
	}
	_, err = g.DB().Model("bm_contact_import_jobs").Where("id", id).Data(g.Map{"expires_at": time.Now().Add(-time.Minute)}).Update()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = JobPreview(ctx, p, id, "all", 1, false); err == nil {
		t.Fatal("expired job remained accessible")
	}
	exercisePendingOnlyPreviewOptInAndSnapshotIdentity(t, ctx, p)
}

func exercisePendingOnlyPreviewOptInAndSnapshotIdentity(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	var err error
	book := excelize.NewFile()
	defer book.Close()
	for i, values := range [][]interface{}{{"email", "name"}, {"first@route.test", "First"}, {"second@route.test", "Second"}} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err = book.SetSheetRow("Sheet1", cell, &values); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := book.WriteToBuffer()
	file := base64.StdEncoding.EncodeToString(raw.Bytes())
	id, err := CreateJob(ctx, p, file, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.DB().Model("bm_contact_import_rows").Where("job_id", id).Where("source_row", 2).Data(g.Map{"status": "unconfirmed", "reason": "catch_all"}).Update(); err != nil {
		t.Fatal(err)
	}
	preview, err := JobPreview(ctx, p, id, "all", 1, false)
	if err != nil || gconv.String(preview["token"]) == "" || gconv.Int(preview["valid"]) != 0 || gconv.Int(preview["unconfirmed"]) != 1 {
		t.Fatal(preview, err)
	}
	token := gconv.String(preview["token"])
	scan, err := Confirmed(ctx, p, file, nil, "", token)
	if err != nil || len(scan.ContactsForImport(false)) != 0 || len(scan.ContactsForImport(true)) != 1 || scan.ContactsForImport(true)[0].Email != "first@route.test" {
		t.Fatal("pending-only choice or active-row exclusion failed", scan, err)
	}
	// Swap which row is unconfirmed without changing either category's count.
	if _, err = g.DB().Exec(ctx, `UPDATE bm_contact_import_rows SET status=CASE WHEN source_row=2 THEN 'queued' ELSE 'unconfirmed' END WHERE job_id=?`, id); err != nil {
		t.Fatal(err)
	}
	next, err := JobPreview(ctx, p, id, "all", 1, false)
	if err != nil || gconv.String(next["token"]) == token {
		t.Fatal("different pending sets reused a proof", next, err)
	}
	scan, err = Confirmed(ctx, p, file, nil, "", gconv.String(next["token"]))
	if err != nil || len(scan.ContactsForImport(true)) != 1 || scan.ContactsForImport(true)[0].Email != "second@route.test" {
		t.Fatal(scan, err)
	}
	old, err := Confirmed(ctx, p, file, nil, "", token)
	if err != nil || old.ContactsForImport(true)[0].Email != "first@route.test" {
		t.Fatal("previously approved set changed", old, err)
	}
}
