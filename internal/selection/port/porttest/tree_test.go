package porttest

import (
	"slices"
	"testing"
)

// The fake walks as a real tree does: a root is "." or a path at or above
// files, each list comes sorted with each file once, and what Changed
// returns is the caller's own.
func TestTreeWalksAsARealTreeDoes(t *testing.T) {
	tree := Tree{Sources: []string{"src/b.go", "src/a.go", "srcs/c.go"}, Tests: []string{"src/a_test.go"}, ChangedFiles: []string{"src/a.go"}}
	sources, tests, failure := tree.Discover(t.Context(), []string{"src/", "src/a.go"})
	if failure != nil || !slices.Equal(sources, []string{"src/a.go", "src/b.go"}) || !slices.Equal(tests, []string{"src/a_test.go"}) {
		t.Errorf("Discover(src/, src/a.go) = %q, %q, %v", sources, tests, failure)
	}
	for path, want := range map[string]bool{".": true, "src": true, "src/a_test.go": true, "sr": false, "src/a": false} {
		if got := tree.Exists(t.Context(), path); got != want {
			t.Errorf("Exists(%q) = %t, want %t", path, got, want)
		}
	}
	changed, _ := tree.Changed(t.Context())
	changed[0] = "other"
	if tree.ChangedFiles[0] != "src/a.go" {
		t.Errorf("Changed() exposed the fake's own list")
	}
}
