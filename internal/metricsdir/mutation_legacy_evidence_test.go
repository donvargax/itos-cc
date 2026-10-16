package metricsdir

import (
	"encoding/json"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func TestGoEvidenceReadsAsSuiteEvidenceAndIsWrittenUnderTheNeutralKey(t *testing.T) {
	var m mutants.Mutant
	if err := ReadMutantJSON([]byte(`{"outcome":"killed","scope":"all-tests","go_evidence":{"tests":{"a_test.go":"h"},"support":{}}}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.SuiteEvidence == nil || m.SuiteEvidence.Tests["a_test.go"] != "h" {
		t.Fatalf("go_evidence read as %+v, want the suite evidence", m.SuiteEvidence)
	}
	data, err := json.Marshal(MutantJSON(m))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["go_evidence"]; ok || raw["suite_evidence"] == nil {
		t.Errorf("written as %s, want suite_evidence alone", data)
	}
}
