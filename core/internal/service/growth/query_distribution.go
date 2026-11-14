package growth

import (
	"errors"
	"fmt"
)

func validateQueryDistribution(markets []Market, pairs [][2]string, limit int) error {
	requested := map[string]bool{}
	for _, m := range markets {
		requested[m.Country+":"+m.Language] = true
	}
	if len(requested) == 0 {
		return errors.New("no target markets")
	}
	counts := map[string]int{}
	for _, pair := range pairs {
		counts[pair[0]+":"+pair[1]]++
	}
	if limit >= len(requested) {
		max := (limit + len(requested) - 1) / len(requested)
		for key := range requested {
			if counts[key] == 0 || counts[key] > max {
				return fmt.Errorf("AI search queries must cover each target market evenly")
			}
		}
	}
	return nil
}
