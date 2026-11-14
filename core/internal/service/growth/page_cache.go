package growth

import (
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
)

// Evidence is frozen per candidate/run, so retrying AI qualification does not
// re-purchase successful page scrapes. Failed pages remain retryable.
func candidatePage(ctx context.Context, p *governance.Principal, candidateID int64, url string) (WebPage, error) {
	var page WebPage
	value, e := g.DB().Model("bm_growth_pages").Ctx(ctx).Where("candidate_id", candidateID).Where("url", url).Value("page")
	if e != nil {
		return page, e
	}
	if !value.IsEmpty() {
		e = json.Unmarshal(value.Bytes(), &page)
		return page, e
	}
	page, e = CollectWebsite(ctx, p, url)
	if e != nil {
		return page, e
	}
	_, e = g.DB().Model("bm_growth_pages").Ctx(ctx).Data(g.Map{"candidate_id": candidateID, "url": url, "page": jsonText(page)}).InsertIgnore()
	return page, e
}
