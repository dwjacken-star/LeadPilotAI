package growth

import (
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/xuri/excelize/v2"
)

func masterExportProspects(ctx context.Context, p *governance.Principal, cid int64, state string) ([]byte, error) {
	if cid < 1 {
		return nil, errors.New("select an owning company")
	}
	if _, e := company(ctx, p, cid, "intelligence:export", false); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, "intelligence:export", "")
	if e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_gm_prospects").Ctx(ctx).Where("company_id", cid).Where(scope).WhereNot("email", "")
	if state != "" {
		q = q.Where("state", state)
	}
	rows, e := q.Order("id").Limit(10000).All()
	if e != nil {
		return nil, e
	}
	book := excelize.NewFile()
	defer book.Close()
	head := []interface{}{"Company", "Email", "Website", "Country", "Language", "Phone", "State", "Verification reason", "Score", "Contact permission", "Permission evidence"}
	if e = book.SetSheetRow("Sheet1", "A1", &head); e != nil {
		return nil, e
	}
	style, e := book.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		return nil, e
	}
	_ = book.SetColStyle("Sheet1", "A:K", style)
	for i, r := range rows {
		row := []interface{}{r["company"].String(), r["email"].String(), r["website"].String(), r["country"].String(), r["language"].String(), r["phone"].String(), r["state"].String(), jsonMap(r["verification"])["code"], r["score"].Float64(), r["contact_permission"].String(), r["permission_evidence"].String()}
		if e = book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &row); e != nil {
			return nil, e
		}
	}
	buffer, e := book.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return buffer.Bytes(), nil
}
