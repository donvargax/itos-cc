package metricsdir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
