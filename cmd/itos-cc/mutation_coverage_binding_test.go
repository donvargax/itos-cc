package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
)

// These are actual legacy consumer functions. They still discover/configure on
// disk until T100, but coverage evidence comes solely from the bound port.
func TestMutationCoverageConsumersUseActualBindings(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	hit, miss := filepath.Join(root, "hit.py"), filepath.Join(root, "miss.py")
	d := covtest.Measurement{Language: "python", Report: "SF:hit.py\nDA:2,1\nend_of_record\n", InventoryReport: "SF:miss.py\nDA:5,1\nend_of_record\n"}
	m := &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: map[string]string{hit: "source", miss: "source"}}, Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "python", Dir: root}: d}}
	in := parsedMutationFixture(t, adapters{MutationCoverage: m}, "run")
	sources := []string{hit, miss}
	r, err := loadCoverage(t.Context(), in, sources, port.OwnTests, nil, nil)
	if err != nil || !r.Has(hit) || r.Has(miss) {
		t.Fatalf("bound measurement: %v %v", r, err)
	}
	r, _, _, _, err = strictCoverage(t.Context(), in, []string{"python"}, sources, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines, complete := r.Lines(miss)
	if !complete || len(lines) != 1 || lines[0].Line != 5 || lines[0].Covered {
		t.Fatalf("actual bound inventory lost: %+v %v", lines, complete)
	}
	// Replacing the independently supplied inventory cannot retain that proof.
	wrong := *m
	wrong.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "python", Dir: root}: {Language: "python", Report: d.Report}}
	in.adapters.MutationCoverage = &wrong
	r, _, _, _, err = strictCoverage(t.Context(), in, []string{"python"}, sources, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, complete := r.Lines(miss); complete {
		t.Fatal("wrong inventory binding still supplied proof")
	}
	in.adapters.MutationCoverage = nil
	if _, err := loadCoverage(t.Context(), in, sources, port.OwnTests, nil, nil); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("missing binding escaped: %v", err)
	}
	in.adapters.MutationCoverage = &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: m.Aggregate.Files}, Declarations: map[coverage.Unloaded]covtest.Measurement{}}
	r, err = loadCoverage(t.Context(), in, sources, port.OwnTests, nil, nil)
	if err != nil || r.Has(hit) {
		t.Fatal("wrong measurement binding returned known real evidence")
	}
	in.values["no-coverage"] = []string{"true"}
	in.adapters.MutationCoverage = nil
	if r, err := loadCoverage(t.Context(), in, sources, port.OwnTests, nil, nil); r != nil || err != nil {
		t.Fatal("unused capability demanded by no-coverage")
	}
	delete(in.values, "no-coverage")
	in.adapters.MutationCoverage = m
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := loadCoverage(ctx, in, sources, port.OwnTests, nil, nil); err != context.Canceled {
		t.Fatalf("caller cancellation lost: %v", err)
	}
}
