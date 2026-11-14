package growth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type SerperConfig struct {
	Enabled       bool   `json:"enabled"`
	APIKey        string `json:"api_key,omitempty"`
	DailySearches int    `json:"daily_searches"`
	DailyScrapes  int    `json:"daily_scrapes"`
	DailyCredits  int    `json:"daily_credits"`
}

var serperHTTP = &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
var serperSearchURL = "https://google.serper.dev/"
var serperScrapeURL = "https://scrape.serper.dev/"

func serperConfig(ctx context.Context) (SerperConfig, error) {
	c := SerperConfig{DailySearches: 100, DailyScrapes: 300, DailyCredits: 5000}
	v, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "growth_serper_config").Value("value")
	if e == nil && !v.IsEmpty() {
		e = json.Unmarshal(v.Bytes(), &c)
	}
	return c, e
}
func SaveSerperSettings(ctx context.Context, p *governance.Principal, c SerperConfig) error {
	if p == nil || !p.Super {
		return errors.New("administrator required")
	}
	if c.DailySearches < 1 || c.DailySearches > 100000 || c.DailyScrapes < 1 || c.DailyScrapes > 100000 || c.DailyCredits < 40 || c.DailyCredits > 10000000 {
		return errors.New("invalid Serper usage limits")
	}
	old, e := serperConfig(ctx)
	if e != nil {
		return e
	}
	c.APIKey = strings.TrimSpace(c.APIKey)
	if c.APIKey == "" {
		c.APIKey = old.APIKey
	} else {
		if len(c.APIKey) > 500 || strings.ContainsAny(c.APIKey, "\r\n") {
			return errors.New("invalid Serper API key")
		}
		c.APIKey, e = protect(c.APIKey, false)
		if e != nil {
			return e
		}
	}
	if c.Enabled && c.APIKey == "" {
		return errors.New("Serper configuration required: enter an API key")
	}
	_, e = g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": "growth_serper_config", "value": jsonText(c)}).OnConflict("name").Save()
	return e
}
func SerperSettings(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	c, e := serperConfig(ctx)
	if e != nil {
		return nil, e
	}
	has := c.APIKey != ""
	c.APIKey = ""
	usage, e := g.DB().GetOne(ctx, `SELECT count(*) FILTER(WHERE kind='search') searches,count(*) FILTER(WHERE kind='scrape') scrapes,coalesce(sum(credits),0) credits FROM bm_growth_serper_usage WHERE created_at>=date_trunc('day',now())`)
	return g.Map{"config": c, "has_key": has, "usage": usage}, e
}
func serperReady(ctx context.Context) error {
	c, e := serperConfig(ctx)
	if e != nil {
		return e
	}
	if !c.Enabled || c.APIKey == "" {
		return errors.New("Serper configuration required: enable search and save an API key")
	}
	return nil
}

// Reservations serialize daily quota claims across workers. An interrupted request
// retains its conservative credit reservation instead of assuming it was free.
func reserveSerper(ctx context.Context, p *governance.Principal, c SerperConfig, kind string) (int64, error) {
	if _, master := ctx.Value(masterTraceKey{}).(masterTrace); !master || !p.Has("intelligence:run") {
		if e := require(p, "outreach:run"); e != nil {
			return 0, e
		}
	}
	reserve := 1
	if kind == "scrape" {
		reserve = 40
	}
	var id int64
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614819)"); e != nil {
			return e
		}
		r, e := tx.GetOne(`SELECT count(*) FILTER(WHERE kind='search') searches,count(*) FILTER(WHERE kind='scrape') scrapes,coalesce(sum(credits),0) credits FROM bm_growth_serper_usage WHERE created_at>=date_trunc('day',now())`)
		if e != nil {
			return e
		}
		if r["credits"].Int()+reserve > c.DailyCredits || kind == "search" && r["searches"].Int() >= c.DailySearches || kind == "scrape" && r["scrapes"].Int() >= c.DailyScrapes {
			return errors.New("Serper daily budget exhausted; configuration required")
		}
		id, e = tx.Model("bm_growth_serper_usage").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": kind, "credits": reserve}).InsertAndGetId()
		return e
	})
	return id, e
}
func callSerper(ctx context.Context, p *governance.Principal, kind, endpoint string, input g.Map, out interface{}) error {
	c, e := serperConfig(ctx)
	if e != nil {
		return e
	}
	if e = serperReady(ctx); e != nil {
		return e
	}
	key, e := protect(c.APIKey, true)
	if e != nil {
		return errors.New("Serper credential unavailable; configuration required")
	}
	body, e := json.Marshal(input)
	if e != nil {
		return e
	}
	id, e := reserveSerper(ctx, p, c, kind)
	if e != nil {
		return e
	}
	status := "failed"
	defer func() {
		_, _ = g.DB().Model("bm_growth_serper_usage").Ctx(context.Background()).Where("id", id).Data(g.Map{"status": status}).Update()
	}()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if e != nil {
		return errors.New("invalid Serper request")
	}
	req.Header.Set("X-API-KEY", key)
	req.Header.Set("Content-Type", "application/json")
	res, e := serperHTTP.Do(req)
	if e != nil {
		return errors.New("Serper connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		switch res.StatusCode {
		case 401, 403:
			return errors.New("Serper authentication or credits unavailable; configuration required")
		case 402:
			return errors.New("Serper credits exhausted; configuration required")
		case 429:
			return errors.New("Serper rate limit reached; retry later")
		}
		return fmt.Errorf("Serper request failed (HTTP %d)", res.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if e != nil || len(raw) > 4*1024*1024 {
		return errors.New("Serper response exceeds allowed size")
	}
	var envelope struct {
		Credits *int            `json:"credits"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Error) > 0 && string(envelope.Error) != "null" && string(envelope.Error) != `""` {
		return errors.New("Serper returned an invalid or failed result")
	}
	if envelope.Credits != nil && *envelope.Credits >= 0 {
		_, e = g.DB().Model("bm_growth_serper_usage").Ctx(ctx).Where("id", id).Data(g.Map{"credits": *envelope.Credits}).Update()
		if e != nil {
			return e
		}
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return errors.New("Serper returned invalid structured data")
	}
	status = "completed"
	return nil
}

var SearchBusinesses = searchSerper

func searchSerper(ctx context.Context, p *governance.Principal, query string, market Market, limit int, channel string) ([]SearchSource, error) {
	sources := []SearchSource{}
	endpoints := []string{"search"}
	if channel == "places" {
		endpoints = []string{"places"}
	} else if channel == "both" {
		endpoints = []string{"search", "places"}
	}
	for _, endpoint := range endpoints {
		input := g.Map{"q": query, "gl": strings.ToLower(market.Country), "hl": market.Language, "num": limit}
		if market.City != "" {
			input["location"] = market.City + ", " + market.Country
		}
		var result struct {
			Organic *[]struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
			} `json:"organic"`
			Places *[]struct {
				Title   string `json:"title"`
				Website string `json:"website"`
				Address string `json:"address"`
				Phone   string `json:"phoneNumber"`
				CID     string `json:"cid"`
				PlaceID string `json:"placeId"`
			} `json:"places"`
		}
		if e := callSerper(ctx, p, "search", serperSearchURL+endpoint, input, &result); e != nil {
			return nil, e
		}
		if endpoint == "search" {
			if result.Organic == nil {
				return nil, errors.New("Serper search response has no organic results field")
			}
			for _, r := range *result.Organic {
				sources = append(sources, SearchSource{URL: r.Link, SourceURL: r.Link, Title: r.Title, Description: r.Snippet, Kind: endpoint})
			}
		} else {
			if result.Places == nil {
				return nil, errors.New("Serper places response has no places results field")
			}
			for _, r := range *result.Places {
				ref := r.Website
				if r.CID != "" {
					ref = "https://www.google.com/maps?cid=" + url.QueryEscape(r.CID)
				}
				sources = append(sources, SearchSource{URL: r.Website, SourceURL: ref, Title: r.Title, Address: r.Address, Phone: r.Phone, PlaceID: r.CID + r.PlaceID, Kind: endpoint})
			}
		}
	}
	// Keep results from both channels when requested; the total limit is applied
	// per query/channel, rather than discarding all Places after organic results.
	if len(sources) > limit*len(endpoints) {
		sources = sources[:limit*len(endpoints)]
	}
	return sources, nil
}

var CollectWebsite = scrapeSerper

func scrapeSerper(ctx context.Context, p *governance.Principal, value string) (WebPage, error) {
	actual, e := safeURL(value)
	if e != nil {
		return WebPage{}, e
	}
	var result struct {
		Text     string `json:"text"`
		Markdown string `json:"markdown"`
		Metadata struct {
			Title      string `json:"title"`
			StatusCode int    `json:"statusCode"`
		} `json:"metadata"`
	}
	if e = callSerper(ctx, p, "scrape", serperScrapeURL, g.Map{"url": actual.String(), "includeMarkdown": true}, &result); e != nil {
		return WebPage{}, e
	}
	if result.Metadata.StatusCode != 0 && (result.Metadata.StatusCode < 200 || result.Metadata.StatusCode >= 400) {
		return WebPage{}, errors.New("Serper target website denied or unavailable")
	}
	if strings.TrimSpace(result.Text) == "" && strings.TrimSpace(result.Markdown) == "" {
		return WebPage{}, errors.New("Serper returned an empty website")
	}
	return parseSerperPage(actual, result.Text, result.Markdown), nil
}

func TestSerper(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	rows, e := searchSerper(ctx, p, "automotive air conditioning parts distributor Manila Philippines", Market{Country: "PH", Language: "en"}, 1, "search")
	if e != nil {
		return nil, e
	}
	page, e := scrapeSerper(ctx, p, "https://example.com")
	if e != nil {
		return nil, e
	}
	return g.Map{"search_ok": true, "scrape_ok": true, "search_results": len(rows), "scrape_characters": len(page.Text)}, nil
}
