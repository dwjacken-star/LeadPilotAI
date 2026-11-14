package growth

import (
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
)

// One logical manager plans within the operator's frozen workflow. It cannot
// create extra workflows, choose unauthorized channels or approve side effects.
func masterExecutionFlow(code string) []string {
	return append([]string{"MASTER"}, masterFlows[code]...)
}

type masterMarketStrategy struct {
	Country       string   `json:"country"`
	Language      string   `json:"language"`
	Focus         string   `json:"focus"`
	BuyerTypes    []string `json:"buyer_types"`
	SearchQueries int      `json:"search_queries"`
	Priority      string   `json:"priority"`
	Reason        string   `json:"reason"`
}
type masterStrategy struct {
	Goal             string                 `json:"goal"`
	Markets          []masterMarketStrategy `json:"markets"`
	Risks            []string               `json:"risks"`
	HumanCheckpoints []string               `json:"human_checkpoints"`
}

func (x *masterExec) plan() (g.Map, error) {
	c := x.Snapshot.Config
	var strategy masterStrategy
	e := x.ai("Act as the single Growth Master planner. Plan only this selected workflow and exactly these country/language targets. Define focused buyer research for each market, priority high/normal/low, and a reason based on the approved company business. Each search_queries must be 1 through the configured queries_per_market. Do not infer market facts without evidence, create extra tasks, approve publication, choose recipients or change budgets. Include human checkpoints for first contact, publication and any contractual claims. Return one strategy per configured market.", g.Map{"workflow": x.Snapshot.Code, "industry": x.Snapshot.Pack, "requirement": c.Requirement, "markets": c.Markets, "queries_per_market": c.QueriesPerMarket, "budget": c.Budget, "approved_company": x.Snapshot.Sources["company"], "registered_skills": masterFlows[x.Snapshot.Code]}, schema(g.Map{"goal": strSchema(), "markets": g.Map{"type": "array", "items": schema(g.Map{"country": strSchema(), "language": strSchema(), "focus": strSchema(), "buyer_types": stringsSchema(), "search_queries": g.Map{"type": "integer"}, "priority": strSchema(), "reason": strSchema()})}, "risks": stringsSchema(), "human_checkpoints": stringsSchema()}), &strategy)
	if e != nil {
		return nil, e
	}
	if len(strategy.Markets) != len(c.Markets) || strategy.Goal == "" {
		return nil, errors.New("Growth Master returned an incomplete market plan")
	}
	seen := map[string]bool{}
	for _, m := range strategy.Markets {
		key := m.Country + ":" + m.Language
		if seen[key] || m.Focus == "" || m.SearchQueries < 1 || m.SearchQueries > c.QueriesPerMarket {
			return nil, errors.New("Growth Master exceeded market allocation")
		}
		allowed := false
		for _, target := range c.Markets {
			if target.Country == m.Country && target.Language == m.Language {
				allowed = true
			}
		}
		if !allowed {
			return nil, errors.New("Growth Master selected an unauthorized market")
		}
		if m.Priority != "high" && m.Priority != "normal" && m.Priority != "low" {
			return nil, errors.New("invalid market priority")
		}
		seen[key] = true
	}
	return g.Map{"goal": strategy.Goal, "markets": strategy.Markets, "risks": strategy.Risks, "human_checkpoints": strategy.HumanCheckpoints, "skills": masterFlows[x.Snapshot.Code], "allocation_rule": fmt.Sprintf("maximum %d searches per market; original budget and approval gates remain enforced", c.QueriesPerMarket)}, nil
}
