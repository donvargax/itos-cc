package domain

import (
	"slices"
	"sort"
)

// States of a function's cached results against its source now.
const (
	SnapshotFresh = "fresh"   // the snapshot records the function as it is
	Stale         = "stale"   // the function, or the tests that import its file, changed since the snapshot recorded it, or it has a site the snapshot never recorded
	Missing       = "missing" // the snapshot does not record the function
)

// CoverageCheck is one current function's independent coverage verdict.
type CoverageCheck struct {
	File, Function string
	// Language is the function's language.
	Language string
	// Line and EndLine are the lines the function starts and ends on now.
	Line, EndLine int
	State         string // "missing", "stale", "fresh", "unsupported"
	Changed       []string
	Blocks        []CoverageBlock
	Evidence      *CoverageEvidence
}

// GoCoverageCheck is CoverageCheck, by its name before other languages had
// strict coverage.
type GoCoverageCheck = CoverageCheck

// Freshness is how a function's cached results hold against the function
// as it is now, as mutation check calls them.
type Freshness struct {
	State string // SnapshotFresh, Stale, or Missing
	// Tests is how the tests that import the file changed since the
	// snapshot, when that alone makes the function Stale; nil otherwise.
	Tests *TestChange
	// Marked is true when the function is Stale because its entry is marked
	// Stale: a run that did not judge it kept it after the tests that
	// import the file changed.
	Marked bool
	// Listed is each file a listed outcome of the function rests on that
	// changed since the snapshot recorded it, when that makes the function
	// Stale: a file that defines one of its tests, or a support file; nil
	// otherwise.
	Listed []string
	// ListedTests is each test recorded with a fingerprint that a listed
	// outcome of the function rests on and that the list no longer prints
	// with that fingerprint, when that makes the function Stale; nil
	// otherwise.
	ListedTests []string
	// Broad is each module test or configured support input changed for a
	// Go all-tests or test-command outcome.
	Broad []string
	// Unrecorded is each site the function has now that its entry does not
	// record although its hash and tests match, in line order, as when a
	// newer itos-cc adds a mutation operator; such a function is Stale. nil
	// otherwise.
	Unrecorded []Site
}

// FreshnessOf is the freshness of entry, the snapshot's entry paired with a
// function by name and hash (PairByHash), nil for none, against the
// function as it is now: its hash, its sites, and tests, how the tests that
// import its file changed since the snapshot (Snapshot.TestsChanged), and
// listed, how the tests and files its listed outcomes rest on did
// (ListedChangedListing). It is
// mutation check's verdict, which the architecture graph shows too, so the
// two cannot drift. The entry holds while it has the function's hash, so a
// function that only moved is fresh, the tests are those it recorded, no
// run kept it marked stale, the tests and files its listed outcomes rest on
// are unchanged, and it records every site the function has.
func FreshnessOf(entry *UnitResult, hash string, tests *TestChange, listed *ListedChange, broad []string, sites []Site) Freshness {
	switch {
	case entry == nil:
		return Freshness{State: Missing}
	case entry.Hash != hash:
		return Freshness{State: Stale}
	case tests != nil:
		return Freshness{State: Stale, Tests: tests}
	case entry.Stale:
		return Freshness{State: Stale, Marked: true}
	}
	if files, tests := listed.Files(entry.Mutants), listed.Tests(entry.Mutants); len(files) > 0 || len(tests) > 0 {
		return Freshness{State: Stale, Listed: files, ListedTests: tests}
	}
	if len(broad) > 0 {
		return Freshness{State: Stale, Broad: broad}
	}
	if u := Unrecorded(entry.Mutants, sites); len(u) > 0 {
		return Freshness{State: Stale, Unrecorded: u}
	}
	return Freshness{State: SnapshotFresh}
}

// unrecorded is each of sites, in line order, that recorded has no mutant
// for.
func Unrecorded(recorded []Mutant, sites []Site) []Site {
	has := map[string]bool{}
	for _, m := range recorded {
		has[m.Key()] = true
	}
	var out []Site
	for _, s := range sites {
		if !has[s.Key()] {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, LineOrder)
	return out
}

// OnSites is entry, the snapshot's entry paired with a function (nil for
// none), less each mutant it records at a site the function no longer
// holds, its counts less theirs, while entry has hash, the function's hash
// now, so its mutants are of the function's text; sites are the function's
// sites now. A recorded mutant matches a site as reuse matches them, by its
// place in the function, original and replacement (Site.Key). Such a mutant
// is not in the code, as when a newer itos-cc removes a mutation operator,
// so it bears on no verdict and no count, and a run, which builds the entry
// from the sites, leaves it out. An entry of another hash is returned as it
// is: it is stale anyway. entry is never changed.
func OnSites(entry *UnitResult, hash string, sites []Site) *UnitResult {
	if entry == nil || entry.Hash != hash {
		return entry
	}
	has := map[string]bool{}
	for _, s := range sites {
		has[s.Key()] = true
	}
	out := *entry
	out.Mutants = make([]Mutant, 0, len(entry.Mutants))
	for _, m := range entry.Mutants {
		if has[m.Key()] {
			out.Mutants = append(out.Mutants, m)
			continue
		}
		out.Sites--
		switch m.Outcome {
		case Killed, Timeout:
			out.Killed--
		case Survived:
			out.Survived--
		case Uncovered:
			out.Uncovered--
		}
	}
	if len(out.Mutants) == len(entry.Mutants) {
		return entry
	}
	return &out
}

// placed is recorded at the line and column of its site now, which differ
// from those recorded when the unchanged function moved.
func Placed(recorded []Mutant, sites []Site) []Mutant {
	at := map[string]Site{}
	for _, s := range sites {
		at[s.Key()] = s
	}
	out := []Mutant{}
	for _, m := range recorded {
		if s, ok := at[m.Key()]; ok {
			m.Line, m.Column = s.Line, s.Column
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Line != out[b].Line {
			return out[a].Line < out[b].Line
		}
		return out[a].Column < out[b].Column
	})
	return out
}

// FileCheck is one source file's functions checked against its snapshot.
type FileCheck struct {
	Rel       string // the file's path as output names it, from the working directory
	Key       string // the file's path as its snapshot names it, from the project root
	Functions []FunctionCheck
	// StaleExceptions is each exception of the functions checked that no
	// longer holds: those whose function changed or whose site is gone,
	// then those whose mutant a fresh entry records killed.
	StaleExceptions []StaleException
}

// FunctionCheck is one function's cached results.
type FunctionCheck struct {
	Function  string // namespace#name
	StartLine int    // where it starts now
	Freshness
	// Entry is the snapshot's entry for the function, zero when Missing.
	Entry UnitResult
	// Mutants is what Entry records when fresh, in site order, each at its
	// site's line and column now, a survivor itos-cc.yaml excepts with its
	// reason; nil otherwise.
	Mutants []Mutant
	// LeftOut is how many mutants the function's entry records at sites it
	// no longer holds, which OnSites left out of Entry, when fresh; 0
	// otherwise. The next mutation run drops them from the entry.
	LeftOut int
	Unit    int // the function's index in its file's units
}

// NeedsMutationCoverage reports whether Run must test at least one mutant.
// Fresh kills, timeouts, uncovered outcomes and excepted survivors are
// reusable; a changed/missing function or ordinary survivor is not.
func NeedsMutationCoverage(checks []FileCheck, mutateAll bool) bool {
	if mutateAll {
		return true
	}
	for _, file := range checks {
		for _, function := range file.Functions {
			if function.State != SnapshotFresh {
				return true
			}
			for _, mutant := range function.Mutants {
				if mutant.Outcome == Survived && mutant.Excepted == "" {
					return true
				}
			}
		}
	}
	return false
}

// judged is true while Entry's mutants hold for the sites it records: the
// function is fresh, or Stale only for sites Entry never recorded. Mutation
// sample and mutation except read only those mutants, so they judge such a
// function as they did before check called it stale.
func (c FunctionCheck) Judged() bool {
	return c.State == SnapshotFresh || len(c.Unrecorded) > 0
}

// CoverageFreshness judges an admitted function's evidence against current inputs.
// Its caller supplies names, hashes and the measured record; it reads nothing.
func CoverageFreshness(check CoverageCheck, key, hash string, evidence *CoverageEvidence, inputs map[string]string) CoverageCheck {
	if evidence != nil && evidence.Complete &&
		evidence.Language == EvidenceKey(check.Language) && evidence.Version == EvidenceVersion(check.Language) &&
		evidence.File == key && evidence.Function == check.Function && evidence.Hash == hash {
		check.Blocks = evidence.Blocks
		check.Evidence = evidence
		check.State = "fresh"
		before, now := CloneHashes(evidence.Inputs), CloneHashes(inputs)
		delete(before, "@producer")
		delete(now, "@producer")
		check.Changed = GoCoverageChanges(before, now)
		if !ValidCoverageProducer(check.Language, evidence.Producer) {
			check.Changed = append(check.Changed, "coverage producer/options")
		}
		if evidence.Producer == "" && len(check.Changed) == 0 {
			check.Changed = []string{"coverage producer/options"}
		}
		if len(check.Changed) > 0 {
			check.State = "stale"
		}
	}
	return check
}
