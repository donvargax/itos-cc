//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The counted scenario of "Rule: macOS mutation commands return only after
// their owned process trees are cleaned up" in features/mutate.feature. It
// judges counted mode on macOS itself; the other counted scenarios run on
// macOS too, unchanged, once counted mode admits it. Its one mutant trial
// is Compare's, which its test kills.

// @ID-MUT-191
func TestCountedExecutionIsAdmittedOnMacOSAndStillRefusedOnWindows(t *testing.T) {
	requireCountedPlatform(t)
	if runtime.GOOS != "darwin" {
		t.Skip("ID-MUT-191 judges counted mode on macOS")
	}
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "CLI.md"))
	if err != nil {
		t.Fatal(err)
	}

	// An interrupted macOS counted run reports partial work and cleans up as
	// on Linux. It runs first: it builds itos-cc in the package's directory,
	// which the fixture repository below replaces as the working directory.
	t.Run("interrupted", checkInterruptedCountedRun)

	// Given a committed project with eligible mutation sites, when a
	// count-one run judges it on macOS.
	moduleRepo(t, compareFiles)
	o := countedRun(t, "--count", "1", "--workers", "1")
	c := o.counted(t)
	if p := c.problem("count.platform"); p != nil {
		t.Fatalf("counted mode refused macOS: %+v\n%s", p, o.stderr)
	}
	if o.code != 0 || !c.OK {
		t.Fatalf("count-one run on macOS: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if len(c.Selected) != 1 || c.Selected[0]["state"] != "judged" || c.Selected[0]["outcome"] != "killed" || c.number("executed") != 1 {
		t.Errorf("selected = %+v, sampling = %+v; want Compare's one site judged killed by one trial", c.Selected, c.Sampling)
	}

	// On Windows counted mode still fails clearly before launching commands,
	// shown here with the platform pretended, as on Linux.
	countedPlatform = "windows"
	o = countedRun(t, "--count", "1")
	countedPlatform = runtime.GOOS
	if p := o.counted(t).problem("count.platform"); o.code != 3 || p == nil || p["platform"] != "windows" ||
		strings.Contains(o.stderr, "itos-cc: coverage") || strings.Contains(o.stderr, "itos-cc: baseline") {
		t.Errorf("counted mode on Windows: exit %d, want count.platform before launching commands\n%s%s", o.code, o.stdout, o.stderr)
	}

	// The help, README and docs name Linux and macOS as the supported
	// platforms, and Windows as #29.
	help := cli(t, "mutation", "run", "--help").stdout
	for name, text := range map[string]string{"mutation run --help": help, "README.md": string(readme), "docs/CLI.md": string(docs)} {
		if !strings.Contains(text, "Linux and macOS") || !strings.Contains(text, "#29") {
			t.Errorf("%s does not name Linux and macOS as counted mode's platforms and Windows as #29", name)
		}
		if strings.Contains(text, "Linux only") || strings.Contains(text, "Linux-only") {
			t.Errorf("%s still says counted mode is Linux only", name)
		}
	}
}
