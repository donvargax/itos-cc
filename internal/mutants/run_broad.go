package mutants

func (s *Execution) markBroadStale() {
	if s.judged == nil || s.stored == nil {
		return
	}
	ids, hashes := FileKeys(s.file)
	for i, entry := range EntriesOf(ids, hashes, s.stored.Units) {
		if s.judged[i] || entry < 0 {
			continue
		}
		unit := &s.result.Snapshot.Units[i]
		for _, m := range s.stored.Units[entry].Mutants {
			if len(BroadChanges(m, s.moduleTests, s.currentSupport)) > 0 {
				unit.Stale = true
				break
			}
		}
	}
}
