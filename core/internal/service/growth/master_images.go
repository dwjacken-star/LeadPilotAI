package growth

import (
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
)

func masterImageFacts(ctx context.Context, p *governance.Principal, cid int64, ids []string) ([]g.Map, error) {
	if len(ids) > 10 {
		return nil, errors.New("select at most 10 publishing images")
	}
	out := []g.Map{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, errors.New("duplicate publishing image")
		}
		seen[id] = true
		r, e := Asset(ctx, p, id, "material:list")
		if e != nil {
			return nil, e
		}
		if r["company_id"].Int64() != cid || !r["published"].Bool() || r["visibility"].String() != "public" || !r["ai_allowed"].Bool() || (r["mime"].String() != "image/jpeg" && r["mime"].String() != "image/png") {
			return nil, errors.New("select published AI-enabled JPG/PNG company images")
		}
		out = append(out, g.Map{"id": id, "version": r["version"].Int(), "sha256": r["sha256"].String(), "filename": r["filename"].String()})
	}
	return out, nil
}
