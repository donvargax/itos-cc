package teststructure

import "sort"

// History is what a snapshot of test structure records: each file's last
// report, by the key it is recorded under, the file's path from the project
// root. A run records the files it measured and keeps every other, so a run
// over some files never drops another's report.
type History struct{ byFile map[string]FileReport }

// HistoryOf is the history files record, as a snapshot holds them, each by
// its File; a file listed twice is its last report.
func HistoryOf(files []FileReport) History {
	h := History{byFile: map[string]FileReport{}}
	for _, r := range files {
		h.byFile[r.File] = r
	}
	return h
}

// Record compares r, this run's report of the file recorded under key, with
// the report recorded for it before, if any, and records r in its place,
// under key. It returns r with the comparison, named as the caller named it.
func (h History) Record(key string, r FileReport) FileReport {
	if prev, ok := h.byFile[key]; ok {
		prev.Compare = nil
		r.CompareTo(prev)
	}
	recorded := r
	recorded.File = key
	h.byFile[key] = recorded
	return r
}

// Files are the recorded reports as a snapshot writes them: sorted by key,
// none carrying a comparison, an empty list rather than none.
func (h History) Files() []FileReport {
	files := []FileReport{}
	for _, r := range h.byFile {
		r.Compare = nil
		files = append(files, r)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	return files
}
