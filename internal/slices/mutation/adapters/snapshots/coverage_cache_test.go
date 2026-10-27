package snapshots_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// The cached listed tests' coverage reads back as it was written, from
// .metrics/coverage/cache/listed.json on the disk; an entry that does not
// decode, or of another version, cannot be read, and none is no entry.
func TestTheListedCoverageEntryReadsBackAsItWasWritten(t *testing.T) {
	root := t.TempDir()
	cache := snapshots.CoverageCache{Store: metricsdir.Store{RootDir: root}}
	if entry, failure := cache.Listed(t.Context()); entry != nil || failure != nil {
		t.Fatalf("no entry read as %+v, %v", entry, failure)
	}
	want := mutants.ListedCoverage{
		Inputs:      map[string]string{"main.go": "h1", "@go-env": "h2"},
		Listed:      []mutants.CachedListing{{ID: "ID-A-01", File: "a.feature", Fingerprint: "fp"}, {ID: "ID-A-02"}},
		ListedFiles: map[string]string{"a.feature": "h3"},
		Measured:    []string{"main.go"},
		Tests: []mutants.CachedTest{{ID: "ID-A-01", Files: map[string][]mutants.CachedPiece{
			"main.go": {{Start: 9, End: 10, Total: 2, Covered: 2, Key: "9.2,10.3", Ran: true, InProcess: true, Integration: true},
				{Start: 12, End: 12, Total: 2, Covered: 1, Branch: true}},
			"b/b.go": {},
		}}},
	}
	if failure := cache.SaveListed(t.Context(), want); failure != nil {
		t.Fatal(failure)
	}
	got, failure := cache.Listed(t.Context())
	if failure != nil || got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("read back %+v, %v; want %+v", got, failure, want)
	}
	path := filepath.Join(root, ".metrics", filepath.FromSlash(snapshots.ListedCoverageName))
	for _, text := range []string{"not json", `{"version": 2}`} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		entry, failure := cache.Listed(t.Context())
		if _, unreadable := failure.(*snapport.Unreadable); entry != nil || !unreadable {
			t.Errorf("%q read as %+v, %v; want unreadable", text, entry, failure)
		}
	}
}
