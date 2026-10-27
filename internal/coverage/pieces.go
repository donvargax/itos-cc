package coverage

import (
	"maps"
	"slices"
)

// Piece is a Segment as a store keeps it, whole: where it is, what it
// weighs and covered, the key Build folds it by, whether it ran whatever
// its weight, whether it is a decision counting its branches rather than
// a line or a statement block, and the sources whose data covered it. A
// report FromPieces makes of a report's Pieces answers every question that
// report answers of its files.
type Piece struct {
	Start, End  int
	Total       float64
	Covered     float64
	Key         string
	Ran         bool
	Branch      bool
	InProcess   bool
	Integration bool
}

// Files is each file the report measured, sorted.
func (r *Report) Files() []string {
	if r == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(r.files))
}

// Pieces is each segment the report holds of file, its lines' then its
// branches', in the report's order.
func (r *Report) Pieces(file string) []Piece {
	if r == nil {
		return nil
	}
	out := []Piece{}
	for _, kind := range []struct {
		segments []Segment
		branch   bool
	}{{r.files[file], false}, {r.branches[file], true}} {
		for _, s := range kind.segments {
			out = append(out, Piece{Start: s.Start, End: s.End, Total: s.Total, Covered: s.Covered, Key: s.Key, Ran: s.ran, Branch: kind.branch,
				InProcess: s.from&(1<<InProcess) != 0, Integration: s.from&(1<<Integration) != 0})
		}
	}
	return out
}

// FromPieces makes a report of the pieces of each file, as Pieces gave
// them: a file with none is measured, and holds nothing.
func FromPieces(files map[string][]Piece) *Report {
	r := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}}
	for file, pieces := range files {
		r.files[file] = []Segment{}
		for _, p := range pieces {
			s := Segment{Start: p.Start, End: p.End, Total: p.Total, Covered: p.Covered, Key: p.Key, ran: p.Ran}
			if p.InProcess {
				s.from |= 1 << InProcess
			}
			if p.Integration {
				s.from |= 1 << Integration
			}
			if p.Branch {
				r.branches[file] = append(r.branches[file], s)
				continue
			}
			r.files[file] = append(r.files[file], s)
		}
	}
	return r
}

// Tests is the coverage of each listed test the report holds, nil for
// none.
func (r *Report) Tests() *TestCoverage {
	if r == nil {
		return nil
	}
	return r.tests
}

// IDs is the listed tests measured, in the list's order.
func (tc *TestCoverage) IDs() []string {
	if tc == nil {
		return nil
	}
	return slices.Clone(tc.ids)
}

// Of is the coverage of the listed test id, nil when it was not measured.
func (tc *TestCoverage) Of(id string) *Report {
	if tc == nil {
		return nil
	}
	return tc.reports[id]
}

// Proves is whether r lists file's executable lines completely, at its
// format's line precision (Lines).
func (r *Report) Proves(file string) bool {
	return r != nil && r.proven[file]
}

// Listed is the executable lines List recorded of file, a static
// inventory's or its module report's, none of them executed, as pieces.
func (r *Report) Listed(file string) []Piece {
	if r == nil {
		return nil
	}
	var out []Piece
	for _, s := range r.inventory[file] {
		out = append(out, Piece{Start: s.Start, End: s.End, Total: s.Total, Key: s.Key})
	}
	return out
}

// FromPlan makes the report of a plan of language in its build root dir
// whose commands all succeeded and wrote their reports, as a store kept
// it: the pieces of each file it names (FromPieces), the lines listed of
// each file of listed (Listed), its language measured, each of proven
// listed completely (Lines), and each of unloaded loaded by no test
// (Unloaded).
func FromPlan(language, dir string, files, listed map[string][]Piece, proven, unloaded []string) *Report {
	r := FromPieces(files)
	for file, pieces := range listed {
		if r.inventory == nil {
			r.inventory = map[string][]Segment{}
		}
		for _, p := range pieces {
			r.inventory[file] = append(r.inventory[file], Segment{Start: p.Start, End: p.End, Total: p.Total, Key: p.Key})
		}
	}
	r.MarkMeasured(language)
	r.proven, r.unloaded = map[string]bool{}, map[string]Unloaded{}
	for _, file := range proven {
		r.proven[file] = true
	}
	for _, file := range unloaded {
		r.unloaded[file] = Unloaded{Language: language, Dir: dir}
	}
	return r
}
