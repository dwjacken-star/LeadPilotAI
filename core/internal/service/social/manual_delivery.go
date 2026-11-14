package social

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func publishingMode(row gdb.Record) string {
	if row["publish_mode"].String() == "" {
		return "automatic"
	}
	return row["publish_mode"].String()
}

func manualAccount(row gdb.Record) bool {
	return row["platform"].String() == "facebook" && row["target_type"].String() == "person" && publishingMode(row) == "manual"
}

func validateManualAccount(row gdb.Record) error {
	if !manualAccount(row) || row["status"].String() != "connected" || !gconv.Bool(object(row["capabilities"])["manual_publish"]) || !strings.HasPrefix(row["external_id"].String(), "manual:") {
		return errors.New("a connected Facebook personal manual account is required")
	}
	return nil
}

func validatePublishingAccount(ctx context.Context, row gdb.Record) error {
	if row["platform"].String() == "instagram" || row["platform"].String() == "threads" {
		c, err := requirePlatform(ctx, row["platform"].String(), true)
		if err != nil {
			return err
		}
		if row["authorization_fingerprint"].String() != authorizationFingerprint(c, row["target_type"].String()) {
			return errors.New("application configuration changed; reconnect this account")
		}
		scopes := []string{}
		_ = json.Unmarshal(row["scopes"].Bytes(), &scopes)
		if !gconv.Bool(accountCapabilities(row["platform"].String(), row["target_type"].String(), scopes)["publish"]) {
			return errors.New("account publishing scopes are missing")
		}
	}
	if publishingMode(row) == "manual" {
		return validateManualAccount(row)
	}
	if publishingMode(row) != "automatic" || (row["platform"].String() == "facebook" && row["target_type"].String() == "person") {
		return errors.New("Facebook personal accounts require manual publishing")
	}
	if _, e := requirePlatform(ctx, row["platform"].String(), true); e != nil {
		return e
	}
	if _, e := tokenFor(row); e != nil {
		return e
	}
	if !gconv.Bool(object(row["capabilities"])["publish"]) {
		return errors.New("account has no publishing scope")
	}
	return nil
}

func linkedInPostReadable(account gdb.Record) bool {
	if account["platform"].String() != "linkedin" || account["target_type"].String() != "person" {
		return true
	}
	return has(gconv.Strings(account["scopes"].Interface()), "r_member_social") || gconv.Bool(object(account["capabilities"])["read_posts"])
}

func accountSyncSupported(account gdb.Record) bool {
	if publishingMode(account) != "automatic" || (account["platform"].String() == "facebook" && account["target_type"].String() == "person") {
		return false
	}
	caps := object(account["capabilities"])
	return linkedInPostReadable(account) || gconv.Bool(caps["comments"]) || gconv.Bool(caps["reactions"]) || gconv.Bool(caps["metrics"])
}

func manualExternalURL(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	u, e := url.Parse(value)
	if e != nil || u.Scheme != "https" || u.User != nil || len(value) > 2000 || (u.Port() != "" && u.Port() != "443") {
		return "", errors.New("use an HTTPS Facebook post URL without credentials")
	}
	host := strings.ToLower(u.Hostname())
	if host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return "", errors.New("use an HTTPS Facebook post URL")
	}
	return u.String(), nil
}

// Call with the parent plan, delivery and account locked in that order.
func manualDeliveryGuard(ctx context.Context, p *governance.Principal, plan, delivery, account gdb.Record) error {
	if publishingMode(delivery) != "manual" {
		return errors.New("delivery is not manual")
	}
	if delivery["remote_id"].String() != "" || !delivery["submitted_at"].IsNil() || !delivery["published_at"].IsNil() {
		return errors.New("manual delivery must not contain automatic publishing evidence")
	}
	if e := validateManualAccount(account); e != nil {
		return e
	}
	if plan["status"].String() != "scheduled" || !delivery["deleted_at"].IsNil() || delivery["due_at"].Time().After(time.Now()) {
		return errors.New("manual delivery is not active and due")
	}
	v, e := g.DB().Model("bm_social_variants").Ctx(ctx).Where("id", delivery["variant_id"]).One()
	if e != nil {
		return e
	}
	if len(v) == 0 || v["plan_id"].Int64() != plan["id"].Int64() || v["state"].String() != "approved" || v["version"].Int() != delivery["variant_version"].Int() || v["plan_version"].Int() != plan["version"].Int() || v["platform"].String() != account["platform"].String() {
		return errors.New("content version or review changed")
	}
	c, e := planConfig(plan)
	if e != nil {
		return e
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil {
		return e
	}
	snapshot := object(delivery["snapshot"])
	if encode(source) != encode(object(v["source_snapshot"])) || encode(source) != encode(object(snapshot["sources"])) {
		return errors.New("source material changed or unavailable; regenerate and review")
	}
	for _, field := range []string{"body", "title", "link"} {
		if gconv.String(snapshot[field]) != v[field].String() {
			return errors.New("delivery snapshot no longer matches reviewed content")
		}
	}
	if encode(imageReferences(snapshot["images"])) != encode(imageReferences(v["images"])) {
		return errors.New("delivery images no longer match reviewed content")
	}
	if _, e = Media(ctx, p, plan, imageReferences(snapshot["images"])); e != nil {
		return e
	}
	return growth.MasterSocialDeliveryGuard(ctx, p, plan["master_content_id"].Int64())
}

func confirmManualDelivery(ctx context.Context, p *governance.Principal, id int64, data g.Map) error {
	if e := require(p, "social:publish"); e != nil {
		return e
	}
	if !gconv.Bool(data["manual_confirmed"]) {
		return errors.New("explicitly confirm that you manually published this post on Facebook")
	}
	externalURL, e := manualExternalURL(gconv.String(data["external_url"]))
	if e != nil {
		return e
	}
	row, e := g.DB().Model("bm_social_deliveries").Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if e != nil {
		return e
	}
	if len(row) == 0 {
		return errors.New("delivery unavailable")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		plan, e := tx.GetOne(`SELECT * FROM bm_social_plans WHERE id=$1 FOR UPDATE`, row["plan_id"])
		if e != nil {
			return e
		}
		delivery, e := tx.GetOne(`SELECT * FROM bm_social_deliveries WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		actor, e := workerUser(ctx, p.ID, p.DeptID, "social:publish")
		if e != nil {
			return e
		}
		if _, e = scoped(ctx, actor, "bm_social_plans", plan["id"].Int64(), "social:publish"); e != nil {
			return e
		}
		if _, e = tx.GetOne(`SELECT id FROM bm_social_accounts WHERE id=$1 FOR UPDATE`, delivery["account_id"]); e != nil {
			return e
		}
		account, e := accountAccess(ctx, actor, delivery["account_id"].Int64(), "social:publish", true)
		if e != nil {
			return e
		}
		if publishingMode(delivery) != "manual" || !delivery["deleted_at"].IsNil() || validateManualAccount(account) != nil {
			return errors.New("manual delivery unavailable")
		}
		// An exact repeat is harmless, but never changes the original confirmation.
		if delivery["status"].String() == "manual_confirmed" {
			if externalURL != "" && externalURL != delivery["remote_url"].String() {
				return errors.New("manual confirmation has already been recorded")
			}
			return nil
		}
		if delivery["status"].String() != "manual_due" {
			return errors.New("only a due manual reminder can be confirmed")
		}
		if e = manualDeliveryGuard(ctx, actor, plan, delivery, account); e != nil {
			return e
		}
		_, e = tx.Model("bm_social_deliveries").Where("id", id).Data(g.Map{"status": "manual_confirmed", "manual_confirmed_at": time.Now(), "manual_confirmed_by": actor.ID, "remote_url": externalURL, "error": ""}).Update()
		if e != nil {
			return e
		}
		return securityAudit(ctx, actor, "manual_confirm", id)
	})
}

func ManualReminders(ctx context.Context, p *governance.Principal) (g.Map, error) {
	planPredicate, e := p.Predicate(ctx, "social:publish", "p")
	if e != nil {
		return nil, e
	}
	accountScope, e := accountPredicate(ctx, p, "social:publish", "a", true)
	if e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_social_deliveries d").Ctx(ctx).InnerJoin("bm_social_plans p", "p.id=d.plan_id").InnerJoin("bm_social_accounts a", "a.id=d.account_id").InnerJoin("bm_social_variants v", "v.id=d.variant_id").Where(planPredicate).Where(accountScope).WhereNull("d.deleted_at").Where("p.status", "scheduled").Where("d.status", "manual_due").Where("d.publish_mode", "manual").Where("d.due_at<=now()").Where("a.platform='facebook' AND a.target_type='person' AND a.publish_mode='manual' AND a.status='connected' AND a.capabilities->>'manual_publish'='true'").Where("v.state='approved' AND v.version=d.variant_version AND v.plan_version=p.version")
	total, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	rows, e := q.Fields("d.id,d.plan_id,d.variant_id,d.account_id,d.status,d.due_at,d.publish_mode,d.manual_confirmed_at,d.manual_confirmed_by,p.name AS plan_name,a.name AS account_name,a.platform,a.target_type,v.title,v.country,v.language").Order("d.due_at,d.id").Limit(500).All()
	if rows == nil {
		rows = gdb.Result{}
	}
	return g.Map{"total": total, "rows": rows}, e
}
