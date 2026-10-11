package selection_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"pgregory.net/rapid"
)

// project is a small working tree: sources and tests in two languages'
// folders, one test only under a folder no source is in.
func project() porttest.Tree {
	return porttest.Tree{
		Sources: []string{"lib/cart/total.ts", "src/billing/invoice.ts", "src/billing/tax.py", "src/main.go"},
		Tests:   []string{"spec/refund_test.go", "src/billing/invoice.test.ts", "src/main_test.go"},
	}
}

func TestSelectChoosesItsFiles(t *testing.T) {
	for _, c := range []struct {
		name    string
		tree    porttest.Tree
		paths   []string
		changed bool
		want    selection.Files
	}{
		{name: "no paths is the whole working directory", tree: project(),
			want: selection.Files{Sources: project().Sources, Tests: project().Tests}},
		{name: "a directory and a file are roots", tree: project(), paths: []string{"src/billing", "lib/cart/total.ts"},
			want: selection.Files{Sources: []string{"lib/cart/total.ts", "src/billing/invoice.ts", "src/billing/tax.py"}, Tests: []string{"src/billing/invoice.test.ts"}}},
		{name: "a path that is not there is a fragment", tree: project(), paths: []string{"billing"},
			want: selection.Files{Sources: []string{"src/billing/invoice.ts", "src/billing/tax.py"}, Tests: []string{"src/billing/invoice.test.ts"}}},
		{name: "a fragment matching tests alone selects them", tree: project(), paths: []string{"refund"},
			want: selection.Files{Tests: []string{"spec/refund_test.go"}}},
		{name: "fragments' files follow the roots' in their own order", tree: project(), paths: []string{"lib", "tax", "main"},
			want: selection.Files{Sources: []string{"lib/cart/total.ts", "src/billing/tax.py", "src/main.go"}, Tests: []string{"src/main_test.go"}}},
		{name: "a root and a fragment over the same file select it twice", tree: project(), paths: []string{"src/main.go", "main.go"},
			want: selection.Files{Sources: []string{"src/main.go", "src/main.go"}}},
		{name: "changed files join the paths", tree: withChanged(project(), "src/main.go"), paths: []string{"lib"}, changed: true,
			want: selection.Files{Sources: []string{"lib/cart/total.ts", "src/main.go"}}},
		{name: "changed alone is the changed files", tree: withChanged(project(), "src/billing/invoice.test.ts", "src/billing/tax.py"), changed: true,
			want: selection.Files{Sources: []string{"src/billing/tax.py"}, Tests: []string{"src/billing/invoice.test.ts"}}},
		{name: "nothing changed selects nothing, whatever the paths", tree: project(), paths: []string{"src", "nosuch"}, changed: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := selection.Select(t.Context(), c.tree, c.paths, c.changed)
			if err != nil {
				t.Fatalf("Select() failed: %v", err)
			}
			if !slices.Equal(got.Sources, c.want.Sources) || !slices.Equal(got.Tests, c.want.Tests) {
				t.Errorf("Select(%q, changed %t) = %+v, want %+v", c.paths, c.changed, got, c.want)
			}
		})
	}
}

func withChanged(tree porttest.Tree, changed ...string) porttest.Tree {
	tree.ChangedFiles = changed
	return tree
}

func TestSelectRefusesTheFirstFragmentNoFileContains(t *testing.T) {
	got, err := selection.Select(t.Context(), project(), []string{"billing", "nosuch", "absent"}, false)
	var unmatched *port.Unmatched
	if !errors.As(err, &unmatched) || unmatched.Fragment != "nosuch" {
		t.Fatalf("Select() failure = %T %v, want Unmatched naming nosuch", err, err)
	}
	if got.Sources != nil || got.Tests != nil {
		t.Errorf("Select() selected %+v on a failure, want nothing", got)
	}
	if unmatched.Error() != "no file matches nosuch" {
		t.Errorf("Unmatched says %q", unmatched.Error())
	}
}

// A file is taken by the first fragment it contains, so a fragment whose
// every file an earlier one took is unmatched: the rule as the command line
// has it, held so that changing it is a decision.
func TestAFragmentWhoseFilesAnEarlierOneTookIsUnmatched(t *testing.T) {
	_, err := selection.Select(t.Context(), project(), []string{"billing", "bill"}, false)
	var unmatched *port.Unmatched
	if !errors.As(err, &unmatched) || unmatched.Fragment != "bill" {
		t.Fatalf("Select() failure = %T %v, want Unmatched naming bill", err, err)
	}
}

func TestSelectPassesTheTreesFailuresOn(t *testing.T) {
	noGit := &port.NoGit{Reason: "exit status 128"}
	walk := &port.Unwalkable{Reason: "open src: permission denied"}
	cancelled := &port.Cancelled{}
	for _, c := range []struct {
		name    string
		tree    porttest.Tree
		paths   []string
		changed bool
		want    port.Failure
	}{
		{name: "changed with no git", tree: porttest.Tree{NoGit: noGit}, paths: []string{"src"}, changed: true, want: noGit},
		{name: "the whole working directory", tree: failing(".", walk), want: walk},
		{name: "a root", tree: failing("src", walk), paths: []string{"src", "billing"}, want: walk},
		{name: "the working directory under a fragment", tree: failing(".", cancelled), paths: []string{"src", "billing"}, want: cancelled},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := selection.Select(t.Context(), c.tree, c.paths, c.changed)
			if err != c.want {
				t.Fatalf("Select() failure = %T %v, want %T %v", err, err, c.want, c.want)
			}
			if got.Sources != nil || got.Tests != nil {
				t.Errorf("Select() selected %+v on a failure, want nothing", got)
			}
		})
	}
	if noGit.Error() != "no git repository: exit status 128" || walk.Error() != "open src: permission denied" || cancelled.Error() != "cancelled selection" {
		t.Errorf("failures say %q, %q, %q", noGit.Error(), walk.Error(), cancelled.Error())
	}
}

func failing(root string, failure port.Failure) porttest.Tree {
	tree := project()
	tree.Failures = map[string]port.Failure{root: failure}
	return tree
}

func TestSelectLeavesItsCallersPathsAlone(t *testing.T) {
	paths := make([]string, 1, 4)
	paths[0] = "lib"
	if _, err := selection.Select(t.Context(), withChanged(project(), "src/main.go"), paths, true); err != nil {
		t.Fatal(err)
	}
	if spare := paths[:cap(paths)]; spare[1] != "" {
		t.Errorf("Select() wrote %q into its caller's paths", spare[1])
	}
}

// Paths and fragments only ever narrow the working directory: what they
// select is exactly the files of "." under a path that is there or holding
// one that is not, and a fragment refused is one of the arguments that is
// not there.
func TestPathsAndFragmentsOnlyNarrowTheWorkingDirectory(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringMatching(`[ab]{1,2}(/[ab]{1,2}){0,2}\.(go|ts)`)
		tree := porttest.Tree{
			Sources: rapid.SliceOfDistinct(name, func(s string) string { return s }).Draw(t, "sources"),
			Tests:   rapid.SliceOfDistinct(rapid.StringMatching(`[ab]{1,2}/[ab]{1,2}_test\.go`), func(s string) string { return s }).Draw(t, "tests"),
		}
		paths := rapid.SliceOfN(rapid.StringMatching(`[ab/.]{1,4}`), 1, 4).Draw(t, "paths")
		all, err := selection.Select(t.Context(), tree, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		got, err := selection.Select(t.Context(), tree, paths, false)
		var unmatched *port.Unmatched
		if errors.As(err, &unmatched) {
			if !slices.Contains(paths, unmatched.Fragment) || tree.Exists(t.Context(), unmatched.Fragment) {
				t.Fatalf("refused %q, not an argument that is not there among %q", unmatched.Fragment, paths)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		chosen := func(f string) bool {
			for _, p := range paths {
				if tree.Exists(t.Context(), p) && (p == "." || f == p || strings.HasPrefix(f, strings.TrimSuffix(p, "/")+"/")) ||
					!tree.Exists(t.Context(), p) && strings.Contains(f, p) {
					return true
				}
			}
			return false
		}
		for _, list := range [][2][]string{{all.Sources, got.Sources}, {all.Tests, got.Tests}} {
			want := slices.DeleteFunc(slices.Clone(list[0]), func(f string) bool { return !chosen(f) })
			if have := slices.Compact(slices.Sorted(slices.Values(list[1]))); !slices.Equal(have, want) {
				t.Fatalf("Select(%q) = %q, want the files of . it names, %q", paths, have, want)
			}
		}
	})
}
