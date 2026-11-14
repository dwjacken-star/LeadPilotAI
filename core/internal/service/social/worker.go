package social

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func queue(ctx context.Context, p *governance.Principal, planID, accountID int64, kind, resource string, payload g.Map) (int64, error) {
	var plan, account interface{}
	if planID > 0 {
		plan = planID
	}
	if accountID > 0 {
		account = accountID
	}
	key := kind + ":" + uuid.NewString()
	if kind == "generate" {
		key = kind + ":" + gconv.String(planID) + ":" + gconv.String(payload["version"])
	}
	if kind == "sync" {
		if payload == nil {
			payload = g.Map{}
		}
		if gconv.Int64(payload["after_id"]) > 0 {
			key = "sync-page:" + gconv.String(accountID) + ":" + hashString(gconv.String(payload["cutoff"])) + ":" + gconv.String(payload["after_id"])
		} else {
			if payload["cutoff"] == nil {
				payload["cutoff"] = time.Now().UTC().Format(time.RFC3339Nano)
			}
			key = kind + ":" + gconv.String(accountID) + ":" + time.Now().UTC().Format("200601021504")
		}
	}
	if kind == "analyze" {
		hash, e := g.DB().Model("bm_social_interactions").Ctx(ctx).Where("id", resource).Value("content_hash")
		if e != nil {
			return 0, e
		}
		key = kind + ":" + resource + ":" + hash.String()
	}
	_, e := g.DB().Model("bm_social_jobs").Ctx(ctx).Data(g.Map{"plan_id": plan, "account_id": account, "owner_id": p.ID, "dept_id": p.DeptID, "kind": kind, "resource_id": resource, "payload": encode(payload), "dedupe_key": key}).OnConflict("dedupe_key").InsertIgnore()
	if e != nil {
		return 0, e
	}
	id, e := g.DB().Model("bm_social_jobs").Ctx(ctx).Where("dedupe_key", key).Value("id")
	return id.Int64(), e
}
func workerUser(ctx context.Context, id, dept int64, permission string) (*governance.Principal, error) {
	p, e := governance.Load(ctx, id)
	if e != nil {
		return nil, e
	}
	if p.Super {
		p.DeptID = dept
	}
	if p.DeptID != dept {
		return nil, errors.New("owner department changed")
	}
	if e = require(p, permission); e != nil {
		return nil, e
	}
	return p, nil
}
func ProcessJob(ctx context.Context) (bool, error) {
	token := uuid.NewString()
	var row gdb.Record
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		row, e = tx.GetOne(`SELECT j.* FROM bm_social_jobs j WHERE j.status='queued' AND j.deleted_at IS NULL AND j.next_at<=now() AND (j.plan_id IS NULL OR EXISTS(SELECT 1 FROM bm_social_plans p WHERE p.id=j.plan_id AND p.status<>'deleted')) ORDER BY j.id LIMIT 1 FOR UPDATE SKIP LOCKED`)
		if e != nil || len(row) == 0 {
			return e
		}
		_, e = tx.Model("bm_social_jobs").Where("id", row["id"]).Data(g.Map{"status": "running", "lease_token": token, "lease_until": time.Now().Add(6 * time.Minute), "attempts": gdb.Raw("attempts+1"), "updated_at": time.Now()}).Update()
		return e
	})
	if e != nil || len(row) == 0 {
		return false, e
	}
	permission := "social:content:edit"
	if row["kind"].String() == "sync" {
		permission = "social:interaction:view"
		if object(row["payload"])["permission"] == "social:report:view" {
			permission = "social:report:view"
		}
	}
	if row["kind"].String() == "analyze" {
		permission = "social:interaction:analyze"
	}
	p, e := workerUser(ctx, row["owner_id"].Int64(), row["dept_id"].Int64(), permission)
	if e == nil {
		switch row["kind"].String() {
		case "ingest":
			e = ingestMaterial(ctx, p, row["resource_id"].String())
		case "generate":
			e = generatePlan(ctx, p, row["plan_id"].Int64(), gconv.Int(object(row["payload"])["version"]))
		case "image_edit":
			e = processPostImage(ctx, p, row)
		case "sync":
			syncCtx := context.WithValue(ctx, syncBatchKey{}, object(row["payload"]))
			e = SyncAccount(syncCtx, p, row["account_id"].Int64(), permission)
		case "analyze":
			e = AnalyzeInteraction(ctx, p, row["resource_id"].Int64())
		default:
			e = errors.New("unsupported job")
		}
	}
	state, errorText := "completed", ""
	if errors.Is(e, generationMore) {
		state = "queued"
		e = nil
	}
	if e != nil {
		state = "failed"
		errorText = str(e.Error(), 500)
		if strings.Contains(errorText, "configuration") || strings.Contains(errorText, "configured") || strings.Contains(errorText, "disabled") || strings.Contains(errorText, "budget") {
			state = "configuration_required"
		}
		if row["kind"].String() == "generate" {
			_, _ = g.DB().Model("bm_social_plans").Where("id", row["plan_id"]).Where("version", gconv.Int(object(row["payload"])["version"])).Data(g.Map{"status": "draft", "error": errorText}).Update()
		}
	}
	_, err := g.DB().Model("bm_social_jobs").Where("id", row["id"]).Where("lease_token", token).Where("status", "running").Data(g.Map{"status": state, "error": errorText, "lease_token": nil, "lease_until": nil, "updated_at": time.Now(), "next_at": time.Now().Add(time.Second), "attempts": 0}).Update()
	return true, err
}
func recoverWork(ctx context.Context) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`UPDATE bm_social_deliveries d SET status='retry',error='resuming durable media preparation or read-only upload reconciliation',lease_token=NULL,lease_until=NULL,next_at=now() WHERE d.status='publishing' AND d.lease_until<now() AND EXISTS(SELECT 1 FROM bm_social_provider_operations o WHERE o.delivery_id=d.id AND (COALESCE(o.progress->>'pending','')='' OR (EXISTS(SELECT 1 FROM bm_social_accounts a WHERE a.id=d.account_id AND a.platform='facebook') AND ((o.progress->>'pending'='upload_video' AND COALESCE(o.progress->>'upload_session','')<>'') OR (o.progress->>'pending' IN('upload_reel','finish_reel') AND COALESCE(o.progress->>'video_id','')<>'')))))`); e != nil {
			return e
		}
		if _, e := tx.Exec(`UPDATE bm_social_deliveries SET status='unknown',error='worker stopped while publishing; verify the Page before any retry',lease_token=NULL,lease_until=NULL WHERE status='publishing' AND lease_until<now()`); e != nil {
			return e
		}
		_, e := tx.Exec(`UPDATE bm_social_jobs SET status=CASE WHEN kind='image_edit' OR attempts>=3 THEN 'failed' ELSE 'queued' END,error='worker lease expired; review results before retrying',lease_token=NULL,lease_until=NULL,next_at=now() WHERE status='running' AND lease_until<now()`)
		return e
	})
}
func ProcessDelivery(ctx context.Context) (bool, error) {
	token := uuid.NewString()
	var row gdb.Record
	manualHandled := false
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		plan, e := tx.GetOne(`SELECT p.* FROM bm_social_plans p WHERE p.status='scheduled' AND EXISTS(SELECT 1 FROM bm_social_deliveries d WHERE d.plan_id=p.id AND d.deleted_at IS NULL AND d.status IN('scheduled','retry') AND d.due_at<=now() AND d.next_at<=now()) ORDER BY p.next_at,p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`)
		if e != nil || len(plan) == 0 {
			return e
		}
		row, e = tx.GetOne(`SELECT d.*,p.dept_id,p.version AS live_plan_version,p.config,p.status AS plan_status FROM bm_social_deliveries d JOIN bm_social_plans p ON p.id=d.plan_id WHERE p.id=$1 AND d.deleted_at IS NULL AND d.status IN('scheduled','retry') AND d.due_at<=now() AND d.next_at<=now() AND p.status='scheduled' ORDER BY d.due_at,d.id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`, plan["id"])
		if e != nil || len(row) == 0 {
			return e
		}
		if publishingMode(row) == "manual" {
			manualHandled = true
			state, reason := "manual_due", ""
			p, guardErr := workerUser(ctx, row["publisher_id"].Int64(), row["dept_id"].Int64(), "social:publish")
			if guardErr == nil {
				_, guardErr = tx.GetOne(`SELECT id FROM bm_social_accounts WHERE id=$1 FOR UPDATE`, row["account_id"])
			}
			var account gdb.Record
			if guardErr == nil {
				account, guardErr = accountAccess(ctx, p, row["account_id"].Int64(), "social:publish", true)
			}
			if guardErr == nil {
				_, guardErr = scoped(ctx, p, "bm_social_plans", plan["id"].Int64(), "social:publish")
			}
			if guardErr == nil {
				guardErr = manualDeliveryGuard(ctx, p, plan, row, account)
			}
			if guardErr != nil {
				state, reason = "blocked", guardErr.Error()
			}
			_, e = tx.Model("bm_social_deliveries").Where("id", row["id"]).Data(g.Map{"status": state, "error": str(reason, 500), "lease_token": nil, "lease_until": nil}).Update()
			return e
		}
		_, e = tx.Model("bm_social_deliveries").Where("id", row["id"]).Data(g.Map{"status": "publishing", "lease_token": token, "lease_until": time.Now().Add(6 * time.Minute), "attempts": gdb.Raw("attempts+1")}).Update()
		return e
	})
	if e != nil || len(row) == 0 {
		return false, e
	}
	if manualHandled {
		return true, nil
	}
	finish := func(state, reason string, remote PublishResult, next time.Time) error {
		values := g.Map{"status": state, "error": str(reason, 500), "lease_token": nil, "lease_until": nil, "next_at": next}
		if remote.ID != "" {
			values["remote_id"] = remote.ID
			values["remote_url"] = remote.URL
			values["submitted_at"] = time.Now()
		}
		if state == "published" {
			values["published_at"] = time.Now()
		}
		_, e := g.DB().Model("bm_social_deliveries").Where("id", row["id"]).Where("lease_token", token).Where("status", "publishing").Data(values).Update()
		return e
	}
	p, e := workerUser(ctx, row["publisher_id"].Int64(), row["dept_id"].Int64(), "social:publish")
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	account, e := accountAccess(ctx, p, row["account_id"].Int64(), "social:publish", true)
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	platform := account["platform"].String()
	if publishingMode(account) != "automatic" || (platform == "facebook" && account["target_type"].String() == "person") {
		return true, finish("blocked", "delivery publishing mode does not match account", PublishResult{}, time.Now())
	}
	if _, e = requirePlatform(ctx, platform, true); e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	access, e := tokenFor(account)
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	if !gconv.Bool(object(account["capabilities"])["publish"]) {
		return true, finish("blocked", "account has no publishing permission", PublishResult{}, time.Now())
	}
	plan, e := scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:publish")
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	c, e := planConfig(plan)
	if e != nil {
		return true, finish("failed", "invalid plan configuration", PublishResult{}, time.Now())
	}
	v, e := g.DB().Model("bm_social_variants").Where("id", row["variant_id"]).One()
	if e != nil {
		return true, finish("failed", "content lookup failed", PublishResult{}, time.Now())
	}
	if v["state"].String() != "approved" || v["version"].Int() != row["variant_version"].Int() || v["plan_version"].Int() != plan["version"].Int() {
		return true, finish("cancelled", "content version or review changed", PublishResult{}, time.Now())
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil || encode(source) != encode(object(v["source_snapshot"])) {
		return true, finish("blocked", "source material changed or unavailable; regenerate and review", PublishResult{}, time.Now())
	}
	snapshot := object(row["snapshot"])
	media, e := Media(ctx, p, plan, imageReferences(snapshot["images"]))
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	mediaURLs := []string{}
	video, e := videoForVariant(ctx, p, plan, platform, snapshot, true, true)
	if e != nil {
		return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
	}
	allMedia := append([]MediaFile{}, media...)
	if video != nil {
		if e = requireVideoAccount(ctx, account); e != nil {
			return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
		}
		allMedia = append(allMedia, *video)
	}
	if platform == "instagram" || platform == "threads" || video != nil {
		if encode(snapshot["media_manifest"]) != encode(imageDigests(allMedia)) {
			return true, finish("blocked", "reviewed media snapshot changed", PublishResult{}, time.Now())
		}
		if platform == "instagram" || platform == "threads" {
			e = validateMetaContent(platform, gconv.String(snapshot["body"]), gconv.String(snapshot["link"]), media, video)
		}
		if e != nil {
			return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
		}
		mediaURLs, e = deliveryMediaURLs(ctx, row["id"].Int64(), allMedia)
		if e != nil {
			return true, finish("blocked", e.Error(), PublishResult{}, time.Now())
		}
	}
	// Hold the account publishing lane across the external request. This serializes
	// spacing checks between processes and fences concurrent requests to this Page.
	result := PublishResult{}
	state, reason := "published", ""
	next := time.Now()
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.GetOne(`SELECT id FROM bm_social_accounts WHERE id=$1 FOR UPDATE`, account["id"]); e != nil {
			return e
		}
		live, e := tx.GetOne(`SELECT p.status,p.version,d.status delivery_status,v.state,v.version variant_version FROM bm_social_deliveries d JOIN bm_social_plans p ON p.id=d.plan_id JOIN bm_social_variants v ON v.id=d.variant_id WHERE d.id=$1 AND d.lease_token=$2`, row["id"], token)
		if e != nil {
			return e
		}
		if len(live) == 0 || live["status"].String() != "scheduled" || live["state"].String() != "approved" || live["version"].Int() != plan["version"].Int() || live["variant_version"].Int() != row["variant_version"].Int() {
			state, reason = "cancelled", "task changed before publishing"
			return nil
		}
		currentActor, err := workerUser(ctx, row["publisher_id"].Int64(), row["dept_id"].Int64(), "social:publish")
		if err != nil {
			state, reason = "blocked", "publisher permission changed"
			return nil
		}
		currentAccount, err := accountAccess(ctx, currentActor, row["account_id"].Int64(), "social:publish", true)
		if err != nil {
			state, reason = "blocked", "Page access changed"
			return nil
		}
		if publishingMode(currentAccount) != "automatic" || (currentAccount["platform"].String() == "facebook" && currentAccount["target_type"].String() == "person") {
			state, reason = "blocked", "delivery publishing mode does not match account"
			return nil
		}
		access, err = tokenFor(currentAccount)
		if err != nil {
			state, reason = "blocked", err.Error()
			return nil
		}
		if platform == "instagram" || platform == "threads" {
			if err = validatePublishingAccount(ctx, currentAccount); err != nil {
				state, reason = "blocked", err.Error()
				return nil
			}
		}
		recent, e := tx.GetValue(`SELECT max(submitted_at) FROM bm_social_deliveries WHERE account_id=$1 AND submitted_at IS NOT NULL`, account["id"])
		if e != nil {
			return e
		}
		if !recent.IsNil() {
			allowed := recent.Time().Add(time.Duration(c.MinIntervalMinutes) * time.Minute)
			if allowed.After(time.Now()) {
				state, reason, next = "retry", "waiting for account publishing interval", allowed
				return nil
			}
		}
		n, e := tx.Model("bm_social_deliveries").Where("account_id", account["id"]).Where("submitted_at>now()-interval '24 hours'").Count()
		if e != nil {
			return e
		}
		if n >= c.DailyPosts {
			state, reason, next = "retry", "daily Page publishing limit", time.Now().Add(time.Hour)
			return nil
		}
		if e = growth.MasterSocialDeliveryGuard(ctx, currentActor, plan["master_content_id"].Int64()); e != nil {
			state, reason = "blocked", e.Error()
			return nil
		}
		input := PublishInput{Body: gconv.String(snapshot["body"]), Title: gconv.String(snapshot["title"]), Link: gconv.String(snapshot["link"]), Images: media, Video: video, MediaType: gconv.String(snapshot["media_type"])}
		if platform == "instagram" || platform == "threads" {
			result, e = publishMetaDelivery(ctx, row["id"].Int64(), platform, access, account["external_id"].String(), input, mediaURLs)
		} else if platform == "facebook" && video != nil {
			result, e = publishFacebookVideo(ctx, row["id"].Int64(), account, access, input)
		} else {
			result, e = publish(ctx, platform, access, account["external_id"].String(), input)
		}
		if e == nil {
			state = result.State
			return nil
		}
		var remote *RemoteError
		state, reason = "failed", e.Error()
		if errors.As(e, &remote) {
			if remote.Unknown {
				state = "unknown"
			} else if remote.Kind == "authorization_revalidation_failed" {
				state = "blocked"
			} else if remote.Kind == "authorization_expired" || remote.Kind == "permission_denied" {
				state = "blocked"
				_, _ = tx.Model("bm_social_accounts").Where("id", account["id"]).Data(g.Map{"status": "reconnect", "error": reason}).Update()
			} else if remote.Kind == "rate_limit" || remote.Kind == "media_processing" {
				state = "retry"
				next = time.Now().Add(remote.RetryAfter)
			}
		}
		return nil
	})
	if e != nil {
		return true, finish("unknown", "publishing result could not be committed; verify Page before retry", result, time.Now())
	}
	return true, finish(state, reason, result, next)
}
func DeliveryControl(ctx context.Context, p *governance.Principal, id int64, operation string, data g.Map) error {
	if operation == "manual_confirm" {
		return confirmManualDelivery(ctx, p, id, data)
	}
	row, e := g.DB().Model("bm_social_deliveries").Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if e != nil {
		return e
	}
	if len(row) == 0 {
		return errors.New("delivery unavailable")
	}
	if _, e = scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:publish"); e != nil {
		return e
	}
	account, e := accountAccess(ctx, p, row["account_id"].Int64(), "social:publish", true)
	if e != nil {
		return e
	}
	if publishingMode(row) == "manual" || publishingMode(account) == "manual" {
		if operation != "cancel" {
			return errors.New("manual deliveries require explicit manual confirmation; platform retry and verification are unavailable")
		}
	}
	switch operation {
	case "cancel":
		if row["status"].String() == "publishing" || row["remote_id"].String() != "" {
			return errors.New("cannot cancel an in-flight or published delivery")
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked", "failed", "unknown"}).Where("remote_id", "").Data("status", "cancelled").Update()
	case "retry", "reconcile":
		if row["status"].String() == "publishing" || row["status"].String() == "published" || row["remote_id"].String() != "" {
			return errors.New("already submitted; verify status instead")
		}
		if operation == "reconcile" {
			progress, e := g.DB().Model("bm_social_provider_operations").Ctx(ctx).Where("delivery_id", id).One()
			if e != nil {
				return e
			}
			state := object(progress["progress"])
			if account["platform"].String() != "facebook" || (gconv.String(state["pending"]) != "upload_video" && gconv.String(state["pending"]) != "upload_reel" && gconv.String(state["pending"]) != "finish_reel") || (gconv.String(state["upload_session"]) == "" && gconv.String(state["video_id"]) == "") {
				return errors.New("no safely reconcilable upload identity; verify the actual provider post instead")
			}
			if e = requireVideoAccount(ctx, account); e != nil {
				return e
			}
		}
		if operation != "reconcile" && row["status"].String() == "unknown" && !gconv.Bool(data["confirmed_absent"]) {
			return errors.New("confirm the post is absent on the Page before retrying")
		}
		if _, e = requirePlatform(ctx, account["platform"].String(), true); e != nil {
			return e
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", id).WhereIn("status", []string{"unknown", "failed", "blocked", "retry"}).Data(g.Map{"status": "scheduled", "next_at": time.Now(), "error": ""}).Update()
	case "verify":
		if !linkedInPostReadable(account) {
			return errors.New("LinkedIn personal post readback is not authorized; submission remains unverified")
		}
		remoteID := row["remote_id"].String()
		if remoteID == "" {
			remoteID = str(gconv.String(data["remote_id"]), 300)
		}
		if remoteID == "" {
			return errors.New("enter the actual platform post ID")
		}
		token, e2 := tokenFor(account)
		if e2 != nil {
			return e2
		}
		var remote PublishResult
		if account["platform"].String() == "facebook" && gconv.String(object(row["snapshot"])["video"]) != "" {
			remote, e2 = readFacebookVideo(ctx, token, remoteID, account["external_id"].String())
		} else {
			remote, e2 = readPost(ctx, account["platform"].String(), token, remoteID, account["external_id"].String())
		}
		if e2 != nil {
			return e2
		}
		values := g.Map{"status": remote.State, "remote_id": remote.ID, "remote_url": remote.URL, "error": ""}
		if remote.State == "published" {
			values["published_at"] = time.Now()
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", id).WhereNot("status", "publishing").Data(values).Update()
	default:
		return errors.New("invalid delivery operation")
	}
	if e == nil {
		e = securityAudit(ctx, p, operation, id)
	}
	return e
}
func Tick(ctx context.Context) error {
	if e := recoverWork(ctx); e != nil {
		return e
	}
	if e := PurgeExpired(ctx); e != nil {
		return e
	}
	if e := PurgeAuthorizationTasks(ctx); e != nil {
		return e
	}
	cfg, e := config(ctx)
	if e != nil {
		return e
	}
	rows, e := g.DB().Model("bm_social_accounts").Ctx(ctx).Where("status", "connected").Where("publish_mode", "automatic").Where("next_sync_at<=now()").Limit(20).All()
	if e != nil {
		return e
	}
	for _, row := range rows {
		if !accountSyncSupported(row) {
			continue
		}
		if !cfg.platform(row["platform"].String()).Enabled {
			continue
		}
		p, e := workerUser(ctx, row["owner_id"].Int64(), row["dept_id"].Int64(), "social:interaction:view")
		if e != nil {
			continue
		}
		if _, e = queue(ctx, p, 0, row["id"].Int64(), "sync", "", g.Map{}); e != nil {
			return e
		}
		_, _ = g.DB().Model("bm_social_accounts").Where("id", row["id"]).Data("next_sync_at", time.Now().Add(time.Duration(cfg.SyncMinutes)*time.Minute)).Update()
	}
	return prepareRecurrences(ctx)
}
func prepareRecurrences(ctx context.Context) error {
	rows, e := g.DB().Model("bm_social_plans").Where("status", "scheduled").Where("next_at<=now()").Limit(20).All()
	if e != nil {
		return e
	}
	for _, row := range rows {
		pending, e := g.DB().Model("bm_social_deliveries d").InnerJoin("bm_social_accounts a", "a.id=d.account_id").Where("d.plan_id", row["id"]).WhereNull("d.deleted_at").WhereIn("d.status", []string{"scheduled", "manual_due", "retry", "publishing", "submitted", "unknown", "blocked"}).Fields("d.status,d.remote_id,a.platform,a.target_type,a.publish_mode,a.scopes,a.capabilities").All()
		if e != nil {
			return e
		}
		n := 0
		for _, d := range pending {
			if d["status"].String() == "submitted" && d["remote_id"].String() != "" && !linkedInPostReadable(d) {
				continue
			}
			n++
		}
		if n > 0 {
			continue
		}
		c, e := planConfig(row)
		if e != nil {
			return e
		}
		if c.Cadence == "once" {
			_, _ = g.DB().Model("bm_social_plans").Where("id", row["id"]).Data("status", "completed").Update()
			continue
		}
		p, e := workerUser(ctx, row["owner_id"].Int64(), row["dept_id"].Int64(), "social:content:edit")
		if e != nil {
			continue
		}
		days := 1
		if c.Cadence == "weekly" {
			days = 7
		}
		location, _ := time.LoadLocation(c.Markets[0].Timezone)
		date, e := time.ParseInLocation("2006-01-02", c.StartDate, location)
		if e != nil {
			date = time.Now().In(location)
		}
		// An early manual run starts this recurrence from its real execution
		// date. Keep cycle_at stable so repeated clicks cannot create new posts.
		immediate, e := g.DB().Model("bm_social_deliveries").Ctx(ctx).Where("plan_id", row["id"]).Where("due_at", row["next_at"].Time()).Where("snapshot->>'run_immediately'=?", "true").Count()
		if e != nil {
			return e
		}
		if immediate > 0 {
			date = row["next_at"].Time().In(location)
		}
		date = date.AddDate(0, 0, days)
		for date.Before(time.Now().In(location).Truncate(24 * time.Hour)) {
			date = date.AddDate(0, 0, days)
		}
		c.StartDate = date.Format("2006-01-02")
		_, e = SavePlan(ctx, p, row["id"].Int64(), row["name"].String(), row["company_id"].Int64(), c)
		if e != nil {
			return e
		}
		// A new AI cycle always returns to human review. Reusing an approved copy is
		// explicit; it is resubmitted for review when source/config versions changed.
		if e = PlanControl(ctx, p, row["id"].Int64(), "generate"); e != nil {
			return e
		}
	}
	return nil
}

var startOnce sync.Once

func Start(ctx context.Context) {
	startOnce.Do(func() {
		launch := func(label string, job func(context.Context) error) {
			go func() {
				tick := time.NewTicker(3 * time.Second)
				defer tick.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-tick.C:
						func() {
							defer func() {
								if recover() != nil {
									g.Log().Error(ctx, "social worker recovered")
								}
							}()
							work, cancel := context.WithTimeout(ctx, 4*time.Minute)
							defer cancel()
							if e := job(work); e != nil {
								g.Log().Error(ctx, "social "+label+" unavailable")
							}
						}()
					}
				}
			}()
		}
		launch("scheduler", Tick)
		launch("publisher", func(ctx context.Context) error { _, e := ProcessDelivery(ctx); return e })
		launch("jobs", func(ctx context.Context) error { _, e := ProcessJob(ctx); return e })
	})
}
