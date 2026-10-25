package mutation

import (
	"path/filepath"
	"strings"
	"testing"

	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The code proof judges a mutant on a switch case's line by this package's
// tests alone: Go's coverage gives a case's expression no block of its own,
// so no listed scenario reaches it. MUT-178 holds that a counted run's
// stages list the listed selection's baseline; this holds the other side,
// that no stage of a trial is taken for it, here a trial whose own stage
// failed, its tests declared nowhere.
func TestACountedRunsStagesTakeNoTrialStageForTheSelectionBaseline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "frozen")
	src := "package board\n\nfunc Pos(x int) bool { return x > 0 }\n"
	start := uint(strings.Index(src, ">"))
	site := mutants.Site{Start: start, End: start + 1, Line: 3, Column: 33, Original: ">", Replacement: ">="}
	tree := selporttest.TreeOf(root, map[string]string{"board.go": src}, nil)
	h := Handler{FreshWorkers: &mutantstest.Workers{Files: map[string]string{filepath.Join(root, "board.go"): src}},
		Tree: tree, Toolchain: &mutantstest.Inputs{}}
	plan := &mutants.FreshPlan{FrozenRoot: root, Selected: []mutants.FreshCandidate{{Path: "board.go", Function: "board#Pos", Site: site}}}
	command := mutants.Command{Root: root, Dir: root, Args: []string{"go", "test", "./."}}
	prep := &mutants.FreshPreparation{Baselines: []mutants.FreshBaseline{{Command: command, Paths: []string{"board.go"}}}}
	result := &countedResult{Selected: []countedSite{{File: "board.go", Line: 3, Column: 33, State: "unattempted"}}}
	var stale []mutants.StaleException
	if err := h.trials(t.Context(), CountedQuery{Workers: 1, TimeoutFactor: 10}, plan, prep, &mutants.FreshExceptions{}, result, &stale); err != nil {
		t.Fatal(err)
	}
	if site := result.Selected[0]; site.State != "failed" || len(site.Stages) != 1 || site.Stages[0].Name != "own" || site.Stages[0].State != "failed" {
		t.Fatalf("site %+v, want failed at its own stage", site)
	}
	if len(result.Stages) != 1 || result.Stages[0].Name != "trials" || result.Stages[0].State != "failed" {
		t.Errorf("the run's stages %+v, want the trials alone, failed", result.Stages)
	}
}
