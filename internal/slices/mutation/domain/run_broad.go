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
	marks := map[UnitKey]bool{}
	for _, w := range s.written() {
		marks[w.key] = w.kept != nil && (s.broadChanged(*w.kept) || s.listedChange != nil && s.listedChange.Changed(w.kept.Mutants))
	}
	units := s.result.Snapshot.Units
	for i, key := range KeysOfEntries(units) {
		units[i].Stale = units[i].Stale || marks[key]
	}
}

// A writtenResult is the key a function's result carries in the snapshot
// this run writes, and, for a function kept, the entry it was kept from.
type writtenResult struct {
	key  UnitKey
	kept *UnitResult
}

// written is, by the index of each function of the file that has a result in
// the snapshot this run writes, that result: a judged function's has its
// current hash, a kept one's the hash it was recorded with, in file order
// among those sharing both. A function not judged that the snapshot never
// recorded has none.
func (s *Execution) written() map[int]writtenResult {
	keys := KeysOfFile(s.file)
	var entries []UnitResult
	if s.stored != nil {
		entries = s.stored.Units
	}
	results := PairResults(keys, entries)
	counted, out := keyer{}, map[int]writtenResult{}
	for i, key := range keys {
		switch recorded := results.For(key); {
		case s.judged == nil || s.judged[i]:
			out[i] = writtenResult{key: counted.next(key.ID(), key.Hash())}
		case recorded != nil:
			out[i] = writtenResult{key: counted.next(key.ID(), recorded.Hash), kept: recorded}
		}
	}
	return out
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
