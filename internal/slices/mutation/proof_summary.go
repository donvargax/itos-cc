package mutation

import (
	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// The unit of a summary names what its judged counts: one change for
// mutation run, counted or complete, one function for mutation check, whose
// counts are per function.
const (
	proofUnitMutant   = "mutant"
	proofUnitFunction = "function"
)

// proofSummary is the member itos_proof of a green run's --json object, the
// summary itos reads from a code proof's stdout and logs. Every count is null
// when nothing was judged, never a zero: itos refuses an explicit judged of 0
// (ci-code-proof-empty), and a range with no site passes here, a test-only
// repair above all.
type proofSummary struct {
	Version   int     `json:"version"`
	Judged    *int    `json:"judged"`
	Unit      *string `json:"unit"`
	Files     *int    `json:"files"`
	Killed    *int    `json:"killed"`
	Survived  *int    `json:"survived"`
	Excepted  *int    `json:"excepted"`
	Uncovered *int    `json:"uncovered"`
}

// proofTally adds up what a run judged, for its summary.
type proofTally struct {
	judged, files, killed, survived, excepted, uncovered int
}

// summary is the tally as a summary of unit, nil when problems fail the run:
// the reader parses a green run's object alone, and a failed run's stays as
// it was.
func (t proofTally) summary(unit string, problems []*cli.Problem) *proofSummary {
	if len(problems) > 0 {
		return nil
	}
	if t.judged == 0 {
		return &proofSummary{Version: 1}
	}
	return &proofSummary{Version: 1, Judged: &t.judged, Unit: &unit, Files: &t.files, Killed: &t.killed,
		Survived: &t.survived, Excepted: &t.excepted, Uncovered: &t.uncovered}
}

// countedProof is the summary of a counted run: the selected sites with an
// outcome, a judged site's or an uncovered one's, which has one though no
// trial ran.
func countedProof(sites []countedSite, problems []*cli.Problem) *proofSummary {
	var t proofTally
	files := map[string]bool{}
	for _, site := range sites {
		if site.Outcome == "" {
			continue
		}
		t.judged++
		files[site.File] = true
		switch {
		case mutants.Noticed(site.Outcome):
			t.killed++
		case site.Outcome == mutants.Survived && site.Excepted != "":
			t.excepted++
		case site.Outcome == mutants.Survived:
			t.survived++
		case site.Outcome == mutants.Uncovered:
			t.uncovered++
		}
	}
	t.files = len(files)
	return t.summary(proofUnitMutant, problems)
}

// runProof is the summary of a complete run: the mutants of the functions
// each file judged, ran or reused, as its counts hold them.
func runProof(files []runFile, problems []*cli.Problem) *proofSummary {
	var t proofTally
	for _, f := range files {
		judged := f.Killed + f.Survived + f.Excepted + f.Uncovered + f.Flaky
		if judged == 0 {
			continue
		}
		t.judged += judged
		t.files++
		t.killed += f.Killed
		t.survived += f.Survived
		t.excepted += f.Excepted
		t.uncovered += f.Uncovered
	}
	return t.summary(proofUnitMutant, problems)
}

// checkProof is the summary of mutation check: the functions judged, and the
// mutants their entries count, as itos's own example reports them.
func checkProof(files []checkFile, problems []*cli.Problem) *proofSummary {
	var t proofTally
	for _, f := range files {
		if len(f.Functions) == 0 {
			continue
		}
		t.files++
		t.judged += len(f.Functions)
		for _, fn := range f.Functions {
			t.killed += fn.Killed
			t.survived += fn.Survived
			t.excepted += fn.Excepted
			t.uncovered += fn.Uncovered
		}
	}
	return t.summary(proofUnitFunction, problems)
}
