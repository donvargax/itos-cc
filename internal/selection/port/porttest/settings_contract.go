package porttest

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// SettingsContract observes current selection settings, independent
// ownership and replacement. open gives the reader over settings whose
// exclude is generated/** alone, at the root it returns, and replace,
// which changes the actual backing settings to exclude replaced/** alone,
// not a call sequence. Codec validity and raw I/O failures remain
// adapter-owned tests.
func SettingsContract(t *testing.T, open func(*testing.T) (port.SettingsReader, string, func())) {
	t.Helper()
	reader, root, replace := open(t)
	first, failure := reader.ReadSelection(t.Context())
	if failure != nil {
		t.Fatal(failure)
	}
	second, failure := reader.ReadSelection(t.Context())
	if failure != nil || second.Root != root || len(second.Exclude) != 1 || second.Exclude[0] != "generated/**" {
		t.Fatalf("current selection settings: %+v %v; want %s and generated/**", second, failure, root)
	}
	first.Exclude[0] = "changed returned slice"
	if second.Exclude[0] != "generated/**" {
		t.Fatal("reads share mutable selection settings")
	}
	replace()
	current, failure := reader.ReadSelection(t.Context())
	if failure != nil || len(current.Exclude) != 1 || current.Exclude[0] != "replaced/**" {
		t.Fatalf("backing replacement not reread: %+v %v", current, failure)
	}
}
