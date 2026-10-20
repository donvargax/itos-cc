package mutants

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
	ids, hashes := FileKeys(s.file)
	marks := map[string][]bool{}
	for i, entry := range EntriesOf(ids, hashes, s.stored.Units) {
		switch {
		case s.judged[i]:
			key := ids[i] + "\x00" + hashes[i]
			marks[key] = append(marks[key], false)
		case entry >= 0:
			recorded := s.stored.Units[entry]
			key := ids[i] + "\x00" + recorded.Hash
			marks[key] = append(marks[key], s.broadChanged(recorded))
		}
	}
	units := s.result.Snapshot.Units
	for i := range units {
		key := UnitID(units[i].Namespace, units[i].Name) + "\x00" + units[i].Hash
		if queue := marks[key]; len(queue) > 0 {
			units[i].Stale = units[i].Stale || queue[0]
			marks[key] = queue[1:]
		}
	}
}

// broadChanged says whether a whole-suite input one of the entry's mutants
// was decided under changed since.
func (s *Execution) broadChanged(entry UnitResult) bool {
	for _, m := range entry.Mutants {
		if len(BroadChanges(m, s.moduleTests, s.currentSupport)) > 0 {
			return true
		}
	}
	return false
}
