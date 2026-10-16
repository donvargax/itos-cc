package mutants

import (
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/lang"
)

// FreshMeasurementSources admits Go inventory and languages with eligible sites,
// independently of which globally ranked sites the count selected.
func FreshMeasurementSources(plan *FreshPlan) []string {
	judged := map[string]bool{"go": true}
	for _, c := range plan.Eligible {
		if spec := lang.Detect(c.Path); spec != nil {
			judged[spec.Name] = true
		}
	}
	var sources []string
	seen := map[string]bool{}
	for _, unit := range plan.Units {
		if spec := lang.Detect(unit.Path); spec == nil || !judged[spec.Name] {
			continue
		}
		p := filepath.Join(plan.FrozenRoot, filepath.FromSlash(unit.Path))
		if !seen[p] {
			seen[p] = true
			sources = append(sources, p)
		}
	}
	return sources
}
