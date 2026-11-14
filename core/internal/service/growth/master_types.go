package growth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

type MasterField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Unit     string   `json:"unit"`
	Options  []string `json:"options"`
}
type MasterPack struct {
	Description     string         `json:"description"`
	ObjectType      string         `json:"object_type"`
	Terms           []string       `json:"terms"`
	BuyerTypes      []string       `json:"buyer_types"`
	IntentTerms     []string       `json:"intent_terms"`
	ExcludeKeywords []string       `json:"exclude_keywords"`
	SourceDomains   []string       `json:"source_domains"`
	Fields          []MasterField  `json:"fields"`
	ScoreWeights    map[string]int `json:"score_weights"`
	MinScore        int            `json:"min_score"`
	MinConfidence   float64        `json:"min_confidence"`
	Tone            string         `json:"tone"`
	ForbiddenClaims []string       `json:"forbidden_claims"`
}
type MasterSkill struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Group      string   `json:"group"`
	Input      []string `json:"input"`
	Output     []string `json:"output"`
	Tools      []string `json:"tools"`
	Gate       string   `json:"gate"`
	Permission string   `json:"permission"`
}

var MasterSkills = []MasterSkill{
	{"S01", "Global News Monitor", "intelligence", []string{"industry", "markets", "sources"}, []string{"evidence_ids", "coverage"}, []string{"serper.search", "serper.scrape"}, "public_source_required", "intelligence:run"},
	{"S02", "Market Trend Analyst", "intelligence", []string{"evidence", "analytics"}, []string{"findings", "citations", "uncertainties"}, []string{"ai.structured"}, "citations_must_exist", "intelligence:run"},
	{"S03", "Product Hotspot Detector", "intelligence", []string{"evidence", "industry"}, []string{"hotspots", "sample_scope"}, []string{"ai.structured"}, "observations_not_sales_statistics", "intelligence:run"},
	{"S04", "Purchase Intent Detector", "intelligence", []string{"industry", "markets"}, []string{"signals", "evidence_ids"}, []string{"serper.search", "ai.structured"}, "dated_evidence_required", "intelligence:run"},
	{"S05", "B2B Company Finder", "prospecting", []string{"industry", "markets", "signals"}, []string{"prospect_ids"}, []string{"serper.search"}, "public_website_and_deduplication", "intelligence:run"},
	{"S06", "Contact Intelligence", "prospecting", []string{"prospect_ids"}, []string{"public_contacts", "evidence_ids"}, []string{"serper.scrape", "ai.structured"}, "contact_must_appear_in_source", "intelligence:run"},
	{"S07", "Data Cleaner", "prospecting", []string{"public_contacts", "filters"}, []string{"verified", "invalid", "unconfirmed", "filtered"}, []string{"email.dns_mx_smtp_catchall", "suppression.check"}, "only_verified_enters_outreach", "intelligence:run"},
	{"S08", "Lead Scoring", "prospecting", []string{"prospect_facts", "weights"}, []string{"score", "breakdown", "rule_version"}, []string{"rules.score"}, "missing_facts_score_zero", "intelligence:run"},
	{"S09", "Product Matching", "marketing", []string{"needs", "approved_offerings"}, []string{"matches", "evidence", "missing_conditions"}, []string{"ai.structured", "product.catalog"}, "approved_ids_only", "intelligence:run"},
	{"S10", "AI Content Creator", "marketing", []string{"approved_facts", "market", "channel"}, []string{"content", "source_snapshot"}, []string{"ai.structured"}, "human_review_before_first_contact_or_publish", "intelligence:run"},
	{"S11", "Campaign Orchestrator", "marketing", []string{"approved_content", "channel", "permission"}, []string{"task_ids", "receipts"}, []string{"maplemail.queue", "social.queue", "whatsapp.template"}, "approval_permission_suppression_and_budget", "intelligence:publish"},
	{"S12", "Reply Intelligence", "conversion", []string{"inbound_events", "approved_facts"}, []string{"classification", "draft", "followup"}, []string{"inbox.classify", "ai.structured", "crm.followup"}, "no_automated_contractual_commitments", "intelligence:run"},
	{"S13", "Growth Analytics", "conversion", []string{"deliveries", "inquiries", "costs"}, []string{"metrics", "availability", "recommendations"}, []string{"db.aggregate"}, "unknown_metrics_remain_unknown", "intelligence:run"},
	{"S14", "Data Sync & Audit", "conversion", []string{"workflow_results"}, []string{"crm_links", "audit", "summary"}, []string{"crm.upsert", "audit.append"}, "scoped_and_idempotent", "intelligence:run"},
}

type MasterSkillConfig struct {
	Removed        bool   `json:"removed,omitempty"`
	Enabled        bool   `json:"enabled"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Retries        int    `json:"retries"`
	MaxItems       int    `json:"max_items"`
	MaxOutput      int    `json:"max_output"`
}
type MasterBudget struct {
	Calls                    int   `json:"calls"`
	Tokens                   int   `json:"tokens"`
	Searches                 int   `json:"searches"`
	Scrapes                  int   `json:"scrapes"`
	Sends                    int   `json:"sends"`
	CostMicros               int64 `json:"cost_micros"`
	AIInputMicrosPerMillion  int64 `json:"ai_input_micros_per_million"`
	AIOutputMicrosPerMillion int64 `json:"ai_output_micros_per_million"`
	SearchMicros             int64 `json:"search_micros"`
	ScrapeMicros             int64 `json:"scrape_micros"`
}
type MasterConfig struct {
	Model            string            `json:"model"`
	Markets          []Market          `json:"markets"`
	Requirement      string            `json:"requirement"`
	ProductIDs       []int64           `json:"product_ids"`
	ProspectIDs      []int64           `json:"prospect_ids"`
	Channels         []string          `json:"channels"`
	SocialAccounts   []int64           `json:"social_accounts"`
	ImageAssetIDs    []string          `json:"image_asset_ids"`
	Senders          []string          `json:"senders"`
	Cadence          string            `json:"cadence"`
	Timezone         string            `json:"timezone"`
	LocalTime        string            `json:"local_time"`
	ExcludeKeywords  []string          `json:"exclude_keywords"`
	MaxItems         int               `json:"max_items"`
	QueriesPerMarket int               `json:"queries_per_market"`
	CooldownDays     int               `json:"cooldown_days"`
	TrackOpen        bool              `json:"track_open"`
	TrackClick       bool              `json:"track_click"`
	Unsubscribe      bool              `json:"unsubscribe"`
	Warmup           bool              `json:"warmup"`
	CCEmail          string            `json:"cc_email"`
	Policy           governance.Policy `json:"policy"`
	Budget           MasterBudget      `json:"budget"`
	InboxRuleIDs     []int64           `json:"inbox_rule_ids"`
	AutoReply        bool              `json:"auto_reply"`
	ReplyRules       []MasterReplyRule `json:"reply_rules"`
	ReplyApprovedBy  int64             `json:"reply_approved_by"`
}
type MasterSnapshot struct {
	Code            string                       `json:"code"`
	Name            string                       `json:"name"`
	WorkflowVersion int                          `json:"workflow_version"`
	PackID          int64                        `json:"pack_id"`
	PackVersion     int                          `json:"pack_version"`
	Pack            MasterPack                   `json:"pack"`
	Config          MasterConfig                 `json:"config"`
	Skills          map[string]MasterSkillConfig `json:"skills"`
	Sources         g.Map                        `json:"sources"`
	EventID         int64                        `json:"event_id,omitempty"`
}

var masterFlows = map[string][]string{
	"WF01": {"S01", "S02", "S03", "S14"}, "WF02": {"S04", "S05", "S06", "S07", "S08", "S09", "S14"},
	"WF03": {"S05", "S06", "S07", "S08", "S09", "S14"}, "WF04": {"S10", "S11", "S14"},
	"WF05": {"S12", "S08", "S09", "S14"}, "WF06": {"S13", "S02", "S14"},
}

func masterDefaultSkill() MasterSkillConfig {
	return MasterSkillConfig{Enabled: true, TimeoutSeconds: 240, Retries: 2, MaxItems: 20, MaxOutput: 2400}
}
func MasterDefaultConfig(code string) MasterConfig {
	at, cadence := "09:00:00", "daily"
	switch code {
	case "WF01":
		at = "06:00:00"
	case "WF02":
		cadence = "6h"
		at = "00:00:00"
	case "WF05":
		cadence = "event"
	case "WF06":
		at = "20:00:00"
	}
	return MasterConfig{Markets: []Market{{Country: "US", Language: "en", Timezone: "America/New_York"}}, Cadence: cadence, Timezone: "Asia/Shanghai", LocalTime: at, MaxItems: 10, QueriesPerMarket: 1, CooldownDays: 30, Unsubscribe: true, TrackOpen: true, TrackClick: true, Policy: governance.Policy{MinIntervalMS: 60000, Concurrency: 1, PerHour: 50, RecipientCooldownHours: 24, WindowStart: "09:00", WindowEnd: "17:00"}, Budget: MasterBudget{Calls: 50, Tokens: 150000, Searches: 20, Scrapes: 40, Sends: 20}}
}
func masterHash(v interface{}) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

var masterKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (p MasterPack) validate() error {
	if p.ObjectType != "product" && p.ObjectType != "service" && p.ObjectType != "both" {
		return errors.New("choose product, service or both")
	}
	if len(p.Terms) == 0 || len(p.Terms) > 100 || len(p.Fields) > 100 || p.MinScore < 0 || p.MinScore > 100 || p.MinConfidence < 0 || p.MinConfidence > 1 {
		return errors.New("invalid industry terms, fields or quality thresholds")
	}
	seen := map[string]bool{}
	for _, f := range p.Fields {
		if !masterKey.MatchString(f.Key) || seen[f.Key] || strings.TrimSpace(f.Label) == "" {
			return errors.New("unique field keys and labels required")
		}
		seen[f.Key] = true
		switch f.Type {
		case "text", "number", "boolean", "enum", "list":
		default:
			return errors.New("invalid industry field type")
		}
		if f.Type == "enum" && len(f.Options) == 0 {
			return errors.New("enumeration choices required")
		}
	}
	sum := 0
	for key, n := range p.ScoreWeights {
		if !strings.Contains("|relevance|market|contact|intent|completeness|", "|"+key+"|") || n < 0 || n > 100 {
			return errors.New("invalid scoring indicator")
		}
		sum += n
	}
	if sum != 100 {
		return errors.New("score weights must total 100")
	}
	for _, d := range p.SourceDomains {
		if strings.ContainsAny(d, "/:@ ") || !strings.Contains(d, ".") {
			return errors.New("use bare source domains")
		}
	}
	for _, items := range [][]string{p.Terms, p.BuyerTypes, p.IntentTerms, p.ExcludeKeywords, p.ForbiddenClaims} {
		if len(items) > 100 {
			return errors.New("at most 100 terms per list")
		}
		for _, s := range items {
			if strings.TrimSpace(s) == "" || len(s) > 500 {
				return errors.New("invalid industry term")
			}
		}
	}
	return nil
}
func masterFieldErrors(pack MasterPack, values g.Map) []string {
	out := []string{}
	for _, f := range pack.Fields {
		v, ok := values[f.Key]
		if !ok || v == nil || fmt.Sprint(v) == "" {
			if f.Required {
				out = append(out, f.Key+": required")
			}
			continue
		}
		valid := true
		switch f.Type {
		case "number":
			switch v.(type) {
			case float64, float32, int, int64, json.Number:
			default:
				valid = false
			}
		case "boolean":
			_, valid = v.(bool)
		case "list":
			switch v.(type) {
			case []interface{}, []string:
			default:
				valid = false
			}
		case "enum":
			valid = false
			for _, s := range f.Options {
				if fmt.Sprint(v) == s {
					valid = true
				}
			}
		case "text":
			_, valid = v.(string)
		}
		if !valid {
			out = append(out, f.Key+": invalid "+f.Type)
		}
	}
	return out
}
func masterNext(c MasterConfig, now time.Time) (time.Time, error) {
	loc, e := time.LoadLocation(c.Timezone)
	if e != nil {
		return now, e
	}
	clock, e := time.Parse("15:04:05", c.LocalTime)
	if e != nil {
		clock, e = time.Parse("15:04", c.LocalTime)
	}
	if e != nil {
		return now, e
	}
	local := now.In(loc)
	at := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, loc)
	switch c.Cadence {
	case "event", "manual":
		return now.AddDate(100, 0, 0), nil
	case "6h":
		for !at.After(now) {
			at = at.Add(6 * time.Hour)
		}
	case "daily":
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
	case "weekly":
		if !at.After(now) {
			at = at.AddDate(0, 0, 7)
		}
	default:
		return now, errors.New("invalid cadence")
	}
	return at, nil
}
func masterExcluded(value string, words []string) bool {
	value = strings.ToLower(value)
	for _, s := range words {
		if strings.TrimSpace(s) != "" && strings.Contains(value, strings.ToLower(strings.TrimSpace(s))) {
			return true
		}
	}
	return false
}

type masterWait struct {
	State, Reason string
	Until         time.Time
}

func (e *masterWait) Error() string { return e.Reason }
