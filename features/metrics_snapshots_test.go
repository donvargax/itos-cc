package features

import (
	"path/filepath"
	"strings"
	"testing"
)

// The outline has four different expanded assertion names. These portable
// wrong-fact guards cover its shared exit predicate without claiming a native
// product failure or changing the runner's per-example identity contract.
func TestSnapshotAssertionsRejectFailedCommandsWithUnchangedArtifact(t *testing.T) {
	d := binary{}
	w := &world{d: d, dir: t.TempDir()}
	q := &snapshotState{w: w, source: "board.go", snapshot: ".metrics/crap.json"}
	data := []byte("{\"version\":1,\"entries\":[]}\n")
	if err := d.Write(filepath.Join(w.dir, q.snapshot), data); err != nil {
		t.Fatal(err)
	}
	q.before = data
	if err := q.written(q.snapshot); err != nil {
		t.Fatalf("successful artifact: %v", err)
	}
	if err := q.sameBytes(); err != nil {
		t.Fatalf("successful unchanged artifact: %v", err)
	}
	w.exit = 1
	if err := q.written(q.snapshot); err == nil || !strings.Contains(err.Error(), "snapshot command exit") {
		t.Fatalf("failed writer accepted: %v", err)
	}
	if err := q.sameBytes(); err == nil || !strings.Contains(err.Error(), "unchanged rerun exit") {
		t.Fatalf("failed rerun accepted stale artifact: %v", err)
	}
}
