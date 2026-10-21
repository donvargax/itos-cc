package mutants

import "sort"

// ListedChange is how the files a snapshot's listed outcomes rest on differ
// from those it recorded.
type ListedChange struct {
	listed  map[string]string // the file of each test, by ID
	tests   map[string]bool   // the tests' files changed
	support []string          // the support files changed, added, or removed, sorted
}

func (s *Snapshot) ListedChanged(now, support map[string]string) *ListedChange {
	if s == nil || !HasListed(s.Units) {
		return nil
	}
	c := &ListedChange{listed: s.Listed, tests: map[string]bool{}}
	for _, file := range s.Listed {
		if file != "" && now[file] != s.ListedFiles[file] {
			c.tests[file] = true
		}
	}
	d := CompareTests(s.Support, support)
	c.support = append(append(append(c.support, d.Changed...), d.Added...), d.Removed...)
	sort.Strings(c.support)
	if len(c.tests) == 0 && len(c.support) == 0 {
		return nil
	}
	return c
}

// files is the files that changed among those the listed outcomes of
// mutants rest on, sorted; nil when none did.
func (c *ListedChange) Files(mutants []Mutant) []string {
	if c == nil {
		return nil
	}
	set := map[string]bool{}
	listed := false
	for _, m := range mutants {
		if m.Scope != ScopeListed {
			continue
		}
		listed = true
		for _, id := range m.Tests {
			if f := c.listed[id]; c.tests[f] {
				set[f] = true
			}
		}
	}
	if listed {
		for _, f := range c.support {
			set[f] = true
		}
	}
	var out []string
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// HasListed reports whether units hold an outcome of ScopeListed.
func HasListed(units []UnitResult) bool {
	for _, u := range units {
		for _, m := range u.Mutants {
			if m.Scope == ScopeListed {
				return true
			}
		}
	}
	return false
}
