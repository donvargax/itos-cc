package porttest

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// SettingsContract observes current semantic values, independent ownership and
// replacement. replace changes actual backing settings, not a call sequence.
// Codec validity and raw I/O failures remain adapter-owned tests.
func SettingsContract(t *testing.T, open func(*testing.T) (mutants.SettingsReader, string, func())) {
	t.Helper()
	reader, root, replace := open(t)
	_, first, failure := reader.Read(t.Context())
	if failure != nil {
		t.Fatal(failure)
	}
	gotRoot, second, failure := reader.Read(t.Context())
	if failure != nil || gotRoot != root || len(second.Exclude) != 1 || second.Exclude[0] != "generated/**" || len(second.Exceptions) != 1 || second.Exceptions[0].Reason != "independent reason" || second.Tests == nil || len(second.Tests.Support) != 1 || second.Tests.Support[0] != "support.txt" {
		t.Fatalf("current semantic settings: %s %+v %v", gotRoot, second, failure)
	}
	first.Exclude[0] = "changed returned slice"
	first.Exceptions[0].Reason = "changed returned exception"
	first.Tests.Support[0] = "changed returned support"
	if second.Exclude[0] != "generated/**" || second.Exceptions[0].Reason != "independent reason" || second.Tests.Support[0] != "support.txt" {
		t.Fatal("reads share mutable semantic settings")
	}
	replace()
	_, current, failure := reader.Read(t.Context())
	if failure != nil || len(current.Exclude) != 1 || current.Exclude[0] != "replaced/**" || current.Tests != nil || len(current.Exceptions) != 0 {
		t.Fatalf("backing replacement not reread: %+v %v", current, failure)
	}
}
