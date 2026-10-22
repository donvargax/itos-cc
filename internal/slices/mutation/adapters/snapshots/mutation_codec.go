package snapshots

import (
	"bytes"
	"encoding/json"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// snapshotWire is one source file's results, stored at
// .metrics/mutate/<path>.json and meant to be committed with the code.
type snapshotWire struct {
	Version  int    `json:"version"`
	File     string `json:"file"`
	Language string `json:"language"`
	// Tests is the SHA-256 of each test file that imports the file, by its
	// slash-separated path from the project root: the results hold while
	// they are the same. A snapshot written before snapshots recorded tests
	// has none, nil, which matches no tests.
	Tests map[string]string `json:"tests"`
	Units []unitResultWire  `json:"units"`
	// Listed is the file that defines each listed test an outcome records
	// (mutantWire.Tests), by its ID, as the list command names it, "" for none.
	// Left out when no outcome records any.
	Listed map[string]string `json:"listed,omitempty"`
	// ListedFiles is the SHA-256 of each file Listed names, and Support of
	// each support file (mutation.tests.support), by its slash-separated
	// path from the project root, when the outcomes were recorded: listed
	// outcomes hold while they are the same (ListedChanged). Left out when
	// no outcome is listed.
	ListedFiles map[string]string `json:"listed_files,omitempty"`
	Support     map[string]string `json:"support,omitempty"`
	// ListedFingerprints is the fingerprint the list command printed for
	// each test of Listed listed with one, by its ID: such a test's outcomes
	// hold while it is listed with it. Left out when none has one.
	ListedFingerprints map[string]string `json:"listed_fingerprints,omitempty"`
}

// unitResultWire is one function's mutants. Hash is the function's source hash:
// while it is unchanged, killed mutants stay killed.
type unitResultWire struct {
	Namespace string       `json:"namespace"`
	Name      string       `json:"name"`
	Hash      string       `json:"hash"`
	StartLine int          `json:"start_line"`
	EndLine   int          `json:"end_line"`
	Killed    int          `json:"killed"`
	Survived  int          `json:"survived"`
	Uncovered int          `json:"uncovered"`
	Sites     int          `json:"sites"`
	Mutants   []mutantWire `json:"mutants"`
	// Coverage is independent executable coverage for this function. Nil
	// means the snapshot predates strict coverage of its language or the
	// function was not measured by an admitted producer. Go evidence is
	// written under go_coverage, as it always was; evidence of a
	// line-precision language (mutants.CoverageEvidence.Language) under
	// line_coverage (MarshalJSON).
	Coverage *mutants.CoverageEvidence `json:"go_coverage,omitempty"`
	// Stale marks an entry decided under tests other than those the
	// snapshot records: a run that did not judge the function kept it,
	// outcomes and survivors included, after the tests that import the file
	// changed. It is never reused, and mutation check calls it stale, until
	// a run judges the function again. Left out of the file when false, so
	// every snapshot without such an entry reads and is written as before.
	Stale bool `json:"stale,omitempty"`
}

// unitLines is unitResultWire as a snapshot writes a unit whose coverage
// evidence is of a line-precision language: under line_coverage rather
// than go_coverage. Converting between the two keeps their fields the same.
type unitLines struct {
	Namespace string                    `json:"namespace"`
	Name      string                    `json:"name"`
	Hash      string                    `json:"hash"`
	StartLine int                       `json:"start_line"`
	EndLine   int                       `json:"end_line"`
	Killed    int                       `json:"killed"`
	Survived  int                       `json:"survived"`
	Uncovered int                       `json:"uncovered"`
	Sites     int                       `json:"sites"`
	Mutants   []mutantWire              `json:"mutants"`
	Coverage  *mutants.CoverageEvidence `json:"line_coverage,omitempty"`
	Stale     bool                      `json:"stale,omitempty"`
}

// MarshalJSON writes u with its coverage evidence under go_coverage, Go's
// key, or under line_coverage for a line-precision language, so a Go
// snapshot reads and is written as before.
func (u unitResultWire) MarshalJSON() ([]byte, error) {
	type plain unitResultWire
	var v any = plain(u)
	if u.Coverage != nil && u.Coverage.Language != "" {
		v = unitLines(u)
	}
	// As the snapshot store writes a snapshot: the encoder that embeds this
	// escapes what its own settings say.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// UnmarshalJSON reads a unit, its coverage evidence under go_coverage or
// line_coverage.
func (u *unitResultWire) UnmarshalJSON(data []byte) error {
	type plain unitResultWire
	var raw struct {
		plain
		Lines *mutants.CoverageEvidence `json:"line_coverage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = unitResultWire(raw.plain)
	if u.Coverage == nil {
		u.Coverage = raw.Lines
	}
	return nil
}

// mutantWire is one site's outcome.
type mutantWire struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Offset      int    `json:"offset"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	// Scope is the scope of the tests that decided Outcome: ScopeAllTests,
	// a --test-command line, or "" for ScopeOwn, which is also how an
	// outcome recorded before scopes reads. See TestScope.
	Scope string `json:"scope,omitempty"`
	// Tests is, for an outcome of ScopeListed, the IDs of the listed tests
	// that decided it after the file's own tests survived the mutant: those
	// whose coverage reaches its line.
	Tests []string `json:"tests,omitempty"`
	// SuiteEvidence is the test and configured-support input of a
	// broad-scope outcome, of any language. Nil means legacy evidence was
	// not recorded. Go outcomes recorded it under go_evidence before every
	// language did; that key reads as this one, and is never written.
	SuiteEvidence *mutants.SuiteEvidence `json:"suite_evidence,omitempty"`
	// Excepted is the reason itos-cc.yaml gives for a survivor it excepts,
	// as a run or a check judged it; never written to the snapshot, which
	// records the survivor as it is.
	Excepted string `json:"-"`
}

// UnmarshalJSON reads a mutant, its evidence under suite_evidence or, as Go
// outcomes recorded it before, go_evidence.
func (m *mutantWire) UnmarshalJSON(data []byte) error {
	type plain mutantWire
	var raw struct {
		plain
		GoEvidence *mutants.SuiteEvidence `json:"go_evidence"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = mutantWire(raw.plain)
	if m.SuiteEvidence == nil {
		m.SuiteEvidence = raw.GoEvidence
	}
	return nil
}

func snapshotWireOf(v mutants.Snapshot) snapshotWire {
	out := snapshotWire{}
	out.Version = v.Version
	out.File = v.File
	out.Language = v.Language
	out.Tests = v.Tests
	if v.Units != nil {
		out.Units = make([]unitResultWire, len(v.Units))
		for i, x := range v.Units {
			out.Units[i] = unitResultWireOf(x)
		}
	}
	out.Listed = v.Listed
	out.ListedFiles = v.ListedFiles
	out.Support = v.Support
	out.ListedFingerprints = v.ListedFingerprints
	return out
}

func (v snapshotWire) domain() mutants.Snapshot {
	out := mutants.Snapshot{}
	out.Version = v.Version
	out.File = v.File
	out.Language = v.Language
	out.Tests = v.Tests
	if v.Units != nil {
		out.Units = make([]mutants.UnitResult, len(v.Units))
		for i, x := range v.Units {
			out.Units[i] = x.domain()
		}
	}
	out.Listed = v.Listed
	out.ListedFiles = v.ListedFiles
	out.Support = v.Support
	out.ListedFingerprints = v.ListedFingerprints
	return out
}

func unitResultWireOf(v mutants.UnitResult) unitResultWire {
	out := unitResultWire{}
	out.Namespace = v.Namespace
	out.Name = v.Name
	out.Hash = v.Hash
	out.StartLine = v.StartLine
	out.EndLine = v.EndLine
	out.Killed = v.Killed
	out.Survived = v.Survived
	out.Uncovered = v.Uncovered
	out.Sites = v.Sites
	if v.Mutants != nil {
		out.Mutants = make([]mutantWire, len(v.Mutants))
		for i, x := range v.Mutants {
			out.Mutants[i] = mutantWireOf(x)
		}
	}
	out.Coverage = v.Coverage
	out.Stale = v.Stale
	return out
}

func (v unitResultWire) domain() mutants.UnitResult {
	out := mutants.UnitResult{}
	out.Namespace = v.Namespace
	out.Name = v.Name
	out.Hash = v.Hash
	out.StartLine = v.StartLine
	out.EndLine = v.EndLine
	out.Killed = v.Killed
	out.Survived = v.Survived
	out.Uncovered = v.Uncovered
	out.Sites = v.Sites
	if v.Mutants != nil {
		out.Mutants = make([]mutants.Mutant, len(v.Mutants))
		for i, x := range v.Mutants {
			out.Mutants[i] = x.domain()
		}
	}
	out.Coverage = v.Coverage
	out.Stale = v.Stale
	return out
}

func mutantWireOf(v mutants.Mutant) mutantWire {
	out := mutantWire{}
	out.Line = v.Line
	out.Column = v.Column
	out.Offset = v.Offset
	out.Original = v.Original
	out.Replacement = v.Replacement
	out.Outcome = v.Outcome
	out.Scope = v.Scope
	out.Tests = v.Tests
	out.SuiteEvidence = v.SuiteEvidence
	out.Excepted = v.Excepted
	return out
}

func (v mutantWire) domain() mutants.Mutant {
	out := mutants.Mutant{}
	out.Line = v.Line
	out.Column = v.Column
	out.Offset = v.Offset
	out.Original = v.Original
	out.Replacement = v.Replacement
	out.Outcome = v.Outcome
	out.Scope = v.Scope
	out.Tests = v.Tests
	out.SuiteEvidence = v.SuiteEvidence
	out.Excepted = v.Excepted
	return out
}

// SnapshotJSON returns the concrete stored representation of the pure model.
func SnapshotJSON(s mutants.Snapshot) any { return snapshotWireOf(s) }

// UnitJSON returns the stored representation of one unit, for boundary consumers.
func UnitJSON(u mutants.UnitResult) any { return unitResultWireOf(u) }

// MutantJSON returns the stored representation of one mutant.
func MutantJSON(m mutants.Mutant) any { return mutantWireOf(m) }

// ReadSnapshotJSON reads a current or legacy stored snapshot.
func ReadSnapshotJSON(data []byte, s *mutants.Snapshot) error {
	var w snapshotWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*s = w.domain()
	return nil
}
func ReadUnitJSON(data []byte, u *mutants.UnitResult) error {
	var w unitResultWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*u = w.domain()
	return nil
}
func ReadMutantJSON(data []byte, m *mutants.Mutant) error {
	var w mutantWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*m = w.domain()
	return nil
}
