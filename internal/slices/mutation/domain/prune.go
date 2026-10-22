package domain

// Pruned is each entry of entries, in their order, that stale, what a
// check of the whole project finds stale of them, says nothing can mend:
// its why ExceptionGone, ExceptionKilled or ExceptionExcluded, each with
// that why and the line stale names. An entry that changed or moved is
// left: mutation except --renew or an edit of its file mends it. A stale
// exception that is no entry of entries, such as a moved entry as it reads
// where its function is now, prunes nothing.
func Pruned(entries []Exception, stale []StaleException) []StaleException {
	used := make([]bool, len(stale))
	var out []StaleException
	for _, e := range entries {
		for i, s := range stale {
			if used[i] || s.Exception != e {
				continue
			}
			used[i] = true
			if s.Why == ExceptionGone || s.Why == ExceptionKilled || s.Why == ExceptionExcluded {
				out = append(out, s)
			}
			break
		}
	}
	return out
}
