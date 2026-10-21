package domain_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The obsolete direct legacy helper detached this read after cancellation.
// Actual Run uses its parent caller context; fail-fast only stops its execute
// child. Preserve bytes/missing/support facts through actual publication here.
func TestListedFinalizationUsesActualInputsAndCallerContext(t *testing.T) {
	f, lister, m, _ := listingFixture(t)
	text := f.files[filepath.Join(f.root, "board_test.go")]
	lister.Listings[f.root]["fixture-list"] = porttest.Listing{Stdout: "first\tboard_test.go\nmissing\tmissing_test.go\n"}
	own := coverage.FromLines("go", map[string][]coverage.Line{f.path: {{Line: 2, Covered: true}}})
	m.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: f.root}: {Language: "go", Report: "mode: set\nexample.test/board/board.go:2.25,2.40 1 1\n", Listed: map[string]map[string]*coverage.Report{"go": {"first": own, "missing": own}}}}
	parsed, err := selection.Parse(t.Context(), f.options.Tree, f.inputs, f.path)
	if err != nil {
		t.Fatal(err)
	}
	for command, facts := range f.workers.Tests[f.root] {
		for _, site := range mutants.Sites(parsed) {
			facts.Verdicts = append(facts.Verdicts, porttest.Verdict{Path: "board.go", Site: site, Outcome: mutants.Survived})
		}
		f.workers.Tests[f.root][command] = facts
	}
	parsed.Close()
	f.workers.Tests[f.root]["select first missing"] = porttest.Tests{}
	f.options.Support = map[string]string{"fixture.feature": "support-hash"}
	result := f.run(t)
	sum := sha256.Sum256([]byte(text))
	want := map[string]string{"board_test.go": hex.EncodeToString(sum[:]), "missing_test.go": ""}
	snap := result[0].Snapshot
	if !reflect.DeepEqual(snap.ListedFiles, want) || snap.Support["fixture.feature"] != "support-hash" {
		t.Fatalf("actual retained listed inputs: %v %v", snap.ListedFiles, snap.Support)
	}
	path := filepath.Join(f.root, "board_test.go")
	if !slices.Contains(f.inputs.Reads, path) {
		t.Fatalf("actual listed bytes were not read: %v", f.inputs.Reads)
	}
	beforeReads, beforeWrites := len(f.inputs.Reads), len(f.snapshots.Written)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("parent stopped")
	cancel(cause)
	_, failure := f.inputs.Tests(ctx, f.root, []string{path})
	var stopped mutants.InputCancelled
	if !errors.As(failure, &stopped) || !errors.Is(failure, cause) || len(f.inputs.Reads) != beforeReads {
		t.Fatalf("current observation lost caller cancellation: %v reads%v", failure, f.inputs.Reads)
	}
	if _, err := mutants.Run(ctx, []string{f.path}, f.options); !errors.Is(err, cause) || len(f.snapshots.Written) != beforeWrites {
		t.Fatalf("cancelled caller published: %v writes%v", err, f.snapshots.Written)
	}
}
