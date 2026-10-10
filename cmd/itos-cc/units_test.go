package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/lang"
)

// The scenarios of features/units.feature that run itos-cc units.

// rootMarkers are the files that make a directory a project root.
var rootMarkers = []string{"package.json", "tsconfig.json", "go.mod", ".git"}

// unitIDs is the namespace#name of each unit itos-cc units --json lists for
// args, run in the test's directory (useDir).
func unitIDs(t *testing.T, args ...string) []string {
	t.Helper()
	o := cli(t, append([]string{"units", "--json"}, args...)...)
	var got struct {
		Units []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"units"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &got); err != nil || o.code != 0 {
		t.Fatalf("exit %d, stdout is not one JSON object: %v\n%s\nstderr:\n%s", o.code, err, o.stdout, o.stderr)
	}
	var ids []string
	for _, u := range got.Units {
		ids = append(ids, u.Namespace+"#"+u.Name)
	}
	return ids
}

// @ID-UNIT-13
func TestAFileOutsideAnyProjectRootIsNamedFromTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	// Two directories at different absolute paths, each with the same x.ts
	// and nothing that makes it a project.
	for i, dir := range []string{t.TempDir(), t.TempDir()} {
		writeFile(t, filepath.Join(dir, "x.ts"), "export function place(a: number): boolean {\n  return a > 0;\n}\n")
		if root := lang.FindUp(filepath.Join(dir, "x.ts"), rootMarkers...); root != "" {
			t.Skipf("%s holds a project marker, so %s is not outside every project root", root, dir)
		}
		useDir(t, dir)
		if got := unitIDs(t, "x.ts"); len(got) != 1 || got[0] != "x#place" {
			t.Errorf("directory %d, %s: units %q, want [x#place]", i+1, dir, got)
		}
	}
}
