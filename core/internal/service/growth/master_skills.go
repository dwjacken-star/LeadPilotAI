package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func (x *masterExec) execute() (g.Map, error) {
	switch x.Skill {
	case "MASTER":
		return x.plan()
	case "S01":
		return x.collectIntelligence(false)
	case "S02":
		return x.analyzeEvidence(false)
	case "S03":
		return x.analyzeEvidence(true)
	case "S04":
		return x.collectIntelligence(true)
	case "S05":
		return x.findCompanies()
	case "S06":
		return x.contacts()
	case "S07":
		return x.clean()
	case "S08":
		return x.score()
	case "S09":
		return x.match()
	case "S10":
		return x.compose()
	case "S11":
		return x.dispatch()
	case "S12":
		return x.replies()
	case "S13":
		return x.analytics()
	case "S14":
		return x.synchronize()
	}
	return nil, errors.New("unknown registered skill")
}
func (x *masterExec) limit() int {
	n := x.Snapshot.Config.MaxItems
	if x.Settings.MaxItems < n {
		n = x.Settings.MaxItems
	}
	return n
}
func (x *masterExec) traceContext() context.Context {
	return context.WithValue(x.Ctx, masterTraceKey{}, masterTrace{RunID: x.Run["id"].Int64(), Skill: x.Skill})
}

// Test seams substitute providers, never bypass permission, budget or quality checks.
var MasterSearchSources = func(ctx context.Context, p *governance.Principal, query string, m Market, n int, news bool) ([]SearchSource, error) {
	if !news {
		return SearchBusinesses(ctx, p, query, m, n, "search")
	}
	var result struct {
		News []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
			Date    string `json:"date"`
		} `json:"news"`
	}
	if e := callSerper(ctx, p, "search", serperSearchURL+"news", g.Map{"q": query, "gl": strings.ToLower(m.Country), "hl": m.Language, "num": n, "tbs": "qdr:w"}, &result); e != nil {
		return nil, e
	}
	out := []SearchSource{}
	for _, r := range result.News {
		out = append(out, SearchSource{URL: r.Link, SourceURL: r.Link, Title: r.Title, Description: r.Snippet, Kind: "news", PublishedAt: r.Date})
	}
	return out, nil
}

func (x *masterExec) search(query string, m Market, n int, news bool) ([]SearchSource, error) {
	cacheKey := masterHash(g.Map{"q": query, "market": m, "limit": n, "news": news})
	cached, e := g.DB().Model("bm_gm_search_cache").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("cache_key", cacheKey).Where("expires_at>now()").Value("sources")
	if e != nil {
		return nil, e
	}
	if !cached.IsEmpty() {
		var out []SearchSource
		e = json.Unmarshal(cached.Bytes(), &out)
		return out, e
	}
	id, e := x.reserve("search", 0)
	if e != nil {
		return nil, e
	}
	out, e := MasterSearchSources(x.traceContext(), x.P, query, m, n, news)
	if e != nil {
		return nil, e
	}
	if len(out) > n {
		out = out[:n]
	}
	_, e = g.DB().Exec(x.Ctx, `INSERT INTO bm_gm_search_cache(company_id,dept_id,cache_key,sources,expires_at) VALUES($1,$2,$3,$4,now()+interval '6 hours') ON CONFLICT(company_id,dept_id,cache_key) DO UPDATE SET sources=excluded.sources,expires_at=excluded.expires_at`, x.Run["company_id"], x.P.DeptID, cacheKey, jsonText(out))
	if e != nil {
		return nil, e
	}
	_, e = g.DB().Model("bm_gm_usage").Ctx(x.Ctx).Where("id", id).Data("status", "completed").Update()
	return out, e
}
func (x *masterExec) evidence(source SearchSource, m Market, body string, metadata g.Map) (int64, error) {
	u, e := safeURL(source.URL)
	if e != nil {
		return 0, e
	}
	u.Fragment = ""
	if source.Kind == "" {
		source.Kind = "website"
	}
	if metadata == nil {
		metadata = g.Map{}
	}
	metadata["snippet"] = cleanText(source.Description, 2000)
	metadata["published_at_source"] = source.PublishedAt
	fingerprint := masterHash(g.Map{"url": u.String(), "title": source.Title, "body": body})
	_, e = g.DB().Model("bm_gm_evidence").Ctx(x.Ctx).Data(g.Map{"company_id": x.Run["company_id"], "dept_id": x.P.DeptID, "owner_id": x.P.ID, "url": u.String(), "fingerprint": fingerprint, "title": cleanText(source.Title, 500), "body": cleanText(body, 16000), "country": m.Country, "language": m.Language, "kind": source.Kind, "metadata": jsonText(metadata)}).OnConflict("company_id,dept_id,fingerprint").InsertIgnore()
	if e != nil {
		return 0, e
	}
	v, e := g.DB().Model("bm_gm_evidence").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("fingerprint", fingerprint).Value("id")
	if e != nil {
		return 0, e
	}
	_, e = g.DB().Model("bm_gm_run_evidence").Ctx(x.Ctx).Data(g.Map{"run_id": x.Run["id"], "evidence_id": v.Int64()}).InsertIgnore()
	return v.Int64(), e
}
func (x *masterExec) evidenceRows() (gdb.Result, error) {
	return g.DB().Model("bm_gm_evidence e").Ctx(x.Ctx).InnerJoin("bm_gm_run_evidence l", "l.evidence_id=e.id").Where("l.run_id", x.Run["id"]).Where("e.dept_id", x.P.DeptID).Where("e.company_id", x.Run["company_id"]).Fields("e.*").Order("e.id").Limit(100).All()
}
func (x *masterExec) prospectRows() (gdb.Result, error) {
	return g.DB().Model("bm_gm_prospects p").Ctx(x.Ctx).InnerJoin("bm_gm_run_prospects l", "l.prospect_id=p.id").Where("l.run_id", x.Run["id"]).Where("p.dept_id", x.P.DeptID).Where("p.company_id", x.Run["company_id"]).Fields("p.*").Order("p.id").Limit(100).All()
}
func masterDomainAllowed(value string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	host := sourceHost(value)
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
func (x *masterExec) collectIntelligence(intent bool) (g.Map, error) {
	ids := []int64{}
	warnings := []string{}
	c, p := x.Snapshot.Config, x.Snapshot.Pack
	kind := "industry_news"
	if intent {
		kind = "purchase_intent"
	}
	for _, m := range c.Markets {
		query := strings.Join(p.Terms, " OR ") + " " + m.Country + " " + cleanText(c.Requirement, 500)
		if intent {
			query += " " + strings.Join(p.IntentTerms, " OR ")
		}
		if len(p.SourceDomains) > 0 {
			parts := []string{}
			for _, d := range p.SourceDomains {
				parts = append(parts, "site:"+d)
			}
			query += " (" + strings.Join(parts, " OR ") + ")"
		}
		sources, e := x.search(query, m, x.limit(), !intent)
		if e != nil {
			return nil, e
		}
		for _, s := range sources {
			if !masterDomainAllowed(s.URL, p.SourceDomains) || masterExcluded(s.Title+" "+s.Description, append(append([]string{}, p.ExcludeKeywords...), c.ExcludeKeywords...)) {
				continue
			}
			s.Kind = kind
			body, level := s.Description, "search_excerpt"
			if page, _, err := x.page(s.URL, m); err == nil {
				body, level = page.Text, "page_text"
			} else {
				var wait *masterWait
				if errors.As(err, &wait) {
					return nil, err
				}
				warnings = append(warnings, cleanText(err.Error(), 200))
			}
			id, e := x.evidence(s, m, body, g.Map{"evidence_level": level, "search_query": query})
			if e != nil {
				warnings = append(warnings, cleanText(e.Error(), 200))
				continue
			}
			ids = append(ids, id)
		}
	}
	out := g.Map{"evidence_ids": ids, "coverage": g.Map{"markets": c.Markets, "sample_count": len(ids), "scope": "configured public search results; not exhaustive market coverage"}, "warnings": warnings}
	if intent && len(ids) > 0 {
		rows, e := x.evidenceRows()
		if e != nil {
			return nil, e
		}
		var result masterFindings
		e = x.ai("Identify explicit public purchasing needs. Cite supplied evidence IDs and short exact supporting excerpts. Distinguish explicit RFQ from inferred interest. Report any deadline only if supplied; use unknown otherwise. Never invent buyers. Return at most 10 findings.", g.Map{"industry": p, "evidence": rows}, masterFindingsSchema(), &result)
		if e != nil {
			return nil, e
		}
		if e = masterCheckFindings(result, rows); e != nil {
			return nil, e
		}
		out["signals"] = result.Findings
	}
	return out, nil
}

type masterFinding struct {
	Title       string  `json:"title"`
	Summary     string  `json:"summary"`
	EvidenceIDs []int64 `json:"evidence_ids"`
	Excerpt     string  `json:"excerpt"`
	Confidence  float64 `json:"confidence"`
	Uncertainty string  `json:"uncertainty"`
}
type masterFindings struct {
	Findings []masterFinding `json:"findings"`
	Scope    string          `json:"scope"`
}

func masterFindingsSchema() g.Map {
	return schema(g.Map{"findings": g.Map{"type": "array", "items": schema(g.Map{"title": strSchema(), "summary": strSchema(), "evidence_ids": g.Map{"type": "array", "items": g.Map{"type": "integer"}}, "excerpt": strSchema(), "confidence": g.Map{"type": "number"}, "uncertainty": strSchema()})}, "scope": strSchema()})
}
func masterCheckFindings(result masterFindings, rows gdb.Result) error {
	if len(result.Findings) > 20 {
		return errors.New("too many findings")
	}
	for _, f := range result.Findings {
		if f.Title == "" || f.Summary == "" || f.Confidence < 0 || f.Confidence > 1 || len(f.EvidenceIDs) == 0 || f.Excerpt == "" {
			return errors.New("finding has no usable evidence")
		}
		matched := false
		for _, id := range f.EvidenceIDs {
			found := false
			for _, r := range rows {
				if r["id"].Int64() == id {
					found = true
					if strings.Contains(strings.ToLower(r["body"].String()+" "+r["title"].String()), strings.ToLower(f.Excerpt)) {
						matched = true
					}
				}
			}
			if !found {
				return errors.New("AI cited unavailable evidence")
			}
		}
		if !matched {
			return errors.New("AI evidence excerpt was not found in the supplied source")
		}
	}
	return nil
}
func (x *masterExec) analyzeEvidence(hotspot bool) (g.Map, error) {
	rows, e := x.evidenceRows()
	if e != nil {
		return nil, e
	}
	analytics := x.previous("S13")
	if len(rows) == 0 && len(analytics) == 0 {
		return g.Map{"findings": []masterFinding{}, "scope": "no usable evidence"}, nil
	}
	// WF06 also uses recent evidence from this company and department.
	if len(rows) == 0 {
		rows, e = g.DB().Model("bm_gm_evidence").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("collected_at>now()-interval '7 days'").Order("id DESC").Limit(30).All()
		if e != nil {
			return nil, e
		}
	}
	if len(rows) == 0 {
		return g.Map{"findings": []masterFinding{}, "analytics": analytics, "scope": "No current market evidence; no market claims generated"}, nil
	}
	instruction := "Analyze market changes and regulations in the configured industry. Separate source facts from inferences. Cite evidence IDs and exact excerpts. A news report is not authoritative legal verification. Return at most 10 findings."
	if hotspot {
		instruction = "Find product, service and application demand signals. Rank only observations in this sample, never claim global sales or market share. Cite supplied evidence IDs and exact supporting excerpts. Return at most 10 findings."
	}
	var result masterFindings
	if e = x.ai(instruction, g.Map{"industry": x.Snapshot.Pack, "evidence": rows, "analytics": analytics}, masterFindingsSchema(), &result); e != nil {
		return nil, e
	}
	if e = masterCheckFindings(result, rows); e != nil {
		return nil, e
	}
	return g.Map{"findings": result.Findings, "scope": result.Scope, "sample_count": len(rows), "as_of": time.Now()}, nil
}
func (x *masterExec) findCompanies() (g.Map, error) {
	c, p := x.Snapshot.Config, x.Snapshot.Pack
	type query struct {
		Country  string `json:"country"`
		Language string `json:"language"`
		Query    string `json:"query"`
	}
	var result struct {
		Queries []query `json:"queries"`
	}
	maxQueries := len(c.Markets) * c.QueriesPerMarket
	if maxQueries > c.Budget.Searches {
		maxQueries = c.Budget.Searches
	}
	if e := x.ai(fmt.Sprintf("Generate at most %d focused B2B company search queries. Respect the exact configured markets and languages. Use buyer types and industry terms. Include relevant explicit purchase signals when available. Return queries only; never invent company names or emails.", maxQueries), g.Map{"industry": p, "markets": c.Markets, "requirement": c.Requirement, "strategy": x.previous("MASTER"), "signals": x.previous("S04")}, schema(g.Map{"queries": g.Map{"type": "array", "items": schema(g.Map{"country": strSchema(), "language": strSchema(), "query": strSchema()})}}), &result); e != nil {
		return nil, e
	}
	if len(result.Queries) == 0 || len(result.Queries) > maxQueries {
		return nil, errors.New("AI returned an invalid search query count")
	}
	ids := []int64{}
	seen := map[string]bool{}
	perMarket := map[string]int{}
	for _, q := range result.Queries {
		var market Market
		for _, m := range c.Markets {
			if m.Country == q.Country && m.Language == q.Language {
				market = m
			}
		}
		if market.Country == "" || q.Query == "" || len(q.Query) > 1500 {
			return nil, errors.New("search query is outside configured markets")
		}
		mk := market.Country + ":" + market.Language
		perMarket[mk]++
		if perMarket[mk] > c.QueriesPerMarket {
			return nil, errors.New("query count exceeds market allocation")
		}
		sources, e := x.search(q.Query, market, x.limit(), false)
		if e != nil {
			return nil, e
		}
		for _, s := range sources {
			if len(ids) >= x.limit() {
				break
			}
			u, e := safeURL(s.URL)
			if e != nil {
				continue
			}
			host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			if seen[host] || masterExcluded(s.Title+" "+s.Description, append(append([]string{}, p.ExcludeKeywords...), c.ExcludeKeywords...)) {
				continue
			}
			seen[host] = true
			id, e := x.evidence(s, market, s.Description, g.Map{"evidence_level": "discovery_excerpt", "search_query": q.Query})
			if e != nil {
				return nil, e
			}
			website := u.Scheme + "://" + u.Host
			_, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Data(g.Map{"company_id": x.Run["company_id"], "dept_id": x.P.DeptID, "owner_id": x.P.ID, "company": cleanText(s.Title, 300), "website": website, "language": market.Language, "facts": jsonText(g.Map{"target_country": market.Country, "target_language": market.Language}), "sources": jsonText([]int64{id})}).OnConflict("company_id,dept_id,website,email").InsertIgnore()
			if e != nil {
				return nil, e
			}
			v, e := g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("website", website).Where("email", "").Value("id")
			if e != nil {
				return nil, e
			}
			ids = append(ids, v.Int64())
			if e = x.linkProspect(v.Int64()); e != nil {
				return nil, e
			}
		}
	}
	return g.Map{"prospect_ids": ids, "queries": result.Queries, "count": len(ids)}, nil
}
func (x *masterExec) linkProspect(id int64) error {
	_, e := g.DB().Model("bm_gm_run_prospects").Ctx(x.Ctx).Data(g.Map{"run_id": x.Run["id"], "prospect_id": id}).InsertIgnore()
	return e
}
func (x *masterExec) page(value string, m Market) (WebPage, int64, error) {
	cached, e := g.DB().Model("bm_gm_evidence").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("url", value).Where("kind", "contact_page").Where("collected_at>now()-interval '24 hours'").Order("id DESC").One()
	if e != nil {
		return WebPage{}, 0, e
	}
	if len(cached) > 0 {
		var page WebPage
		meta := jsonMap(cached["metadata"])
		if gconv.Scan(meta["page"], &page) == nil && page.Text != "" {
			_, e = g.DB().Model("bm_gm_run_evidence").Ctx(x.Ctx).Data(g.Map{"run_id": x.Run["id"], "evidence_id": cached["id"]}).InsertIgnore()
			return page, cached["id"].Int64(), e
		}
	}
	usage, e := x.reserve("scrape", 0)
	if e != nil {
		return WebPage{}, 0, e
	}
	page, e := CollectWebsite(x.traceContext(), x.P, value)
	if e != nil {
		return page, 0, e
	}
	if _, e = safeURL(page.URL); e != nil {
		return page, 0, e
	}
	page.Text = cleanText(page.Text, 16000)
	id, e := x.evidence(SearchSource{URL: page.URL, Title: page.URL, Kind: "contact_page"}, m, page.Text, g.Map{"evidence_level": "page_text", "page": page})
	if e == nil {
		_, e = g.DB().Model("bm_gm_usage").Ctx(x.Ctx).Where("id", usage).Data("status", "completed").Update()
	}
	return page, id, e
}

type masterContactFacts struct {
	Company         string  `json:"company"`
	Country         string  `json:"country"`
	CountryEvidence string  `json:"country_evidence"`
	Language        string  `json:"language"`
	Summary         string  `json:"summary"`
	Relevant        bool    `json:"relevant"`
	Confidence      float64 `json:"confidence"`
	Phone           string  `json:"phone"`
	Need            string  `json:"need"`
	NeedEvidence    string  `json:"need_evidence"`
}

func (x *masterExec) contacts() (g.Map, error) {
	rows, e := x.prospectRows()
	if e != nil {
		return nil, e
	}
	count := 0
	warnings := []string{}
	for _, r := range rows {
		if r["email"].String() != "" {
			continue
		}
		facts := jsonMap(r["facts"])
		m := Market{Country: gconv.String(facts["target_country"]), Language: r["language"].String()}
		page, eid, e := x.page(r["website"].String(), m)
		if e != nil {
			var wait *masterWait
			if errors.As(e, &wait) {
				return nil, e
			}
			warnings = append(warnings, fmt.Sprintf("%s: %s", r["website"].String(), cleanText(e.Error(), 180)))
			continue
		}
		pages := []WebPage{page}
		sources := []int64{eid}
		for _, link := range page.Links {
			if len(pages) >= 3 {
				break
			}
			more, id, err := x.page(link, m)
			if err != nil {
				var wait *masterWait
				if errors.As(err, &wait) {
					return nil, err
				}
				continue
			}
			pages = append(pages, more)
			sources = append(sources, id)
		}
		text := ""
		emails := map[string]bool{}
		for _, p := range pages {
			text += "\n" + p.Text
			for _, email := range p.Emails {
				v, _, ok := emailvalidation.Normalize(email)
				if ok {
					emails[v] = true
				}
			}
		}
		text = cleanText(text, 20000)
		var result masterContactFacts
		err := x.ai("Extract business facts from these public pages. Country must be supported by a short exact excerpt, otherwise leave it empty. Phone must appear verbatim in the pages. A business description alone is not an explicit purchase need: leave need and need_evidence empty unless a current procurement request is present. Do not infer private contacts. Return the website language.", g.Map{"industry": x.Snapshot.Pack, "requirement": x.Snapshot.Config.Requirement, "text": text}, schema(g.Map{"company": strSchema(), "country": strSchema(), "country_evidence": strSchema(), "language": strSchema(), "summary": strSchema(), "relevant": g.Map{"type": "boolean"}, "confidence": g.Map{"type": "number"}, "phone": strSchema(), "need": strSchema(), "need_evidence": strSchema()}), &result)
		if err != nil {
			return nil, err
		}
		if result.Confidence < 0 || result.Confidence > 1 {
			return nil, errors.New("invalid contact confidence")
		}
		if result.Country != "" && (!regexpCountry(result.Country) || result.CountryEvidence == "" || !strings.Contains(strings.ToLower(text), strings.ToLower(result.CountryEvidence))) {
			result.Country = ""
			result.CountryEvidence = ""
		}
		if result.Phone != "" && !strings.Contains(text, result.Phone) {
			result.Phone = ""
		}
		if result.NeedEvidence == "" || !strings.Contains(strings.ToLower(text), strings.ToLower(result.NeedEvidence)) {
			result.Need = ""
			result.NeedEvidence = ""
		}
		facts["qualification"] = result
		facts["confidence"] = result.Confidence
		facts["relevant"] = result.Relevant
		facts["summary"] = result.Summary
		facts["country_evidence"] = result.CountryEvidence
		facts["need"] = result.Need
		facts["need_evidence"] = result.NeedEvidence
		state := "collected"
		if len(emails) == 0 {
			state = "no_email"
		}
		values := g.Map{"company": cleanText(result.Company, 300), "country": result.Country, "language": cleanText(result.Language, 30), "phone": cleanText(result.Phone, 100), "facts": jsonText(facts), "sources": jsonText(sources), "state": state, "updated_at": time.Now()}
		if values["company"] == "" {
			values["company"] = r["company"].String()
		}
		if _, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("id", r["id"]).Data(values).Update(); e != nil {
			return nil, e
		}
		for email := range emails {
			if count >= x.limit() {
				break
			}
			v := g.Map{}
			for key, value := range values {
				v[key] = value
			}
			v["email"], v["website"], v["company_id"], v["dept_id"], v["owner_id"] = email, r["website"].String(), x.Run["company_id"], x.P.DeptID, x.P.ID
			if _, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Data(v).OnConflict("company_id,dept_id,website,email").Save(); e != nil {
				return nil, e
			}
			id, e := g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("website", r["website"]).Where("email", email).Value("id")
			if e != nil {
				return nil, e
			}
			if e = x.linkProspect(id.Int64()); e != nil {
				return nil, e
			}
			count++
		}
	}
	return g.Map{"public_email_count": count, "warnings": warnings}, nil
}

var MasterVerifyEmail = emailvalidation.Check

func (x *masterExec) clean() (g.Map, error) {
	rows, e := x.prospectRows()
	if e != nil {
		return nil, e
	}
	counts := g.Map{"verified": 0, "invalid": 0, "unconfirmed": 0, "filtered": 0, "suppressed": 0}
	filters := append(append([]string{}, x.Snapshot.Pack.ExcludeKeywords...), x.Snapshot.Config.ExcludeKeywords...)
	for _, r := range rows {
		email := r["email"].String()
		if email == "" {
			continue
		}
		state := "unconfirmed"
		verification := g.Map{}
		facts := jsonMap(r["facts"])
		if masterExcluded(email+" "+r["company"].String(), filters) {
			state = "filtered"
		} else if r["suppressed"].Bool() {
			state = "suppressed"
		} else if !gconv.Bool(facts["relevant"]) || gconv.Float64(facts["confidence"]) < x.Snapshot.Pack.MinConfidence || r["country"].String() == "" || r["country"].String() != gconv.String(facts["target_country"]) {
			verification = g.Map{"status": "unknown", "code": "qualification_requires_review", "stage": "evidence"}
		} else {
			sender := x.Snapshot.Config.Senders[0]
			suppressed, e := deliveryprefs.IsSuppressed(x.Ctx, sender, email)
			if e != nil {
				return nil, e
			}
			if suppressed {
				state = "suppressed"
			} else {
				if e = x.check(); e != nil {
					return nil, e
				}
				result := MasterVerifyEmail(x.Ctx, email, sender, true)
				verification = gconv.Map(result)
				if result.Status == emailvalidation.Invalid {
					state = "invalid"
				} else if result.Status == emailvalidation.Valid && result.Stage == "smtp" {
					state = "verified"
				}
			}
		}
		if _, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("id", r["id"]).Data(g.Map{"state": state, "verification": jsonText(verification), "updated_at": time.Now()}).Update(); e != nil {
			return nil, e
		}
		counts[state] = gconv.Int(counts[state]) + 1
	}
	return counts, nil
}
func masterScore(pack MasterPack, facts g.Map, state, country string) (float64, g.Map) {
	factors := map[string]float64{"relevance": 0, "market": 0, "contact": 0, "intent": 0, "completeness": 0}
	if gconv.Bool(facts["relevant"]) {
		confidence := gconv.Float64(facts["confidence"])
		if confidence >= pack.MinConfidence {
			factors["relevance"] = confidence
		}
	}
	if country != "" && country == gconv.String(facts["target_country"]) && gconv.String(facts["country_evidence"]) != "" {
		factors["market"] = 1
	}
	if state == "verified" {
		factors["contact"] = 1
	}
	if gconv.String(facts["need_evidence"]) != "" {
		factors["intent"] = 1
	}
	if gconv.String(facts["summary"]) != "" {
		factors["completeness"] += .5
	}
	if country != "" {
		factors["completeness"] += .5
	}
	total := float64(0)
	breakdown := g.Map{}
	for key, weight := range pack.ScoreWeights {
		value := float64(weight) * factors[key]
		total += value
		breakdown[key] = g.Map{"weight": weight, "factor": factors[key], "score": value}
	}
	return total, breakdown
}
func (x *masterExec) score() (g.Map, error) {
	rows, e := x.prospectRows()
	if e != nil {
		return nil, e
	}
	count := 0
	for _, r := range rows {
		if r["email"].String() == "" {
			continue
		}
		score, parts := masterScore(x.Snapshot.Pack, jsonMap(r["facts"]), r["state"].String(), r["country"].String())
		_, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("id", r["id"]).Data(g.Map{"score": score, "scoring": jsonText(g.Map{"breakdown": parts, "pack_id": x.Snapshot.PackID, "pack_version": x.Snapshot.PackVersion, "minimum": x.Snapshot.Pack.MinScore}), "updated_at": time.Now()}).Update()
		if e != nil {
			return nil, e
		}
		count++
	}
	return g.Map{"scored": count, "pack_version": x.Snapshot.PackVersion}, nil
}
func (x *masterExec) match() (g.Map, error) {
	rows, e := x.prospectRows()
	if e != nil {
		return nil, e
	}
	products := gconv.Maps(x.Snapshot.Sources["products"])
	validIDs := map[int64]bool{}
	for _, p := range products {
		validIDs[gconv.Int64(p["id"])] = true
	}
	count := 0
	for _, r := range rows {
		if r["state"].String() != "verified" || r["score"].Float64() < float64(x.Snapshot.Pack.MinScore) {
			continue
		}
		var result struct {
			ProductIDs []int64  `json:"product_ids"`
			Reason     string   `json:"reason"`
			Missing    []string `json:"missing_conditions"`
			Confidence float64  `json:"confidence"`
		}
		if len(products) == 0 {
			result.Reason = "No approved offerings selected; company-level outreach only"
			result.Missing = []string{"offering-specific requirements"}
		} else {
			if e = x.ai("Match only supplied approved product/service IDs to the customer evidence. Return empty product_ids if exact requirements cannot be established. For fitment, standards, compatibility and contractual terms require explicit evidence. Explain missing conditions; no invented equivalences.", g.Map{"industry": x.Snapshot.Pack, "customer": jsonMap(r["facts"]), "approved_offerings": products}, schema(g.Map{"product_ids": g.Map{"type": "array", "items": g.Map{"type": "integer"}}, "reason": strSchema(), "missing_conditions": stringsSchema(), "confidence": g.Map{"type": "number"}}), &result); e != nil {
				return nil, e
			}
			for _, id := range result.ProductIDs {
				if !validIDs[id] {
					return nil, errors.New("AI selected an unapproved offering")
				}
			}
			if result.Confidence < 0 || result.Confidence > 1 {
				return nil, errors.New("invalid matching confidence")
			}
		}
		_, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("id", r["id"]).Data("matching", jsonText(result)).Update()
		if e != nil {
			return nil, e
		}
		count++
	}
	return g.Map{"matched": count, "approved_offering_count": len(products)}, nil
}
func (x *masterExec) synchronize() (g.Map, error) {
	rows, e := x.prospectRows()
	if e != nil {
		return nil, e
	}
	linked := 0
	if x.Snapshot.Code == "WF02" || x.Snapshot.Code == "WF03" || x.Snapshot.Code == "WF05" {
		if e = masterRequired(x.P, "crm:list", "crm:manage"); e != nil {
			return nil, e
		}
		for _, r := range rows {
			if r["state"].String() != "verified" || r["suppressed"].Bool() || r["score"].Float64() < float64(x.Snapshot.Pack.MinScore) {
				continue
			}
			e = g.DB().Transaction(x.Ctx, func(ctx context.Context, tx gdb.TX) error {
				if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(615101)`); e != nil {
					return e
				}
				existing, e := tx.Model("bm_crm_customers").Where("dept_id", x.P.DeptID).Where("email", r["email"]).One()
				if e != nil {
					return e
				}
				var id int64
				if len(existing) > 0 {
					if !existing["deleted_at"].IsNil() {
						return nil
					}
					if _, e = crm.Customer(ctx, x.P, existing["id"].Int64(), "crm:manage"); e != nil {
						return e
					}
					id = existing["id"].Int64()
				} else {
					id, e = crm.Save(ctx, x.P, 0, g.Map{"email": r["email"].String(), "company": r["company"].String(), "country": r["country"].String(), "name": "", "stage": "new", "note": "Growth Master verified public business contact. Source website: " + r["website"].String()})
					if e != nil {
						return e
					}
				}
				_, e = tx.Model("bm_gm_prospects").Where("id", r["id"]).Data("crm_id", id).Update()
				return e
			})
			if e != nil {
				return nil, e
			}
			linked++
		}
	}
	summary := g.Map{"crm_links": linked, "prospect_count": len(rows), "workflow": x.Snapshot.Code, "pack_version": x.Snapshot.PackVersion, "completed_at": time.Now()}
	if e = masterAudit(x.Ctx, x.P, x.Run["company_id"].Int64(), x.Run["id"].Int64(), 0, "workflow_summary", summary); e != nil {
		return nil, e
	}
	_, e = g.DB().Model("bm_gm_runs").Ctx(x.Ctx).Where("id", x.Run["id"]).Data("summary", jsonText(summary)).Update()
	return summary, e
}
func masterCanonicalSite(value string) string {
	u, e := url.Parse(value)
	if e != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
