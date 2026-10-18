package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// run is the seam the in-process driver runs itos-cc through (decision 44):
// these run units through it against porttest's fakes and an itos-cc.yaml
// held in memory, touching no disk, as the binary's scenarios run it
// against internal/workdir, internal/metricsdir and the file on the disk.

// heldConfig is itos-cc.yaml held in memory at root: its text, or none when
// text is nil.
type heldConfig struct {
	root string
	text []byte
}

func (c heldConfig) Read(context.Context) (string, []byte, error) {
	return c.root, c.text, nil
}

// fakes are adapters over a working tree at work, held in memory, of a
// TypeScript source and its test and a Python source, with config as its
// itos-cc.yaml.
func fakes(config []byte) adapters {
	work := filepath.FromSlash("/work")
	return adapters{
		Dir: work,
		Tree: selporttest.TreeOf(work, map[string]string{
			"src/cart.ts":      "export function total(p: number[]): number {\n  return p.length;\n}\n",
			"src/price.py":     "def discount(p):\n    return p\n",
			"src/cart.test.ts": "function cartOf(n: number) {\n  return n;\n}\n",
		}, nil),
		Surroundings: langporttest.Surroundings{Dir: "/work"},
		Store:        &snapporttest.Store{RootDir: work},
		Config:       heldConfig{root: work, text: config},
	}
}

// runFaked runs itos-cc with args through run against a, and gives its exit
// code, stdout and stderr.
func runFaked(t *testing.T, a adapters, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, strings.NewReader(""), &stdout, &stderr, a)
	return code, stdout.String(), stderr.String()
}

// units lists each unit of the fake tree's sources, named from its working
// directory, as UNIT-01 reads them from the binary.
func TestRunWithFakesListsUnits(t *testing.T) {
	code, stdout, stderr := runFaked(t, fakes(nil), "units")
	want := []string{
		filepath.FromSlash("src/cart.ts") + ":1-3  cart#total  function",
		filepath.FromSlash("src/price.py") + ":1-2  price#discount  function",
	}
	if got := strings.Split(strings.TrimSpace(stdout), "\n"); code != 0 || !slices.Equal(got, want) || stderr != "" {
		t.Errorf("units: exit %d, stdout %q, stderr %q; want 0, %q and nothing", code, got, stderr, want)
	}
}

// --json is one object, its units' files named from the working directory.
func TestRunWithFakesListsUnitsAsJSON(t *testing.T) {
	code, stdout, _ := runFaked(t, fakes(nil), "units", "--tests", "--json")
	var object struct {
		OK    bool `json:"ok"`
		Units []struct {
			File string `json:"file"`
			Name string `json:"name"`
		} `json:"units"`
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		t.Fatalf("units --json: %v\n%s", err, stdout)
	}
	if code != 0 || !object.OK || len(object.Units) != 1 || object.Units[0].File != filepath.FromSlash("src/cart.test.ts") || object.Units[0].Name != "cartOf" {
		t.Errorf("units --tests --json: exit %d, %+v; want 0 and src/cart.test.ts's cartOf", code, object)
	}
}

// The itos-cc.yaml the adapters give is the one units reads: its exclude
// leaves a source out, said on stderr, and a file that cannot be read as a
// config is config.invalid.
func TestRunWithFakesReadsTheConfigGiven(t *testing.T) {
	code, stdout, stderr := runFaked(t, fakes([]byte("exclude: [\"**/*.py\"]\n")), "units")
	if code != 0 || strings.Contains(stdout, "price") || !strings.Contains(stdout, "cart#total") || !strings.Contains(stderr, "1 sources left out by exclude") {
		t.Errorf("an exclude: exit %d, stdout %q, stderr %q; want 0, cart alone and the count said", code, stdout, stderr)
	}
	code, stdout, _ = runFaked(t, fakes([]byte("exclude: {a: b}\n")), "units", "--json")
	if code != 80 || !strings.Contains(stdout, `"rule": "config.invalid"`) {
		t.Errorf("an invalid file: exit %d, stdout %q; want 80 and config.invalid", code, stdout)
	}
}

// The top-level help and a problem before any command write to the streams
// given.
func TestRunWithFakesWritesTheStreamsGiven(t *testing.T) {
	code, stdout, _ := runFaked(t, fakes(nil), "--help")
	if code != 0 || !strings.Contains(stdout, "usage: itos-cc <command>") || !strings.Contains(stdout, "units") {
		t.Errorf("--help: exit %d, stdout %q; want 0 and the usage", code, stdout)
	}
	code, stdout, _ = runFaked(t, fakes(nil), "help", "units")
	if code != 0 || !strings.Contains(stdout, "usage: itos-cc units") {
		t.Errorf("help units: exit %d, stdout %q; want 0 and units' help", code, stdout)
	}
	code, _, stderr := runFaked(t, fakes(nil), "nosuch")
	if code != 81 || !strings.Contains(stderr, `there is no command "nosuch"`) {
		t.Errorf("an unknown command: exit %d, stderr %q; want 81 and command.unknown", code, stderr)
	}
}
