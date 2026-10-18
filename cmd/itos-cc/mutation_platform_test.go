package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cmd hands the application the platform counted mode and --fail-fast
// check as a value, countedPlatform, which the test binary's itos-cc takes
// from platformEnv. So each refusal of an unsupported platform shows here
// on every system the tests run on, where ID-MUT-248 sees it natively on
// Windows alone. Kept: the public CLI cannot pretend a platform, so this
// private platform-value guard, split from the ID-MUT-173, 191 and 199
// holders, stays at the boundary that detects the host (T-130).
func TestUnsupportedPlatformRefusalsTakeThePlatformAsAValue(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		mode string
		args []string
		rule string
	}{
		{"counted", []string{"--count", "1"}, "count.platform"},
		{"fail-fast", []string{"--fail-fast"}, "fail-fast.platform"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			t.Parallel()
			dir := moduleRepo(t, compareFiles)
			useEnv(t, platformEnv, "windows")
			o := itosCc(t, append([]string{"mutation", "run", "--json"}, c.args...)...)
			if p := o.counted(t).problem(c.rule); o.code != 82 || p == nil || p["platform"] != "windows" {
				t.Errorf("%s on Windows: exit %d, want 82 with %s for windows\n%s%s", c.mode, o.code, c.rule, o.stdout, o.stderr)
			}
			// It fails before launching any command, and writes nothing.
			for _, launched := range []string{"itos-cc: coverage", "itos-cc: baseline", "itos-cc: ["} {
				if strings.Contains(o.stderr, launched) {
					t.Errorf("%s on Windows launched a command (%q):\n%s", c.mode, launched, o.stderr)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, ".metrics")); !os.IsNotExist(err) {
				t.Errorf("%s on Windows wrote .metrics: %v", c.mode, err)
			}
		})
	}
}
