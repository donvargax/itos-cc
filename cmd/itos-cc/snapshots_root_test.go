package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenarios of snapshots-at-root in features/metrics_snapshots.feature.
// Each runs in the module of mutate_since_test.go, whose git repository
// holds src/board.go and its tests, from src/ or from the root.

// inSrc makes the module's src/ the test's directory (useDir).
func inSrc(t *testing.T, root string) {
	t.Helper()
	useDir(t, filepath.Join(root, "src"))
}

// snapshotFiles is every "file" a snapshot at path records, at any depth.
func snapshotFiles(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no snapshot at %s: %v", path, err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var files []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if s, ok := e.(string); ok && k == "file" {
					files = append(files, s)
				}
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return files
}

func noMetricsIn(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".metrics")); err == nil {
		t.Errorf("a .metrics directory was created in %s", dir)
	}
}

// @ID-SNAP-08
func TestSnapshotsLiveAtTheProjectRootWhereverACommandRuns(t *testing.T) {
	t.Parallel()
	root := boardRepo(t, nil)
	inSrc(t, root)
	if o := mutateRun(t, "board.go"); o.code != 1 {
		t.Fatalf("exit %d, want 1 for clear's survivor; stdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}
	snapshot := filepath.Join(root, ".metrics", "mutate", "src", "board.go.json")
	if got := snapshotFiles(t, snapshot); len(got) != 1 || got[0] != "src/board.go" {
		t.Errorf("%s names %q, want \"src/board.go\"", snapshot, got)
	}
	noMetricsIn(t, filepath.Join(root, "src"))
}

// @ID-SNAP-09
func TestEveryCommandsSnapshotNamesFilesFromTheRoot(t *testing.T) {
	t.Parallel()
	// scrap.json records test files alone, so it names src/board.go's test.
	for _, c := range []struct {
		args     []string
		snapshot string
		file     string
	}{
		{[]string{"crap", "--no-coverage"}, "crap.json", "src/board.go"},
		{[]string{"dry", "--min-lines", "1", "--min-nodes", "1", "--threshold", "0.1"}, "dry.json", "src/board.go"},
		{[]string{"scrap"}, "scrap.json", "src/board_test.go"},
	} {
		t.Run(c.args[0], func(t *testing.T) {
			root := boardRepo(t, nil)
			inSrc(t, root)
			if o := cli(t, c.args...); o.code != 0 {
				t.Fatalf("itos-cc %v: exit %d, stderr:\n%s", c.args, o.code, o.stderr)
			}
			got := snapshotFiles(t, filepath.Join(root, ".metrics", c.snapshot))
			found := false
			for _, f := range got {
				if f == c.file {
					found = true
				} else if !strings.HasPrefix(f, "src/") {
					t.Errorf("%s names %q, want every file from the root", c.snapshot, f)
				}
			}
			if !found {
				t.Errorf("%s at the root names %q, want %q among them", c.snapshot, got, c.file)
			}
			noMetricsIn(t, filepath.Join(root, "src"))
		})
	}
}

// @ID-SNAP-10
func TestACommandFindsTheSameResultsFromAnyDirectory(t *testing.T) {
	t.Parallel()
	root := boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("mutation run from the root: exit %d, want every mutant killed; stdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}
	inSrc(t, root)
	if o := mutationCheck(t, "board.go"); o.code != 0 {
		t.Errorf("mutation check board.go from src/: exit %d, want 0; stdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}
	useDir(t, root)
	if o := mutationCheck(t, boardSource); o.code != 0 {
		t.Errorf("mutation check %s from the root: exit %d, want 0; stdout:\n%s\nstderr:\n%s", boardSource, o.code, o.stdout, o.stderr)
	}
}

// @ID-SNAP-11
func TestPathsOnTheCommandLineAndInOutputStayRelativeToTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	root := boardRepo(t, nil)
	inSrc(t, root)
	if o := mutateRun(t, "board.go"); !strings.HasPrefix(o.stdout, "board.go: ") {
		t.Errorf("stdout's summary:\n%s\nwant it to name the file \"board.go\"; stderr:\n%s", o.stdout, o.stderr)
	}
	files := mutateRun(t, "--json", "board.go").json(t).Files
	if len(files) != 1 || files[0].File != "board.go" {
		t.Errorf("files %+v, want one, \"board.go\"", files)
	}
}

// @ID-SNAP-12
func TestOutsideAGitRepositoryTheWorkingDirectoryIsTheRoot(t *testing.T) {
	t.Parallel()
	dir := inEmptyDir(t)
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/m\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "board.go"), boardFiles["src/board.go"])
	if o := cli(t, "crap", "--no-coverage"); o.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", o.code, o.stderr)
	}
	if got := snapshotFiles(t, filepath.Join(dir, ".metrics", "crap.json")); len(got) == 0 || got[0] != "board.go" {
		t.Errorf("crap.json names %q, want \"board.go\"", got)
	}
}
