package mutate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func TestListedFinalizationRetainsActualHashesAfterCallerCancellation(t *testing.T) {
	root := t.TempDir()
	text := "package board\n// actual retained test bytes\n"
	path := filepath.Join(root, "board_test.go")
	inputs := &porttest.Inputs{Dir: root, Files: map[string]string{path: text}}
	snapshot := Snapshot{Listed: map[string]string{"actual": "board_test.go", "missing": "missing_test.go"}, Units: []UnitResult{{Mutants: []Mutant{{Scope: ScopeListed, Tests: []string{"actual", "missing"}}}}}}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("parent stopped")
	cancel(cause)
	recordListedWithInputs(ctx, &snapshot, root, map[string]string{"fixture.feature": "support-hash"}, inputs)
	sum := sha256.Sum256([]byte(text))
	want := map[string]string{"board_test.go": hex.EncodeToString(sum[:]), "missing_test.go": ""}
	if !reflect.DeepEqual(snapshot.ListedFiles, want) || snapshot.Support["fixture.feature"] != "support-hash" {
		t.Fatalf("actual retained listed inputs: %v %v", snapshot.ListedFiles, snapshot.Support)
	}
	if !reflect.DeepEqual(inputs.Reads, []string{path}) {
		t.Fatalf("retained hash did not read actual memory bytes: %v", inputs.Reads)
	}
	before := len(inputs.Reads)
	if current := listedHash(ctx, root, "board_test.go", inputs); current != "" || len(inputs.Reads) != before {
		t.Fatalf("current freshness ignored caller cancellation: %s %v", current, inputs.Reads)
	}
	_, failure := inputs.Tests(ctx, root, []string{path})
	var stopped mutants.InputCancelled
	if !errors.As(failure, &stopped) || !errors.Is(failure, cause) {
		t.Fatalf("ordinary current inputs lost caller cause: %v", failure)
	}
}
