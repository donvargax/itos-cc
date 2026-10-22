package domain

import "sort"

// ListedChange is how the tests and files a snapshot's listed outcomes rest
// on differ from those it recorded.
type ListedChange struct {
	listed  map[string]string // the file of each test, by ID
	printed map[string]bool   // the tests recorded with a fingerprint, dated by it
	tests   map[string]bool   // the files changed of the tests recorded without one
	changed map[string]bool   // the tests recorded with a fingerprint the list no longer prints them with
	support []string          // the support files changed, added, or removed, sorted
}

// ListedChanged is how the files the listed outcomes of s rest on differ
// from now, the hash of each file s.Listed names now, and support, the
// support files' hashes now, judged without the list: a test recorded with
// a fingerprint whose file changed counts as changed (ListedChangedListing
// judges it by the list).
func (s *Snapshot) ListedChanged(now, support map[string]string) *ListedChange {
	return s.listedChanged(now, support, nil)
}

// listedChanged is ListedChanged, the tests recorded with a fingerprint
// whose file changed judged by listing, the list command's output, when it
// is not nil: such a test changed when the list no longer prints it with
// that fingerprint, whatever file it names. A test whose file is unchanged
// has an unchanged fingerprint.
func (s *Snapshot) listedChanged(now, support map[string]string, listing *ListedInventory) *ListedChange {
	if s == nil || !HasListed(s.Units) {
		return nil
	}
	c := &ListedChange{listed: s.Listed, printed: map[string]bool{}, tests: map[string]bool{}, changed: map[string]bool{}}
	for id, file := range s.Listed {
		fingerprint := s.ListedFingerprints[id]
		if fingerprint == "" {
			if file != "" && now[file] != s.ListedFiles[file] {
				c.tests[file] = true
			}
			continue
		}
		c.printed[id] = true
		if s.fileHolds(id, now) {
			continue
		}
		if listing == nil || listing.fingerprint(id) != fingerprint {
			c.changed[id] = true
		}
	}
	d := CompareTests(s.Support, support)
	c.support = append(append(append(c.support, d.Changed...), d.Added...), d.Removed...)
	sort.Strings(c.support)
	if len(c.tests) == 0 && len(c.changed) == 0 && len(c.support) == 0 {
		return nil
	}
	return c
}

// fileHolds is whether the file s records the test id in is as s recorded
// it, now the hash of each file s.Listed names now; never for a test listed
// with no file.
func (s *Snapshot) fileHolds(id string, now map[string]string) bool {
	file := s.Listed[id]
	return file != "" && now[file] == s.ListedFiles[file]
}

// needsListing is whether the freshness of a listed outcome of s needs the
// list command: a test recorded with a fingerprint whose file changed, now
// the hash of each file s.Listed names now.
func (s *Snapshot) needsListing(now map[string]string) bool {
	for id := range s.Listed {
		if s.ListedFingerprints[id] != "" && !s.fileHolds(id, now) {
			return true
		}
	}
	return false
}

// Files is the files that changed among those the listed outcomes of
// mutants rest on, sorted: the file of a test recorded without a
// fingerprint, and the support files; nil when none did.
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
			if f := c.listed[id]; !c.printed[id] && c.tests[f] {
				set[f] = true
			}
		}
	}
	if listed {
		for _, f := range c.support {
			set[f] = true
		}
	}
	return sortedSet(set)
}

// Tests is the tests recorded with a fingerprint that the listed outcomes
// of mutants rest on and that the list no longer prints with it, sorted;
// nil when none.
func (c *ListedChange) Tests(mutants []Mutant) []string {
	if c == nil {
		return nil
	}
	set := map[string]bool{}
	for _, m := range mutants {
		if m.Scope != ScopeListed {
			continue
		}
		for _, id := range m.Tests {
			if c.changed[id] {
				set[id] = true
			}
		}
	}
	return sortedSet(set)
}

// Changed is whether a test or file a listed outcome of mutants rests on
// changed (Files, Tests).
func (c *ListedChange) Changed(mutants []Mutant) bool {
	return len(c.Files(mutants)) > 0 || len(c.Tests(mutants)) > 0
}

// sortedSet is the members of set, sorted; nil when it has none.
func sortedSet(set map[string]bool) []string {
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
