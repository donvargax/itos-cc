package domain

// markBroadStale marks Stale each entry kept for a function the run did not
// judge whose recorded whole-suite evidence no longer holds. KeepUnjudged
// gives a function the snapshot does not record no entry, so the snapshot's
// units are not the file's functions place for place (issue #40): each kept
// entry is found by the function it belongs to, its namespace#name and the
// hash it carries (a kept entry carries its recorded one, a judged function
// its current one), taken in file order among those sharing both, as
// KeepUnjudged keeps them.
func (s *Execution) markBroadStale() {
	if s.judged == nil || s.stored == nil {
		return
	}
	keys := KeysOfFile(s.file)
	results := PairResults(keys, s.stored.Units)
	// The key each function's result carries in the snapshot written.
	written, marks := keyer{}, map[UnitKey]bool{}
	for i, key := range keys {
		switch recorded := results.For(key); {
		case s.judged[i]:
			marks[written.next(key.ID(), key.Hash())] = false
		case recorded != nil:
			marks[written.next(key.ID(), recorded.Hash)] = s.broadChanged(*recorded)
		}
	}
	units := s.result.Snapshot.Units
	for i, key := range KeysOfEntries(units) {
		units[i].Stale = units[i].Stale || marks[key]
	}
}

// broadChanged says whether a whole-suite input one of the entry's mutants
// was decided under changed since.
func (s *Execution) broadChanged(entry UnitResult) bool {
	for _, m := range entry.Mutants {
		if len(BroadChanges(m, s.suite.Of(m.TestScope()), s.currentSupport)) > 0 {
			return true
		}
	}
	return false
}
