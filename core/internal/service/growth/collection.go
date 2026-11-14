package growth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func discover(ctx context.Context, p *governance.Principal, queryID int64) error {
	query, e := g.DB().Model("bm_growth_queries").Ctx(ctx).Where("id", queryID).One()
	if e != nil {
		return e
	}
	if len(query) == 0 {
		return errors.New("query not found")
	}
	scope, c, e := runContext(ctx, p, query["run_id"].Int64())
	if e != nil {
		return e
	}
	market := Market{Country: query["country"].String(), Language: query["language"].String()}
	for _, m := range c.Markets {
		if m.Country == market.Country && m.Language == market.Language {
			market = m
			break
		}
	}
	sources := []SearchSource{}
	if query["status"].String() == "searched" || query["status"].String() == "completed" || query["status"].String() == "no_results" {
		if e = json.Unmarshal(query["sources"].Bytes(), &sources); e != nil {
			return e
		}
	} else {
		sources, e = SearchBusinesses(ctx, p, query["query"].String(), market, c.resultsPerQuery(), c.searchChannel())
		if e != nil {
			_, _ = g.DB().Model("bm_growth_queries").Ctx(ctx).Where("id", queryID).Data(g.Map{"status": "failed", "error": cleanText(e.Error(), 500)}).Update()
			return e
		}
		_, e = g.DB().Model("bm_growth_queries").Ctx(ctx).Where("id", queryID).Data(g.Map{"sources": jsonText(sources), "status": "searched", "error": ""}).Update()
		if e != nil {
			return e
		}
	}
	for _, source := range sources {
		identity := ""
		website := source.URL
		if website != "" {
			u, err := safeURL(website)
			if err != nil {
				continue
			}
			identity = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		} else if source.PlaceID != "" {
			identity = "place:" + source.PlaceID
		} else {
			continue
		}
		hash := sha256.Sum256([]byte(market.Country + ":" + identity))
		key := hex.EncodeToString(hash[:])
		state := "discovered"
		if website == "" {
			state = "no_website"
		}
		_, e = g.DB().Model("bm_growth_candidates").Ctx(ctx).Data(g.Map{"run_id": scope["run_id"], "plan_id": scope["plan_id"], "company_id": scope["company_id"], "dept_id": p.DeptID, "owner_id": p.ID, "identity_key": key, "website": website, "company": cleanText(source.Title, 300), "target_country": market.Country, "country": "", "language": market.Language, "address": cleanText(source.Address, 1000), "phone": cleanText(source.Phone, 100), "facts": jsonText(g.Map{"discovery": source, "country_status": "target_market_unconfirmed"}), "sources": jsonText([]g.Map{{"url": source.SourceURL, "evidence": source.Description, "collected_at": time.Now().UTC().Format(time.RFC3339)}}), "state": state}).InsertIgnore()
		if e != nil {
			return e
		}
		candidate, e := g.DB().Model("bm_growth_candidates").Ctx(ctx).Where("run_id", scope["run_id"]).Where("identity_key", key).Value("id")
		if e != nil {
			return e
		}
		if website != "" {
			if e = childJob(ctx, p, "collect", fmt.Sprint(queryID), fmt.Sprintf("collect-candidate:%d", candidate.Int64()), query["run_id"].Int64(), g.Map{"candidate_id": candidate.Int64(), "url": website, "country": market.Country, "language": market.Language}); e != nil {
				return e
			}
		}
	}
	status := "completed"
	if len(sources) == 0 {
		status = "no_results"
	}
	_, e = g.DB().Model("bm_growth_queries").Ctx(ctx).Where("id", queryID).Data(g.Map{"status": status, "error": ""}).Update()
	return e
}

type businessContact struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	Evidence string `json:"evidence"`
}
type qualifiedBusiness struct {
	CountryEvidence string            `json:"country_evidence"`
	Company         string            `json:"company"`
	Country         string            `json:"country"`
	Language        string            `json:"language"`
	Relevant        bool              `json:"relevant"`
	Confidence      float64           `json:"confidence"`
	Evidence        string            `json:"evidence"`
	Summary         string            `json:"summary"`
	Address         string            `json:"address"`
	AddressEvidence string            `json:"address_evidence"`
	Phone           string            `json:"phone"`
	PhoneEvidence   string            `json:"phone_evidence"`
	Contacts        []businessContact `json:"contacts"`
}

func collect(ctx context.Context, p *governance.Principal, payload g.Map) (err error) {
	runID := gconv.Int64(payload["run_id"])
	scope, c, e := runContext(ctx, p, runID)
	if e != nil {
		return e
	}
	candidateID := gconv.Int64(payload["candidate_id"])
	if candidateID == 0 {
		// Jobs queued by an older release are retained, but are collected by Serper.
		u, e := safeURL(gconv.String(payload["url"]))
		if e != nil {
			return e
		}
		hash := sha256.Sum256([]byte(gconv.String(payload["country"]) + ":" + strings.TrimPrefix(u.Hostname(), "www.")))
		key := hex.EncodeToString(hash[:])
		_, e = g.DB().Model("bm_growth_candidates").Data(g.Map{"run_id": runID, "plan_id": scope["plan_id"], "company_id": scope["company_id"], "dept_id": p.DeptID, "owner_id": p.ID, "identity_key": key, "website": u.String(), "target_country": payload["country"], "country": "", "language": payload["language"]}).InsertIgnore()
		if e != nil {
			return e
		}
		value, e := g.DB().Model("bm_growth_candidates").Where("run_id", runID).Where("identity_key", key).Value("id")
		if e != nil {
			return e
		}
		candidateID = value.Int64()
	}
	candidate, e := scoped(ctx, p, "bm_growth_candidates", candidateID, "outreach:run")
	if e != nil {
		return e
	}
	if candidate["run_id"].Int64() != runID {
		return errors.New("candidate does not belong to this run")
	}
	if candidate["state"].String() == "collected" {
		return nil
	}
	defer func() {
		if err != nil {
			_, _ = g.DB().Model("bm_growth_candidates").Where("id", candidateID).Data(g.Map{"state": "failed", "error": cleanText(err.Error(), 800), "updated_at": time.Now()}).Update()
		}
	}()
	_, e = g.DB().Model("bm_growth_candidates").Ctx(ctx).Where("id", candidateID).Data(g.Map{"state": "collecting", "error": "", "updated_at": time.Now()}).Update()
	if e != nil {
		return e
	}
	page, e := candidatePage(ctx, p, candidateID, candidate["website"].String())
	if e != nil {
		return e
	}
	pages := []WebPage{page}
	pageErrors := []string{}
	visited := map[string]bool{page.URL: true}
	for _, link := range page.Links {
		if len(pages)+len(pageErrors) >= c.PageLimit {
			break
		}
		if visited[link] {
			continue
		}
		visited[link] = true
		if more, e := candidatePage(ctx, p, candidateID, link); e == nil {
			pages = append(pages, more)
		} else {
			pageErrors = append(pageErrors, cleanText(e.Error(), 160))
		}
	}
	var text strings.Builder
	emailSources := map[string][]g.Map{}
	allEmails := map[string]bool{}
	filtered := map[string]bool{}
	evidencePages := []g.Map{}
	for _, part := range pages {
		text.WriteString(part.Text + "\n")
		evidencePages = append(evidencePages, g.Map{"url": part.URL, "evidence": cleanText(part.Text, 4000), "collected_at": time.Now().UTC().Format(time.RFC3339)})
		for _, raw := range part.Emails {
			email, ok := collectedEmail(raw)
			if !ok {
				continue
			}
			allEmails[email] = true
			skip := false
			for _, word := range c.ExcludeKeywords {
				if word != "" && strings.Contains(email, strings.ToLower(word)) {
					skip = true
					filtered[email] = true
					break
				}
			}
			if !skip {
				emailSources[email] = append(emailSources[email], g.Map{"url": part.URL, "evidence": email, "collected_at": time.Now().UTC().Format(time.RFC3339)})
			}
		}
	}
	emails := []string{}
	for email := range allEmails {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	// Persist acquired evidence before AI qualification, so failures never erase a company.
	_, e = g.DB().Model("bm_growth_candidates").Ctx(ctx).Where("id", candidateID).Data(g.Map{"emails": jsonText(emails), "sources": jsonText(evidencePages), "filtered_count": len(filtered), "updated_at": time.Now()}).Update()
	if e != nil {
		return e
	}
	var found qualifiedBusiness
	in := AIInput{Model: c.Model, Instruction: "Qualify this business against the requirement using ONLY supplied website evidence. Country is the two-letter ISO code of the actual company address, or empty if uncertain; country_evidence must quote the company address or country text verbatim; target market alone is not evidence. Copy company name and addresses from the evidence. Contacts must be publicly named with a verbatim evidence quote; never invent email addresses, people or roles. Return empty strings/arrays for missing fields. Language follows the company website. Even if no email is present, summarize and classify the company.", Data: g.Map{"text": cleanText(text.String(), 50000), "target_market": payload["country"], "requirement": c.Requirement}, Schema: schema(g.Map{"company": strSchema(), "country": strSchema(), "country_evidence": strSchema(), "language": strSchema(), "relevant": g.Map{"type": "boolean"}, "confidence": g.Map{"type": "number"}, "evidence": strSchema(), "summary": strSchema(), "address": strSchema(), "address_evidence": strSchema(), "phone": strSchema(), "phone_evidence": strSchema(), "contacts": g.Map{"type": "array", "items": schema(g.Map{"name": strSchema(), "role": strSchema(), "email": strSchema(), "evidence": strSchema()})}})}
	if e = IntelligenceProvider.Extract(ctx, p, "lead_qualification", in, &found); e != nil {
		return e
	}
	if found.Language == "" {
		found.Language = gconv.String(payload["language"])
	}
	grounded := func(s string) bool {
		return s != "" && strings.Contains(strings.ToLower(text.String()), strings.ToLower(s))
	}
	if !grounded(found.Company) {
		found.Company = candidate["company"].String()
	}
	if !grounded(found.Address) || !grounded(found.AddressEvidence) {
		found.Address = ""
		found.AddressEvidence = ""
	}
	if !grounded(found.Phone) || !grounded(found.PhoneEvidence) {
		found.Phone = ""
		found.PhoneEvidence = ""
	}
	contacts := []businessContact{}
	for _, contact := range found.Contacts {
		email, _, ok := emailvalidation.Normalize(contact.Email)
		if !grounded(contact.Evidence) || !grounded(contact.Name) {
			continue
		}
		if contact.Role != "" && !strings.Contains(strings.ToLower(contact.Evidence), strings.ToLower(contact.Role)) {
			contact.Role = ""
		}
		if !ok || !allEmails[email] || !strings.Contains(strings.ToLower(contact.Evidence), email) {
			contact.Email = ""
		} else {
			contact.Email = email
		}
		contacts = append(contacts, contact)
	}
	found.Contacts = contacts
	if len(found.Country) != 2 || strings.ToUpper(found.Country) != found.Country {
		found.Country = ""
	}
	confident := grounded(found.CountryEvidence) && found.Country == gconv.String(payload["country"]) && found.Confidence >= .85 && grounded(found.Evidence)
	state := "review"
	if confident {
		state = "collected"
	}
	facts := gconv.Map(found)
	facts["discovery"] = jsonMap(candidate["facts"])["discovery"]
	facts["page_errors"] = pageErrors
	facts["candidate_id"] = candidateID
	candidateState := state
	if !found.Relevant {
		candidateState = "unmatched"
	} else if len(allEmails) == 0 {
		candidateState = "no_email"
	} else if len(emailSources) == 0 {
		candidateState = "filtered"
	}
	if len(pageErrors) > 0 && candidateState == "no_email" {
		candidateState = "partial"
	}
	values := g.Map{"company": found.Company, "language": found.Language, "facts": jsonText(facts), "state": candidateState, "updated_at": time.Now()}
	if found.Country != "" && grounded(found.CountryEvidence) {
		values["country"] = found.Country
	}
	if found.Address != "" {
		values["address"] = found.Address
	}
	if found.Phone != "" {
		values["phone"] = found.Phone
	}
	if len(pageErrors) > 0 {
		values["error"] = "Some contact pages could not be collected: " + strings.Join(pageErrors, "; ")
	}
	if _, e = g.DB().Model("bm_growth_candidates").Ctx(ctx).Where("id", candidateID).Data(values).Update(); e != nil {
		return e
	}
	if !found.Relevant || len(emailSources) == 0 {
		return nil
	}
	for _, email := range emails {
		sources, allowed := emailSources[email]
		if !allowed {
			continue
		}
		reached := false
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614812)"); e != nil {
				return e
			}
			loc, _ := time.LoadLocation(c.Timezone)
			now := time.Now().In(loc)
			midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			count, e := tx.Model("bm_growth_leads").Where("plan_id", scope["plan_id"]).Where("created_at>=?", midnight).Count()
			if e != nil {
				return e
			}
			if count >= c.DailyLeads {
				reached = true
				return nil
			}
			old, e := tx.Model("bm_growth_leads").Where("plan_id", scope["plan_id"]).Where("email", email).Value("id")
			if e != nil {
				return e
			}
			if !old.IsEmpty() {
				return nil
			}
			id, e := tx.Model("bm_growth_leads").Data(g.Map{"plan_id": scope["plan_id"], "company_id": scope["company_id"], "dept_id": p.DeptID, "owner_id": p.ID, "email": email, "website": page.URL, "company": found.Company, "country": found.Country, "language": found.Language, "facts": jsonText(facts), "sources": jsonText(sources), "state": state, "assigned_sender": c.Senders[count%len(c.Senders)]}).InsertAndGetId()
			if e != nil {
				return e
			}
			if _, e = tx.Model("bm_growth_candidates").Where("id", candidateID).Data(g.Map{"imported_count": gdb.Raw("imported_count+1")}).Update(); e != nil {
				return e
			}
			if state == "collected" {
				_, e = tx.Model("bm_growth_jobs").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": "verify", "resource_id": fmt.Sprint(id), "dedupe_key": fmt.Sprintf("verify:%d:%d", id, runID), "payload": jsonText(g.Map{"run_id": runID})}).InsertIgnore()
			}
			return e
		})
		if e != nil {
			return e
		}
		if reached {
			_, e = g.DB().Model("bm_growth_candidates").Where("id", candidateID).Data(g.Map{"state": "limited", "error": "Daily lead limit reached; remaining addresses retained in candidate"}).Update()
			return e
		}
	}
	return nil
}
