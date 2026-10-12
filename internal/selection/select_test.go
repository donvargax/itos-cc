package selection_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"pgregory.net/rapid"
)

// dir is the working directory the trees are at: absolute, and holding no
// fragment the tests give, since a fragment is matched against the whole
// of a path.
var dir = func() string {
	abs, err := filepath.Abs(string(filepath.Separator) + "work")
	if err != nil {
		panic(err)
	}
	return abs
}()

// files are a tree's files, by slash-separated paths from dir, holding
// nothing.
func files(names ...string) map[string]string {
	held := map[string]string{}
	for _, name := range names {
		held[name] = ""
	}
	return held
}

// at is each of rels, slash-separated from dir, made absolute.
func at(rels ...string) []string {
	var paths []string
	for _, rel := range rels {
		paths = append(paths, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	return paths
}

// project is a small working tree: sources and tests in two languages'
// folders, one test only under a folder no source is in.
func project() porttest.Tree {
	return porttest.TreeOf(dir, files(
		"lib/cart/total.ts", "src/billing/invoice.ts", "src/billing/tax.py", "src/main.go",
		"spec/refund_test.go", "src/billing/invoice.test.ts", "src/main_test.go",
	), nil)
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
			want: selection.Files{
				Sources: at("lib/cart/total.ts", "src/billing/invoice.ts", "src/billing/tax.py", "src/main.go"),
				Tests:   at("spec/refund_test.go", "src/billing/invoice.test.ts", "src/main_test.go"),
			}},
		{name: "a directory and a file are roots", tree: project(), paths: []string{"src/billing", "lib/cart/total.ts"},
			want: selection.Files{Sources: at("lib/cart/total.ts", "src/billing/invoice.ts", "src/billing/tax.py"), Tests: at("src/billing/invoice.test.ts")}},
		{name: "a path that is not there is a fragment", tree: project(), paths: []string{"billing"},
			want: selection.Files{Sources: at("src/billing/invoice.ts", "src/billing/tax.py"), Tests: at("src/billing/invoice.test.ts")}},
		{name: "a fragment matching tests alone selects them", tree: project(), paths: []string{"refund"},
			want: selection.Files{Tests: at("spec/refund_test.go")}},
		{name: "fragments' files follow the roots' in their own order", tree: project(), paths: []string{"lib", "tax", "main"},
			want: selection.Files{Sources: at("lib/cart/total.ts", "src/billing/tax.py", "src/main.go"), Tests: at("src/main_test.go")}},
		{name: "a root and a fragment over the same file select it twice", tree: project(), paths: []string{"src/main.go", "main.go"},
			want: selection.Files{Sources: at("src/main.go", "src/main.go")}},
		{name: "changed files join the paths", tree: withChanged(project(), "src/main.go"), paths: []string{"lib"}, changed: true,
			want: selection.Files{Sources: at("lib/cart/total.ts", "src/main.go")}},
		{name: "changed alone is the changed files", tree: withChanged(project(), "src/billing/invoice.test.ts", "src/billing/tax.py"), changed: true,
			want: selection.Files{Sources: at("src/billing/tax.py"), Tests: at("src/billing/invoice.test.ts")}},
		{name: "nothing changed selects nothing, whatever the paths", tree: project(), paths: []string{"src", "nosuch"}, changed: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := selection.Select(t.Context(), c.tree, c.paths, c.changed, selection.Exclusion{})
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
	got, err := selection.Select(t.Context(), project(), []string{"billing", "nosuch", "absent"}, false, selection.Exclusion{})
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
	_, err := selection.Select(t.Context(), project(), []string{"billing", "bill"}, false, selection.Exclusion{})
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
			got, err := selection.Select(t.Context(), c.tree, c.paths, c.changed, selection.Exclusion{})
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
	if _, err := selection.Select(t.Context(), withChanged(project(), "src/main.go"), paths, true, selection.Exclusion{}); err != nil {
		t.Fatal(err)
	}
	if spare := paths[:cap(paths)]; spare[1] != "" {
		t.Errorf("Select() wrote %q into its caller's paths", spare[1])
	}
}

// Paths and fragments only ever narrow the working directory: what they
// select is exactly the files of "." under a path that is there or holding
// one that is not, and a fragment refused is one of the arguments that is
// not there. The paths are relative, as a path from the working directory
// is: one may climb above it, never start at the root of the disk.
func TestPathsAndFragmentsOnlyNarrowTheWorkingDirectory(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringMatching(`[ab]{1,2}(/[ab]{1,2}){0,2}\.(go|ts)`)
		sources := rapid.SliceOfDistinct(name, func(s string) string { return s }).Draw(t, "sources")
		tests := rapid.SliceOfDistinct(rapid.StringMatching(`[ab]{1,2}/[ab]{1,2}_test\.go`), func(s string) string { return s }).Draw(t, "tests")
		tree := porttest.TreeOf(dir, files(append(sources, tests...)...), nil)
		paths := rapid.SliceOfN(rapid.StringMatching(`[ab.][ab/.]{0,3}`), 1, 4).Draw(t, "paths")
		all, err := selection.Select(t.Context(), tree, nil, false, selection.Exclusion{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := selection.Select(t.Context(), tree, paths, false, selection.Exclusion{})
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
				root := filepath.Join(dir, p)
				if tree.Exists(t.Context(), p) && (f == root || strings.HasPrefix(f, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))) ||
					!tree.Exists(t.Context(), p) && strings.Contains(filepath.ToSlash(f), p) {
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

// fakes is the project with a package of fakes beside its sources, and a
// test of the fakes.
func fakes() porttest.Tree {
	return porttest.TreeOf(dir, files(
		"src/main.go", "src/port/porttest/fake.go", "src/port/porttest/fake_test.go", "src/main_test.go",
	), nil)
}

// porttests is the Exclusion of every porttest package, from dir.
var porttests = selection.Exclusion{Root: dir, Globs: []string{"**/porttest/**"}}

func TestExclusionLeavesSourcesOutHoweverTheyWereChosen(t *testing.T) {
	for _, c := range []struct {
		name    string
		tree    porttest.Tree
		paths   []string
		changed bool
		want    selection.Files
	}{
		{name: "no paths", tree: fakes(),
			want: selection.Files{Sources: at("src/main.go"), Tests: at("src/main_test.go", "src/port/porttest/fake_test.go"), Excluded: 1}},
		{name: "a root holding them", tree: fakes(), paths: []string{"src/port"},
			want: selection.Files{Tests: at("src/port/porttest/fake_test.go"), Excluded: 1}},
		{name: "a root that is one", tree: fakes(), paths: []string{"src/port/porttest/fake.go"},
			want: selection.Files{Excluded: 1}},
		{name: "a fragment only they and their tests hold", tree: fakes(), paths: []string{"porttest"},
			want: selection.Files{Tests: at("src/port/porttest/fake_test.go"), Excluded: 1}},
		{name: "a fragment only one holds is matched all the same", tree: fakes(), paths: []string{"fake.go"},
			want: selection.Files{Excluded: 1}},
		{name: "changed naming one", tree: withChanged(fakes(), "src/port/porttest/fake.go"), changed: true,
			want: selection.Files{Excluded: 1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := selection.Select(t.Context(), c.tree, c.paths, c.changed, porttests)
			if err != nil {
				t.Fatalf("Select() failed: %v", err)
			}
			slices.Sort(got.Tests)
			if !slices.Equal(got.Sources, c.want.Sources) || !slices.Equal(got.Tests, c.want.Tests) || got.Excluded != c.want.Excluded {
				t.Errorf("Select(%q, changed %t) = %+v, want %+v", c.paths, c.changed, got, c.want)
			}
		})
	}
}

func TestExclusionMatchesSlashPathsFromItsRoot(t *testing.T) {
	for _, c := range []struct {
		glob string
		rel  string
		want bool
	}{
		{"**/porttest/**", "internal/a/port/porttest/fake.go", true},
		{"**/porttest/**", "porttest/fake.go", true},
		{"**/porttest/**", "internal/porttests/fake.go", false},
		{"features/**", "features/steps.go", true},
		{"features/**", "src/features/steps.go", false},
		{"src/*.go", "src/a.go", true},
		{"src/*.go", "src/a/b.go", false},
		{"src/?.go", "src/a.go", true},
		{"src/[ab].go", "src/c.go", false},
		{"{lib,src}/gen/*.ts", "lib/gen/x.ts", true},
	} {
		e := selection.Exclusion{Root: dir, Globs: []string{c.glob}}
		if got := e.Excludes(filepath.Join(dir, filepath.FromSlash(c.rel))); got != c.want {
			t.Errorf("%q excludes %s: %t, want %t", c.glob, c.rel, got, c.want)
		}
	}
}

func TestExclusionNeverReachesAboveItsRoot(t *testing.T) {
	e := selection.Exclusion{Root: filepath.Join(dir, "project"), Globs: []string{"**"}}
	if e.Excludes(filepath.Join(dir, "other", "a.go")) {
		t.Errorf("%v excludes a file outside its root", e)
	}
	if !e.Excludes(filepath.Join(dir, "project", "a.go")) {
		t.Errorf("%v excludes nothing under its root", e)
	}
	if (selection.Exclusion{}).Excludes(filepath.Join(dir, "a.go")) {
		t.Error("the zero Exclusion excludes a file")
	}
}

// Exclusion only ever takes sources away: what a selection with one gives
// is what it gives without, less the sources the globs match, and the
// count of those; the tests are the same.
func TestExclusionOnlyTakesSourcesAway(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringMatching(`[ab]{1,2}(/[ab]{1,2}){0,2}\.(go|ts)`)
		sources := rapid.SliceOfDistinct(name, func(s string) string { return s }).Draw(t, "sources")
		tests := rapid.SliceOfDistinct(rapid.StringMatching(`[ab]{1,2}/[ab]{1,2}_test\.go`), func(s string) string { return s }).Draw(t, "tests")
		tree := porttest.TreeOf(dir, files(append(sources, tests...)...), nil)
		globs := rapid.SliceOfN(rapid.SampledFrom([]string{"a/**", "**/b/**", "*.go", "**/*.ts", "b?/*", "{a,b}/a.go", "**/*_test.go"}), 0, 3).Draw(t, "globs")
		exclude := selection.Exclusion{Root: dir, Globs: globs}
		whole, err := selection.Select(t.Context(), tree, nil, false, selection.Exclusion{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := selection.Select(t.Context(), tree, nil, false, exclude)
		if err != nil {
			t.Fatal(err)
		}
		want := slices.DeleteFunc(slices.Clone(whole.Sources), exclude.Excludes)
		if !slices.Equal(got.Sources, want) || !slices.Equal(got.Tests, whole.Tests) || got.Excluded != len(whole.Sources)-len(want) {
			t.Fatalf("Select excluding %q = %+v, want sources %q, tests %q, %d excluded", globs, got, want, whole.Tests, len(whole.Sources)-len(want))
		}
	})
}
