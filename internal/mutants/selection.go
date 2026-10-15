package mutants

import (
	"slices"
	"strings"
)

// FailedSelection is a selection of listed tests that fails without any
// mutant.
type FailedSelection struct {
	IDs      []string // in the order the mutants name them
	Command  string   // the shell command line that ran them
	ExitCode int      // the command's exit code, -1 when it gave none
	Output   string
}

// SelectionKey names the set ids, whatever their order.
func SelectionKey(ids []string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

// AddFailed records that f, a selection some mutant of the file needed,
// failed without a mutant, once.
func (r *FileResult) AddFailed(f *FailedSelection) {
	key := SelectionKey(f.IDs)
	if !slices.ContainsFunc(r.FailedSelections, func(g FailedSelection) bool { return SelectionKey(g.IDs) == key }) {
		r.FailedSelections = append(r.FailedSelections, *f)
	}
}
