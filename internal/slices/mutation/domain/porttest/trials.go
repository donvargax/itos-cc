package porttest

import (
	"path/filepath"
	"slices"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// trial is tests as they decide this run of change: a Verdict cycling
// through Outcomes takes its next one, counted per site. f.mu is held.
func (f *Workers) trial(c mutants.Command, tests Tests, change *mutants.Mutation) Tests {
	if change == nil {
		return tests
	}
	verdicts := slices.Clone(tests.Verdicts)
	for i, v := range verdicts {
		if len(v.Outcomes) == 0 || !v.decides(c, change) {
			continue
		}
		if f.trials == nil {
			f.trials = map[string]int{}
		}
		key := filepath.Clean(change.Path) + "\x00" + change.Site.Key()
		verdicts[i].Outcome = v.Outcomes[f.trials[key]%len(v.Outcomes)]
		f.trials[key]++
	}
	tests.Verdicts = verdicts
	return tests
}

// decides says whether v is the verdict of change, a mutant of a file
// under c's root.
func (v Verdict) decides(c mutants.Command, change *mutants.Mutation) bool {
	rel, err := filepath.Rel(c.Root, change.Path)
	return err == nil && filepath.ToSlash(rel) == filepath.ToSlash(v.Path) && v.Site.Line == change.Site.Line &&
		v.Site.Original == change.Site.Original && v.Site.Replacement == change.Site.Replacement
}
