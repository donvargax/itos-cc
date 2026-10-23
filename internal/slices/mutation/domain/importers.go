package domain

// RestsOnImporters reports whether m's outcome, of a function of the file
// s records, rests on the tests that import the file (Snapshot.Tests), so
// that a change among them makes it stale. Every outcome does but a kill,
// timeout or over-limit (Noticed) decided by listed tests each of which s
// dates: by a recorded non-empty hash of the file that defines it, or by a
// fingerprint. Such a kill rests on those tests and the support files
// instead (ListedChange). A survivor could die to a changed importing
// test, and a listed test with no file and no fingerprint would leave the
// support files its only date, so they rest on the importing tests, and so
// does every outcome of a snapshot that records no tests, nil, as one
// written before snapshots recorded them.
func (s *Snapshot) RestsOnImporters(m Mutant) bool {
	if s == nil || s.Tests == nil || m.Scope != ScopeListed || !Noticed(m.Outcome) || len(m.Tests) == 0 {
		return true
	}
	for _, id := range m.Tests {
		if !s.dates(id) {
			return true
		}
	}
	return false
}

// AnyRestsOnImporters reports whether any of mutants rests on the tests
// that import the file (RestsOnImporters); an entry that records no
// mutant does, as it did before listed kills.
func (s *Snapshot) AnyRestsOnImporters(mutants []Mutant) bool {
	if len(mutants) == 0 {
		return true
	}
	for _, m := range mutants {
		if s.RestsOnImporters(m) {
			return true
		}
	}
	return false
}

// dates reports whether s dates the listed test id apart from the support
// files: it records the test with a fingerprint, or a non-empty hash of
// the file that defines it.
func (s *Snapshot) dates(id string) bool {
	if s.ListedFingerprints[id] != "" {
		return true
	}
	file := s.Listed[id]
	return file != "" && s.ListedFiles[file] != ""
}

// apartFromImporters is s with only the outcomes that do not rest on the
// tests that import its file (RestsOnImporters): what of s still holds
// once they changed. Each entry keeps its hash, place and mark, and s is
// never changed.
func (s *Snapshot) apartFromImporters() *Snapshot {
	out := *s
	out.Units = make([]UnitResult, len(s.Units))
	for i, u := range s.Units {
		u.Mutants = nil
		for _, m := range s.Units[i].Mutants {
			if !s.RestsOnImporters(m) {
				u.Mutants = append(u.Mutants, m)
			}
		}
		out.Units[i] = u
	}
	return &out
}
