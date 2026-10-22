package snapshots

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Go evidence is written under go_coverage exactly as before, and the
// evidence of a line-precision language under line_coverage, each reading
// back as the unit's Coverage.
func TestUnitCoverageEvidenceKeepsGoKeyAndWritesLinesBesideIt(t *testing.T) {
	goUnit := mutants.UnitResult{Namespace: "main", Name: "a", Mutants: []mutants.Mutant{{Original: ">", Replacement: ">="}},
		Coverage: &mutants.CoverageEvidence{Version: mutants.GoCoverageEvidenceVersion, File: "main.go", Function: "main#a", Complete: true,
			Blocks: []mutants.CoverageBlock{{Span: "3.20,3.30", Line: 3, Column: 20, Weight: 1, Covered: true}}}}
	type plain unitResultWire
	before, err := json.Marshal(plain(unitResultWireOf(goUnit)))
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(UnitJSON(goUnit))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("Go unit written as\n%s\nwant as before\n%s", after, before)
	}
	pyUnit := mutants.UnitResult{Namespace: "calc", Name: "dormant", Mutants: []mutants.Mutant{},
		Coverage: &mutants.CoverageEvidence{Version: mutants.LineCoverageEvidenceVersion, Language: "python", File: "calc.py", Function: "calc#dormant", Complete: true,
			Blocks: []mutants.CoverageBlock{{Span: "8", Line: 8, Weight: 1}}}}
	data, err := json.Marshal(UnitJSON(pyUnit))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"line_coverage":{"version":1,"language":"python"`) || strings.Contains(string(data), "go_coverage") {
		t.Errorf("Python unit written as %s, want its evidence under line_coverage", data)
	}
	for _, unit := range []mutants.UnitResult{goUnit, pyUnit} {
		data, err := json.Marshal(UnitJSON(unit))
		if err != nil {
			t.Fatal(err)
		}
		var read mutants.UnitResult
		if err := ReadUnitJSON(data, &read); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(read, unit) {
			t.Errorf("%s read back as %+v, want %+v", data, read, unit)
		}
	}
}

// An over-limit outcome records the cap it went over under "limit", which
// reads back; every other outcome writes no such key, so a snapshot
// written before over-limit outcomes reads and is written as before.
func TestAnOverLimitOutcomeRecordsItsCap(t *testing.T) {
	over, err := json.Marshal(mutantWireOf(mutants.Mutant{Line: 7, Original: "1", Replacement: "0", Outcome: mutants.OverLimit, Limit: mutants.LimitOutput}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(over), `"outcome":"over-limit","limit":"output"`) {
		t.Errorf("an over-limit mutant written as %s", over)
	}
	var read mutantWire
	if err := json.Unmarshal(over, &read); err != nil || read.domain().Limit != mutants.LimitOutput {
		t.Errorf("an over-limit mutant read as %+v, %v", read.domain(), err)
	}
	killed, err := json.Marshal(mutantWireOf(mutants.Mutant{Line: 7, Original: "1", Replacement: "0", Outcome: mutants.Timeout}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(killed), "limit") {
		t.Errorf("a timed-out mutant written as %s", killed)
	}
}
