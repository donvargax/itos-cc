package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of bug-1 in features/coverage.feature, until coverage.feature
// runs through godog (T-61): a report's paths match the sources whichever
// separator it was written with, \ as a report written on Windows has them
// or /, so a report from any system matches sources on every system.

// @ID-COV-32
func TestAReportMatchesSourcesWhicheverSeparatorItWasWrittenWith(t *testing.T) {
	t.Parallel()
	for _, separator := range []string{`\`, "/"} {
		t.Run(separator, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "src", "demo", "board.go"),
				"package demo\n\nfunc Place(x int) int {\n\ty := x\n\ty++\n\ty++\n\treturn y\n}\n")
			// Three of the four lines of Place's body ran: 75%.
			writeFile(t, filepath.Join(dir, "lcov.info"), "SF:"+strings.Join([]string{"src", "demo", "board.go"}, separator)+
				"\nDA:4,1\nDA:5,1\nDA:6,1\nDA:7,0\nend_of_record\n")
			useDir(t, dir)

			o := itosCc(t, "crap", "--json", "--coverage-report", "lcov.info")
			c := crapOf(t, o)
			if len(c.Entries) != 1 {
				t.Fatalf("%d functions scored, want Place alone\n%s%s", len(c.Entries), o.stdout, o.stderr)
			}
			if e := c.Entries[0]; e.Coverage == nil || *e.Coverage != 75 {
				t.Errorf("%s of %s: coverage %v, want 75%%: the report written with %s separators matches no source\n%s%s",
					e.Name, e.File, e.Coverage, separator, o.stdout, o.stderr)
			}
		})
	}
}
