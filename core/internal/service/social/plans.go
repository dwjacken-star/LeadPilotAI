package social

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type Market struct {
	Country   string `json:"country"`
	Language  string `json:"language"`
	Timezone  string `json:"timezone"`
	LocalTime string `json:"local_time"`
}
type PlanConfig struct {
	Platforms          []string `json:"platforms"`
	Accounts           []int64  `json:"accounts"`
	Products           []int64  `json:"products"`
	Assets             []string `json:"assets"`
	TaskAssets         []string `json:"task_assets"`
	RowIDs             []int64  `json:"row_ids"`
	Markets            []Market `json:"markets"`
	Requirement        string   `json:"requirement"`
	Model              string   `json:"model"`
	Link               string   `json:"link"`
	StartDate          string   `json:"start_date"`
	Cadence            string   `json:"cadence"`
	ReuseApproved      bool     `json:"reuse_approved"`
	DailyPosts         int      `json:"daily_posts"`
	MinIntervalMinutes int      `json:"min_interval_minutes"`
	SyncInteractions   bool     `json:"sync_interactions"`
	AnalyzeComments    bool     `json:"analyze_comments"`
}

var languageCode = regexp.MustCompile(`^[a-z]{2,3}([-_][A-Za-z0-9]{2,8}){0,2}$`)
var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func validateConfig(c PlanConfig) error {
	if c.ReuseApproved {
		return errors.New("recurring content requires review each cycle")
	}
	if len(c.Platforms) < 1 || len(c.Platforms) > 4 || len(c.Markets) < 1 || len(c.Markets) > 30 {
		return errors.New("select platforms and 1-30 markets")
	}
	seen := map[string]bool{}
	for _, s := range c.Platforms {
		if !platformOK(s) || seen[s] {
			return errors.New("invalid or duplicate platform")
		}
		seen[s] = true
	}
	seen = map[string]bool{}
	for _, m := range c.Markets {
		if !countryCode.MatchString(m.Country) || !languageCode.MatchString(m.Language) {
			return errors.New("select country and language")
		}
		if _, e := time.LoadLocation(m.Timezone); e != nil {
			return errors.New("invalid market timezone")
		}
		if _, e := parsePublishingTime(m.LocalTime); e != nil {
			return errors.New("use HH:mm:ss local publishing time")
		}
		key := m.Country + ":" + m.Language
		if seen[key] {
			return errors.New("duplicate country and language")
		}
		seen[key] = true
	}
	if c.Cadence != "once" && c.Cadence != "daily" && c.Cadence != "weekly" {
		return errors.New("invalid recurrence")
	}
	if c.StartDate != "" {
		if _, e := time.Parse("2006-01-02", c.StartDate); e != nil {
			return errors.New("invalid start date")
		}
	}
	if c.DailyPosts < 1 || c.DailyPosts > 20 || c.MinIntervalMinutes < 5 || c.MinIntervalMinutes > 1440 {
		return errors.New("set daily posts 1-20 and interval 5-1440 minutes")
	}
	if utf8.RuneCountInString(c.Requirement) > 8000 || len(c.Products) > 100 || len(c.Assets) > 30 || len(c.TaskAssets) > 50 || len(c.Accounts) > 30 || len(c.RowIDs) > 100 {
		return errors.New("reduce task inputs")
	}
	if c.Link != "" {
		u, e := url.Parse(c.Link)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return errors.New("use an HTTPS landing page URL")
		}
	}
	return nil
}
func SavePlan(ctx context.Context, p *governance.Principal, id int64, name string, companyID int64, c PlanConfig, requestKeys ...string) (int64, error) {
	if id == 0 && len(requestKeys) > 0 && requestKeys[0] != "" {
		return savePlanOnce(ctx, p, requestKeys[0], name, companyID, c)
	}
	if e := require(p, "social:plan:manage"); e != nil {
		return 0, e
	}
	name = str(name, 200)
	if name == "" {
		return 0, errors.New("plan name required")
	}
	if e := validateConfig(c); e != nil {
		return 0, e
	}
	companies, e := growth.Companies(ctx, p, "social:plan:manage")
	if e != nil {
		return 0, e
	}
	found := false
	for _, r := range companies {
		if r["id"].Int64() == companyID {
			found = true
		}
	}
	if !found {
		return 0, errors.New("company unavailable")
	}
	for _, a := range c.Accounts {
		row, e := accountAccess(ctx, p, a, "social:plan:manage", true)
		if e != nil {
			return 0, e
		}
		if !has(c.Platforms, row["platform"].String()) {
			return 0, errors.New("account platform is not selected")
		}
	}
	if id == 0 {
		if e = p.CheckOwnership(ctx, "social:plan:manage", p.DeptID, p.ID); e != nil {
			return 0, e
		}
		id, e = g.DB().Model("bm_social_plans").Ctx(ctx).Data(g.Map{"name": name, "company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "config": encode(c)}).InsertAndGetId()
		return id, e
	}
	old, e := scoped(ctx, p, "bm_social_plans", id, "social:plan:manage")
	if e != nil {
		return 0, e
	}
	if old["company_id"].Int64() != companyID {
		count, e := g.DB().Model("bm_social_assets").Where("plan_id", id).Count()
		if e != nil {
			return 0, e
		}
		if count > 0 {
			return 0, errors.New("a task with uploaded materials cannot switch companies")
		}
	}
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		current, e := tx.GetOne(`SELECT * FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if encode(object(current["config"])) == encode(object(encode(c))) && current["name"].String() == name && current["company_id"].Int64() == companyID {
			return nil
		}
		sending, e := tx.Model("bm_social_deliveries").Where("plan_id", id).Where("status", "publishing").Count()
		if e != nil {
			return e
		}
		if sending > 0 {
			return errors.New("wait for the in-flight publishing request before editing")
		}
		if _, e = tx.Model("bm_social_plans").Where("id", id).Data(g.Map{"name": name, "company_id": companyID, "config": encode(c), "status": "draft", "version": contentVersion(current, c, companyID), "error": "", "next_at": nil, "updated_at": time.Now()}).Update(); e != nil {
			return e
		}
		_, e = tx.Model("bm_social_deliveries").Where("plan_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).Data(g.Map{"status": "cancelled", "error": "plan changed; review new content before publishing"}).Update()
		return e
	})
	return id, e
}

// Scheduling and quota edits retain the reviewed content version.
func contentVersion(old gdb.Record, c PlanConfig, company int64) int {
	previous, _ := planConfig(old)
	fingerprint := func(v PlanConfig) string {
		markets := []string{}
		for _, m := range v.Markets {
			markets = append(markets, m.Country+":"+m.Language)
		}
		return encode(g.Map{"platforms": v.Platforms, "products": v.Products, "assets": v.Assets, "task_assets": v.TaskAssets, "rows": v.RowIDs, "markets": markets, "requirement": v.Requirement, "model": v.Model, "link": v.Link})
	}
	n := old["version"].Int()
	if old["company_id"].Int64() != company || fingerprint(previous) != fingerprint(c) {
		n++
	}
	return n
}

var generationMore = errors.New("more variants remain")

func sourceContext(ctx context.Context, p *governance.Principal, plan gdb.Record, c PlanConfig) (g.Map, error) {
	source, e := growth.SocialSources(ctx, p, plan["company_id"].Int64(), c.Products, c.Assets)
	if e != nil {
		return nil, e
	}
	materials := []g.Map{}
	for _, id := range c.TaskAssets {
		asset, e := Asset(ctx, p, id, sourcePermission(p))
		if e != nil {
			return nil, e
		}
		if asset["plan_id"].Int64() != plan["id"].Int64() || !asset["approved"].Bool() || asset["status"].String() != "ready" || !asset["ai_allowed"].Bool() {
			return nil, errors.New("selected task material is not approved and AI-enabled")
		}
		if asset["category"].String() == "internal" {
			return nil, errors.New("internal material cannot be used for public marketing")
		}
		data := g.Map{"id": id, "filename": asset["filename"].String(), "version": asset["version"].Int(), "facts": growth.SocialPublicFacts(object(asset["extracted"]))}
		if asset["category"].String() == "product_sheet" {
			q := g.DB().Model("bm_social_asset_rows").Ctx(ctx).Where("asset_id", id)
			count, e := q.Clone().Count()
			if e != nil {
				return nil, e
			}
			if len(c.RowIDs) > 0 {
				q = q.WhereIn("id", c.RowIDs)
			} else if count > 30 {
				return nil, errors.New("select specific product rows from the workbook preview")
			}
			rows, e := q.Fields("sheet,row_number,sku,name,fields,state").Order("id").Limit(100).All()
			if e != nil {
				return nil, e
			}
			for _, r := range rows {
				r["fields"].Set(growth.SocialPublicFields(object(r["fields"])))
				if r["state"].String() != "valid" {
					return nil, errors.New("selected workbook rows need SKU and product name")
				}
			}
			data["rows"] = rows
		}
		materials = append(materials, data)
	}
	source["task_materials"] = materials
	source["row_ids"] = c.RowIDs
	if len(encode(source)) > 180000 {
		return nil, errors.New("narrow the selected material or product rows")
	}
	return source, nil
}
func generatePlan(ctx context.Context, p *governance.Principal, id int64, expected int) error {
	plan, e := scoped(ctx, p, "bm_social_plans", id, "social:content:edit")
	if e != nil {
		return e
	}
	if plan["version"].Int() != expected {
		return errors.New("plan changed; generate again")
	}
	c, e := planConfig(plan)
	if e != nil {
		return e
	}
	if strings.TrimSpace(c.Requirement) == "" {
		return errors.New("writing requirements required")
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil {
		return e
	}
	generated := false
	for _, platform := range c.Platforms {
		for _, market := range c.Markets {
			existing, e := g.DB().Model("bm_social_variants").Where(g.Map{"plan_id": id, "plan_version": expected, "platform": platform, "country": market.Country, "language": market.Language}).Count()
			if e != nil {
				return e
			}
			if existing > 0 {
				continue
			}
			if generated {
				return generationMore
			}
			var result struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Language string `json:"language"`
			}
			e = growth.SocialAI(ctx, p, "social_compose", growth.AIInput{Model: c.Model, MaxOutput: 3000, Instruction: "Write a professional B2B social media post in exactly the selected language for the selected country and platform. Use ONLY supplied company-owned approved facts and writing requirements. Never invent prices, certifications, warranties, product compatibility, testimonials, or contact details. LinkedIn: professional insight and clear business value; Facebook: readable product introduction and engagement. Return plain text body with paragraph breaks, optional modest hashtags, and a concise title. Do not return email salutations, HTML, Markdown formatting, or URLs; the application attaches the landing link. Do not reference external member comments or social profiles.", Data: g.Map{"source": source, "requirements": c.Requirement, "platform": platform, "country": market.Country, "language": market.Language}, Schema: jsonSchema(g.Map{"title": stringSchema(), "body": stringSchema(), "language": stringSchema()})}, &result)
			if e != nil {
				return e
			}
			if result.Language != market.Language || strings.TrimSpace(result.Body) == "" {
				return errors.New("AI returned empty content or the wrong language")
			}
			limit := 60000
			if platform == "linkedin" {
				limit = 2800
			}
			if platform == "instagram" {
				limit = 2200
			}
			if platform == "threads" {
				limit = 500
			}
			if utf8.RuneCountInString(result.Body) > limit {
				return errors.New("AI post is too long")
			}
			e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				live, e := tx.GetOne(`SELECT version FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
				if e != nil {
					return e
				}
				if live["version"].Int() != expected {
					return errors.New("plan changed during generation")
				}
				_, e = tx.Model("bm_social_variants").Data(g.Map{"plan_id": id, "plan_version": expected, "platform": platform, "country": market.Country, "language": market.Language, "timezone": market.Timezone, "title": str(result.Title, 200), "body": strings.TrimSpace(result.Body), "link": c.Link, "source_snapshot": encode(source)}).Insert()
				return e
			})
			if e != nil {
				return e
			}
			generated = true
		}
	}
	_, e = g.DB().Model("bm_social_plans").Where("id", id).Where("version", expected).Data(g.Map{"status": "review", "error": "", "updated_at": time.Now()}).Update()
	return e
}
func variant(ctx context.Context, p *governance.Principal, id int64, permission string) (gdb.Record, gdb.Record, error) {
	r, e := g.DB().Model("bm_social_variants").Ctx(ctx).Where("id", id).One()
	if e != nil {
		return nil, nil, e
	}
	if len(r) == 0 {
		return nil, nil, errors.New("variant unavailable")
	}
	plan, e := scoped(ctx, p, "bm_social_plans", r["plan_id"].Int64(), permission)
	if e != nil {
		return nil, nil, e
	}
	if r["plan_version"].Int() != plan["version"].Int() {
		return nil, nil, errors.New("this is a historical content version")
	}
	return r, plan, nil
}
func SaveVariant(ctx context.Context, p *governance.Principal, id int64, data g.Map) error {
	row, plan, e := variant(ctx, p, id, "social:content:edit")
	if e != nil {
		return e
	}
	body := strings.TrimSpace(gconv.String(data["body"]))
	if body == "" {
		return errors.New("post body required")
	}
	limit := 60000
	if row["platform"].String() == "linkedin" {
		limit = 2800
	}
	if row["platform"].String() == "instagram" {
		limit = 2200
	}
	if row["platform"].String() == "threads" {
		limit = 500
	}
	if utf8.RuneCountInString(body) > limit {
		return errors.New("post body too long")
	}
	link := gconv.String(data["link"])
	if link != "" {
		u, e := url.Parse(link)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return errors.New("HTTPS landing link required")
		}
	}
	images := imageReferences(data["images"])
	if len(images) > 10 {
		return errors.New("at most 10 images per post")
	}
	draftMedia, e := readMedia(ctx, p, plan, images, false)
	if e != nil {
		return e
	}
	video, e := videoForVariant(ctx, p, plan, row["platform"].String(), data, false, true)
	if e != nil {
		return e
	}
	kind := gconv.String(data["media_type"])
	if kind == "" {
		kind = "images"
	}
	if platform := row["platform"].String(); (platform == "instagram" && (len(images) > 0 || video != nil)) || platform == "threads" {
		if e = validateMetaContent(platform, body, link, draftMedia, video); e != nil {
			return e
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, e := tx.GetOne(`SELECT id FROM bm_social_plans WHERE id=$1 FOR UPDATE`, plan["id"])
		if e != nil {
			return e
		}
		n, e := tx.Model("bm_social_deliveries").Where("variant_id", id).Where("status", "publishing").Count()
		if e != nil {
			return e
		}
		if n > 0 {
			return errors.New("publishing in progress")
		}
		if _, e = tx.Model("bm_social_variants").Where("id", id).Data(g.Map{"title": str(gconv.String(data["title"]), 200), "body": body, "link": link, "images": encode(images), "video": gconv.String(data["video"]), "media_type": kind, "public_reels_confirmed": gconv.Bool(data["public_reels_confirmed"]), "state": "review", "version": gdb.Raw("version+1"), "approved_by": nil, "approved_at": nil, "updated_at": time.Now()}).Update(); e != nil {
			return e
		}
		_, e = tx.Model("bm_social_deliveries").Where("variant_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).Data(g.Map{"status": "cancelled", "error": "content changed; review required"}).Update()
		return e
	})
}
func ApproveVariant(ctx context.Context, p *governance.Principal, id int64) error {
	row, plan, e := variant(ctx, p, id, "social:content:review")
	if e != nil {
		return e
	}
	if row["body"].String() == "" {
		return errors.New("post body required")
	}
	video, e := videoForVariant(ctx, p, plan, row["platform"].String(), row.Map(), false, true)
	if e != nil {
		return e
	}
	if platform := row["platform"].String(); platform == "instagram" || platform == "threads" {
		media, err := readMedia(ctx, p, plan, imageReferences(row["images"]), false)
		if err != nil {
			return err
		}
		if err = validateMetaContent(platform, row["body"].String(), row["link"].String(), media, video); err != nil {
			return err
		}
	}
	c, e := planConfig(plan)
	if e != nil {
		return e
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil {
		return e
	}
	if encode(source) != encode(object(row["source_snapshot"])) {
		return errors.New("source material changed; regenerate and review")
	}
	return approveVariantImages(ctx, p, id)
}
func marketDue(m Market, date string, now time.Time) (time.Time, error) {
	due, e := marketTimeAt(m, date, now)
	if e != nil {
		return due, e
	}
	if due.Before(now.Add(5 * time.Second)) {
		return due, errors.New("publishing time has passed; choose a future local date/time")
	}
	return due, nil
}
func marketTimeAt(m Market, date string, now time.Time) (time.Time, error) {
	location, e := time.LoadLocation(m.Timezone)
	if e != nil {
		return time.Time{}, e
	}
	if date == "" {
		date = now.In(location).Format("2006-01-02")
	}
	clock, e := parsePublishingTime(m.LocalTime)
	if e != nil {
		return time.Time{}, e
	}
	local := date + " " + clock.Format("15:04:05")
	due, e := time.ParseInLocation("2006-01-02 15:04:05", local, location)
	if e != nil {
		return due, e
	}
	if due.Format("2006-01-02 15:04:05") != local {
		return due, errors.New("this local time does not exist due to daylight saving; choose another time")
	}
	return due, nil
}
func parsePublishingTime(value string) (time.Time, error) {
	if len(value) == 5 {
		return time.Parse("15:04", value)
	}
	if len(value) != 8 {
		return time.Time{}, errors.New("use HH:mm:ss local publishing time")
	}
	return time.Parse("15:04:05", value)
}
func Schedule(ctx context.Context, p *governance.Principal, id int64) error {
	return scheduleLocked(ctx, p, id, false)
}
func RunNow(ctx context.Context, p *governance.Principal, id int64) error {
	e := scheduleLocked(ctx, p, id, true)
	if e == nil {
		e = securityAudit(ctx, p, "run_now", id)
	}
	return e
}
func scheduleLocked(ctx context.Context, p *governance.Principal, id int64, immediate bool) error {
	if _, e := scoped(ctx, p, "bm_social_plans", id, "social:publish"); e != nil {
		return e
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.GetOne(`SELECT id FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id); e != nil {
			return e
		}
		return schedulePlan(ctx, p, id, immediate)
	})
}
func schedulePlan(ctx context.Context, p *governance.Principal, id int64, immediate bool) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	plan, e := scoped(ctx, p, "bm_social_plans", id, "social:publish")
	if e != nil {
		return e
	}
	if immediate && plan["status"].String() != "draft" && plan["status"].String() != "scheduled" && plan["status"].String() != "paused" {
		return errors.New("only ready, scheduled or paused plans can run now")
	}
	c, e := planConfig(plan)
	if e != nil {
		return e
	}
	if len(c.Accounts) == 0 {
		return errors.New("connect and select a social account before scheduling")
	}
	fillDate := immediate && c.StartDate == ""
	if fillDate {
		location, err := time.LoadLocation(c.Markets[0].Timezone)
		if err != nil {
			return err
		}
		c.StartDate = now.In(location).Format("2006-01-02")
	}
	variants, e := g.DB().Model("bm_social_variants").Ctx(ctx).Where("plan_id", id).Where("plan_version", plan["version"]).All()
	if e != nil {
		return e
	}
	if len(variants) != len(c.Platforms)*len(c.Markets) {
		return errors.New("generate and review every platform/market version first")
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil {
		return e
	}
	type target struct {
		row      gdb.Record
		account  gdb.Record
		due      time.Time
		cycle    time.Time
		manifest []g.Map
	}
	targets := []target{}
	earliest := time.Time{}
	for _, v := range variants {
		if v["state"].String() != "approved" || encode(source) != encode(object(v["source_snapshot"])) {
			return errors.New("content review is missing or source version changed")
		}
		market := Market{}
		for _, m := range c.Markets {
			if m.Country == v["country"].String() && m.Language == v["language"].String() {
				market = m
			}
		}
		cycle, e := marketTimeAt(market, c.StartDate, now)
		if e != nil && (!immediate || cycle.IsZero()) {
			return e
		}
		due := cycle
		if immediate {
			due = now
		} else if due.Before(now.Add(5 * time.Second)) {
			return errors.New("publishing time has passed; choose a future local date/time")
		}
		if cycle.IsZero() {
			return errors.New("invalid publishing cycle")
		}
		matched := false
		for _, aid := range c.Accounts {
			a, e := accountAccess(ctx, p, aid, "social:publish", true)
			if e != nil {
				return e
			}
			if a["platform"].String() != v["platform"].String() {
				continue
			}
			matched = true
			if e = validatePublishingAccount(ctx, a); e != nil {
				return e
			}
			media, err := Media(ctx, p, plan, imageReferences(v["images"]))
			if err != nil {
				return err
			}
			video, err := videoForVariant(ctx, p, plan, v["platform"].String(), v.Map(), true, true)
			if err != nil {
				return err
			}
			if video != nil {
				if err = requireVideoAccount(ctx, a); err != nil {
					return err
				}
			}
			if platform := v["platform"].String(); platform == "instagram" || platform == "threads" {
				if e = validateMetaContent(platform, v["body"].String(), v["link"].String(), media, video); e != nil {
					return e
				}
				if video != nil {
					media = append(media, *video)
				}
			}
			if e != nil {
				return e
			}
			targets = append(targets, target{v, a, due, cycle, imageDigests(media)})
			if earliest.IsZero() || due.Before(earliest) {
				earliest = due
			}
		}
		if !matched {
			return errors.New("select an authorized Page for every platform")
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		live, e := tx.GetOne(`SELECT version FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if live["version"].Int() != plan["version"].Int() {
			return errors.New("plan changed")
		}

		for _, v := range variants {
			current, err := tx.Model("bm_social_variants").Where("id", v["id"]).One()
			if err != nil {
				return err
			}
			if current["version"].Int() != v["version"].Int() || current["state"].String() != "approved" {
				return errors.New("content changed while scheduling")
			}
		}
		queued, active := 0, 0
		for _, t := range targets {
			snapshot := g.Map{"title": t.row["title"].String(), "body": t.row["body"].String(), "link": t.row["link"].String(), "images": imageReferences(t.row["images"]), "country": t.row["country"].String(), "language": t.row["language"].String(), "sources": source, "policy": c, "target_type": t.account["target_type"].String(), "publish_mode": publishingMode(t.account)}
			snapshot["video"] = t.row["video"].String()
			snapshot["media_type"] = t.row["media_type"].String()
			snapshot["public_reels_confirmed"] = t.row["public_reels_confirmed"].Bool()
			if platform := t.row["platform"].String(); platform == "instagram" || platform == "threads" || t.row["video"].String() != "" {
				snapshot["media_manifest"] = t.manifest
			}
			if immediate {
				prior, err := tx.GetOne(`SELECT * FROM bm_social_deliveries WHERE variant_id=$1 AND variant_version=$2 AND account_id=$3 AND cycle_at=$4 FOR UPDATE`, t.row["id"], t.row["version"], t.account["id"], t.cycle)
				if err != nil {
					return err
				}
				if len(prior) > 0 {
					if !prior["deleted_at"].IsNil() {
						continue
					}
					switch prior["status"].String() {
					case "publishing", "submitted":
						active++
						continue
					case "scheduled", "manual_due", "retry", "blocked", "cancelled":
						if prior["remote_id"].String() != "" {
							continue
						}
						// A second click never pushes an already due row back or resets
						// its platform retry delay. A paused plan explicitly resumes.
						if plan["status"].String() == "scheduled" && !prior["due_at"].Time().After(now) && prior["status"].String() != "blocked" && prior["status"].String() != "cancelled" {
							active++
							continue
						}
						frozen := object(prior["snapshot"])
						frozen["run_immediately"] = true
						next := now
						if prior["status"].String() == "retry" && prior["next_at"].Time().After(now) {
							next = prior["next_at"].Time()
						}
						_, err = tx.Model("bm_social_deliveries").Where("id", prior["id"]).Data(g.Map{"publisher_id": p.ID, "due_at": now, "next_at": next, "status": "scheduled", "error": "", "snapshot": encode(frozen)}).Update()
						if err != nil {
							return err
						}
						queued++
						continue
					default:
						continue // Never replay published, failed or uncertain results.
					}
				}
				snapshot["run_immediately"] = true
			}
			_, e = tx.Model("bm_social_deliveries").Data(g.Map{"publisher_id": p.ID, "plan_id": id, "variant_id": t.row["id"], "account_id": t.account["id"], "variant_version": t.row["version"], "publish_mode": publishingMode(t.account), "snapshot": encode(snapshot), "cycle_at": t.cycle, "due_at": t.due, "next_at": t.due}).OnConflict("variant_id,variant_version,account_id,cycle_at").InsertIgnore()
			if e != nil {
				return e
			}
			queued++
		}
		if immediate && queued == 0 {
			if active > 0 {
				return nil
			}
			return errors.New("this batch has already been submitted or needs delivery verification; it will not be published again")
		}
		updates := g.Map{"status": "scheduled", "next_at": earliest, "error": ""}
		if fillDate {
			updates["config"] = encode(c)
		}
		_, e = tx.Model("bm_social_plans").Where("id", id).Data(updates).Update()
		return e
	})
}
func PlanControl(ctx context.Context, p *governance.Principal, id int64, operation string) error {
	permission := "social:plan:manage"
	if operation == "generate" {
		permission = "social:content:edit"
	}
	if operation == "resume" || operation == "run_now" {
		permission = "social:publish"
	}
	plan, e := scoped(ctx, p, "bm_social_plans", id, permission)
	if e != nil {
		return e
	}
	switch operation {
	case "delete":
		return deletePlan(ctx, p, plan)
	case "run_now":
		return RunNow(ctx, p, id)
	case "generate":
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			live, e := tx.GetOne(`SELECT * FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
			if e != nil {
				return e
			}

			pending, e := tx.Model("bm_social_deliveries").Where("plan_id", id).Where("status", "publishing").Count()
			if e != nil {
				return e
			}
			if pending > 0 {
				return errors.New("wait for in-flight publishing")
			}
			if live["status"].String() == "generating" {
				return errors.New("generation already running")
			}
			if _, e = tx.Model("bm_social_deliveries").Where("plan_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).Data(g.Map{"status": "cancelled", "error": "new content requested"}).Update(); e != nil {
				return e
			}
			version := live["version"].Int() + 1
			if _, e = tx.Model("bm_social_plans").Where("id", id).Data(g.Map{"version": version, "status": "generating", "error": ""}).Update(); e != nil {
				return e
			}
			_, e = queue(ctx, p, id, 0, "generate", "", g.Map{"version": version})
			return e
		})
	case "pause":
		_, e = g.DB().Model("bm_social_plans").Where("id", id).Data("status", "paused").Update()
	case "resume":
		if plan["status"].String() != "paused" {
			return errors.New("only paused plans can resume")
		}
		pending, err := g.DB().Model("bm_social_deliveries").Ctx(ctx).Where("plan_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).All()
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return Schedule(ctx, p, id)
		}
		for _, d := range pending {
			a, err := accountAccess(ctx, p, d["account_id"].Int64(), "social:publish", true)
			if err != nil {
				return err
			}
			if err = validatePublishingAccount(ctx, a); err != nil {
				return err
			}
		}
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			live, err := tx.GetOne(`SELECT status FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
			if err != nil {
				return err
			}
			if live["status"].String() != "paused" {
				return errors.New("plan changed while resuming")
			}
			if _, err := tx.Model("bm_social_deliveries").Where("plan_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).Data(g.Map{"publisher_id": p.ID, "status": "scheduled", "next_at": time.Now(), "error": ""}).Update(); err != nil {
				return err
			}
			_, err = tx.Model("bm_social_plans").Where("id", id).Where("status", "paused").Data("status", "scheduled").Update()
			return err
		})
	case "archive":
		_, e = g.DB().Model("bm_social_plans").Where("id", id).Data("status", "archived").Update()
	default:
		return errors.New("unsupported plan operation")
	}
	if e == nil {
		e = securityAudit(ctx, p, operation, plan["id"].Int64())
	}
	return e
}

func ManualVariants(ctx context.Context, p *governance.Principal, id int64) error {
	plan, e := scoped(ctx, p, "bm_social_plans", id, "social:content:edit")
	if e != nil {
		return e
	}
	c, e := planConfig(plan)
	if e != nil {
		return e
	}
	source, e := sourceContext(ctx, p, plan, c)
	if e != nil {
		return e
	}
	for _, platform := range c.Platforms {
		for _, m := range c.Markets {
			_, e = g.DB().Model("bm_social_variants").Ctx(ctx).Data(g.Map{"plan_id": id, "plan_version": plan["version"], "platform": platform, "country": m.Country, "language": m.Language, "timezone": m.Timezone, "link": c.Link, "source_snapshot": encode(source)}).OnConflict("plan_id,plan_version,platform,country,language").InsertIgnore()
			if e != nil {
				return e
			}
		}
	}
	return nil
}
