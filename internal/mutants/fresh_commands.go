package mutants

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
)

// FreshSelectedPaths orders actual selected files before command observation.
func FreshSelectedPaths(plan *FreshPlan) []string {
	var paths []string
	for _, c := range plan.Selected {
		if !slices.Contains(paths, c.Path) {
			paths = append(paths, c.Path)
		}
	}
	sort.Strings(paths)
	return paths
}

// AddFreshBaseline preserves command deduplication after each real observation.
func AddFreshBaseline(baselines []FreshBaseline, root, rel string, c Command) ([]FreshBaseline, error) {
	spec := lang.Detect(rel)
	if spec == nil {
		return nil, fmt.Errorf("selected file %s has no supported language", rel)
	}
	if c.RunsNothing() && (spec.Name == "python" || spec.Name == "kotlin") {
		return baselines, nil
	}
	if len(c.Args) == 0 {
		return nil, fmt.Errorf("selected file %s has no own-test command", rel)
	}
	local, err := filepath.Rel(root, c.Dir)
	if err != nil || local == ".." || strings.HasPrefix(local, ".."+string(filepath.Separator)) || filepath.IsAbs(local) {
		return nil, fmt.Errorf("own-test command of %s leaves the frozen root: export path escapes private tree: %q", rel, c.Dir)
	}
	for i := range baselines {
		if baselines[i].Command.Key() == c.Key() {
			baselines[i].Paths = append(baselines[i].Paths, rel)
			return baselines, nil
		}
	}
	return append(baselines, FreshBaseline{Command: c, Paths: []string{rel}}), nil
}
