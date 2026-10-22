package worktrees

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// nodeTree writes files, by slash-separated path, under a new folder,
// and returns it.
func nodeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	tree, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		path := filepath.Join(tree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return tree
}

// TestNodeWorkspaceIsTheTreeAMemberNeeds holds which tree a worker copies
// for a TypeScript package (slice-19, issue #44): its workspace's root,
// when a pnpm-workspace.yaml's packages or a package.json's workspaces
// above it and within the project root match it, else its own.
func TestNodeWorkspaceIsTheTreeAMemberNeeds(t *testing.T) {
	member := map[string]string{"packages/shapes/package.json": "{}\n", "tsconfig.base.json": "{}\n"}
	with := func(files map[string]string) map[string]string {
		out := map[string]string{}
		for _, m := range []map[string]string{member, files} {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	for name, tc := range map[string]struct {
		files     map[string]string
		workspace bool
	}{
		"pnpm":                     {with(map[string]string{"pnpm-workspace.yaml": "packages:\n  - \"packages/*\"\n"}), true},
		"pnpm with ./ and **":      {with(map[string]string{"pnpm-workspace.yaml": "packages:\n  - ./packages/**\n"}), true},
		"pnpm settings alone":      {with(map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 1440\n"}), false},
		"pnpm excluding it":        {with(map[string]string{"pnpm-workspace.yaml": "packages:\n  - packages/*\n  - \"!packages/shapes\"\n"}), false},
		"npm":                      {with(map[string]string{"package.json": `{"workspaces": ["packages/*"]}`}), true},
		"yarn's packages":          {with(map[string]string{"package.json": `{"workspaces": {"packages": ["packages/*"]}}`}), true},
		"a workspace of others":    {with(map[string]string{"package.json": `{"workspaces": ["apps/*"]}`}), false},
		"a package.json, no field": {with(map[string]string{"package.json": `{"name": "root"}`}), false},
		"no workspace":             {member, false},
	} {
		t.Run(name, func(t *testing.T) {
			tree := nodeTree(t, tc.files)
			pkg := filepath.Join(tree, "packages", "shapes")
			want := pkg
			if tc.workspace {
				want = tree
			}
			if got := nodeWorkspace(pkg, tree); got != want {
				t.Errorf("the tree is %s, want %s", got, want)
			}
		})
	}
	t.Run("a workspace above the project root is not copied", func(t *testing.T) {
		tree := nodeTree(t, with(map[string]string{"pnpm-workspace.yaml": "packages:\n  - packages/*\n"}))
		pkg := filepath.Join(tree, "packages", "shapes")
		if got := nodeWorkspace(pkg, filepath.Join(tree, "packages")); got != pkg {
			t.Errorf("the tree is %s, want the package's own", got)
		}
	})
	t.Run("a folder with no package.json is its own tree", func(t *testing.T) {
		tree := nodeTree(t, map[string]string{"pnpm-workspace.yaml": "packages:\n  - packages/*\n", "packages/gomod/go.mod": "module m\n"})
		pkg := filepath.Join(tree, "packages", "gomod")
		if got := nodeWorkspace(pkg, tree); got != pkg {
			t.Errorf("the tree is %s, want the folder's own", got)
		}
	})
}

// TestMembersShareACopyOfTheirWorkspace holds that each worker copies a
// workspace member's whole workspace root once, its node_modules linked,
// and runs each member in its place there.
func TestMembersShareACopyOfTheirWorkspace(t *testing.T) {
	tree := nodeTree(t, map[string]string{
		"pnpm-workspace.yaml":       "packages:\n  - packages/*\n",
		"package.json":              "{}\n",
		"tsconfig.base.json":        "{}\n",
		"node_modules/dep/index.js": "\n",
		"packages/a/package.json":   "{}\n",
		"packages/a/tsconfig.json":  `{"extends": "../../tsconfig.base.json"}`,
		"packages/b/package.json":   "{}\n",
		"packages/b/dist/out.js":    "\n",
	})
	w := &worker{dir: t.TempDir(), copies: map[string]*workerCopy{}, project: tree}
	var places []string
	for _, member := range []string{"a", "b"} {
		root := filepath.Join(tree, "packages", member)
		place, err := w.copyOf(t.Context(), mutants.Command{Root: root, Dir: root})
		if err != nil {
			t.Fatal(err)
		}
		places = append(places, place)
	}
	if len(w.copies) != 1 {
		t.Fatalf("the worker made %d copies, want one of the workspace", len(w.copies))
	}
	dir := w.copies[tree].dir
	for i, member := range []string{"a", "b"} {
		if want := filepath.Join(dir, "packages", member); places[i] != want {
			t.Errorf("%s runs in %s, want %s", member, places[i], want)
		}
	}
	for name, held := range map[string]bool{"tsconfig.base.json": true, "pnpm-workspace.yaml": true, "packages/a/tsconfig.json": true, "packages/b/dist": false} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); (err == nil) != held {
			t.Errorf("the copy holds %s: %v, want %v", name, err == nil, held)
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, "node_modules")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the copy's node_modules is not a link: %v", err)
	}
}

// TestInProjectRunsInTheProjectsOwnFolder holds that a run in the project
// runs the command in its own folder, no copy, and names the tree a copy
// holds.
func TestInProjectRunsInTheProjectsOwnFolder(t *testing.T) {
	tree := nodeTree(t, map[string]string{
		"package.json":            `{"workspaces": ["packages/*"]}`,
		"packages/a/package.json": "{}\n",
	})
	t.Setenv(runnerFixtureMode, "setup")
	opened, failure := (Workers{Platform: runtime.GOOS, Dir: tree}).Open(t.Context(), mutants.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	p := opened.(*pool)
	t.Cleanup(func() {
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	})
	root := filepath.Join(tree, "packages", "a")
	run, failure := p.InProject(t.Context(), mutants.Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()})
	if failure != nil || !run.Result.Passed {
		t.Fatalf("the run in the project: %+v %v", run, failure)
	}
	if run.Copied != tree {
		t.Errorf("the tree a copy holds is %s, want the workspace's %s", run.Copied, tree)
	}
	cwd := ""
	for _, line := range strings.Split(run.Result.Output, "\n") {
		if strings.HasPrefix(line, "cwd=") {
			cwd = strings.TrimPrefix(line, "cwd=")
		}
	}
	if got, err := filepath.EvalSymlinks(cwd); err != nil || got != root {
		t.Errorf("it ran in %q, want the project's %s (%v)", cwd, root, err)
	}
	if len(p.workers[0].worker.copies) != 0 {
		t.Errorf("a run in the project made a copy")
	}
}
