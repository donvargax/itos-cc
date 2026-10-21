package mutants

import "strings"

// Exception is one excepted mutant: a survivor no test can kill because it
// changes no behaviour. It is found by its function, the function's hash
// when it was excepted, and the site's place within the function, so a
// function that only moved keeps it.
type Exception struct {
	File     string `json:"file" yaml:"file"`         // slash-separated, from the project root
	Function string `json:"function" yaml:"function"` // namespace#name
	Hash     string `json:"hash" yaml:"hash"`         // the function's hash, as the snapshot records it
	// LineInFunction is the site's line counted from the function's first
	// line, which is 1.
	LineInFunction int    `json:"line_in_function" yaml:"line_in_function"`
	Column         int    `json:"column" yaml:"column"`
	Original       string `json:"original" yaml:"original"`
	Replacement    string `json:"replacement" yaml:"replacement"` // empty for a deletion
	Reason         string `json:"reason" yaml:"reason"`
	// LineText is the text of the site's line, less the space around it, as
	// it was when the entry was written: what mutation except --renew
	// matches the line against. Optional: entries written before it have
	// none.
	LineText string `json:"line_text,omitempty" yaml:"line_text,omitempty"`
}

// SameSite reports whether e and o except the same site: one function, one
// place within it, one change.
func (e Exception) SameSite(o Exception) bool {
	return e.File == o.File && e.Function == o.Function && e.LineInFunction == o.LineInFunction &&
		e.Column == o.Column && e.Original == o.Original && e.Replacement == o.Replacement
}

// Tests is mutation.tests: commands, run through the platform shell at the
// project root, that list the project's tests and run a selection of them,
// sharing only the tests' IDs and paths.
type Tests struct {
	// List prints one test per line: its ID and, after a tab, optionally
	// the file that defines it.
	List string
	// Run runs the tests {pattern} selects.
	Run string
	// IDsPattern is the pattern of a selection, with {ids} the selected
	// IDs, each written as Each, with {id} the ID, joined by Sep.
	IDsPattern string
	Each, Sep  string
	// Whole, optional, runs every listed test; without it, Run selects them
	// all.
	Whole string
	// Support are globs, from the project root, of the files every listed
	// test depends on, such as the step code its checks run: a kill by
	// listed tests holds while they are unchanged.
	Support []string
}

// Select is the command that runs the tests ids.
func (t *Tests) Select(ids []string) string {
	each := make([]string, len(ids))
	for i, id := range ids {
		each[i] = strings.ReplaceAll(t.Each, "{id}", id)
	}
	pattern := strings.ReplaceAll(t.IDsPattern, "{ids}", strings.Join(each, t.Sep))
	return strings.ReplaceAll(t.Run, "{pattern}", pattern)
}

// All is the command that runs every test, ids.
func (t *Tests) All(ids []string) string {
	if t.Whole != "" {
		return t.Whole
	}
	return t.Select(ids)
}
