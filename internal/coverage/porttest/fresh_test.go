package porttest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

func TestFreshCollectorsRefuseEveryUnplannedAdmittedSource(t *testing.T) {
	m, root, sources := fixture(t, "python", map[string]string{"a.py": "def a():\n    return 1\n"}, Measurement{Report: "SF:a.py\nDA:2,1\nend_of_record\n"})
	other := filepath.Join(t.TempDir(), "outside.py")
	m.Aggregate.Files[other] = "def outside():\n    return 2\n"
	for _, requested := range [][]string{{other}, append(sources, other)} {
		r, err := m.CollectFresh(t.Context(), port.Request{Sources: requested})
		if err == nil || !strings.Contains(err.Error(), other) {
			t.Fatalf("unplanned source passed: %+v %v", r, err)
		}
		if len(requested) == 2 {
			if hit, ok := r.LineCovered(filepath.Join(root, "a.py"), 2); !hit || !ok {
				t.Fatal("valid sibling report was discarded")
			}
		}
	}
}

func TestFreshListedRequiresActualPerIDDataWhileOrdinaryRemainsBestEffort(t *testing.T) {
	m, root, sources := fixture(t, "python", map[string]string{"a.py": "def a():\n    return 1\n"}, Measurement{Report: "SF:a.py\nDA:2,1\nend_of_record\n", Listed: map[string]map[string]*coverage.Report{}})
	group := coverage.Unloaded{Language: "python", Dir: root}
	d := m.Declarations[group]
	measured, err := m.CollectFresh(t.Context(), port.Request{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	d.Listed["python"] = map[string]*coverage.Report{"actual": measured}
	m.Declarations[group] = d
	tests := &coverage.PerTest{Root: root, Tests: []coverage.Test{{ID: "actual"}, {ID: "missing"}}, All: "independent-whole", Select: func(ids []string) string { return "independent-select " + strings.Join(ids, " ") }}
	got, err := m.MeasureFreshTests(t.Context(), port.Request{Sources: sources}, tests)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing selected ID accepted: %+v %v", got, err)
	}
	r, failure := m.MeasureListed(t.Context(), port.Request{Sources: sources}, tests)
	if failure != nil || r == nil {
		t.Fatalf("ordinary best effort changed: %+v %v", r, failure)
	}
	if ids := r.LineTests(filepath.Join(root, "a.py"), 2); len(ids) != 1 || ids[0] != "actual" {
		t.Fatalf("ordinary good sibling disappeared: %v", ids)
	}
}
