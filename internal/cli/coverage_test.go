package cli

import "testing"

// --coverage-command with no --coverage-report is a usage error, worded
// once for every command that takes both.
func TestACommandWithNoReportIsAUsageError(t *testing.T) {
	p := CommandNeedsReport()
	if p.Code != CodeUsage || p.Rule != "coverage.command-needs-report" || p.Fix != "Add --coverage-report FILE." {
		t.Errorf("got %+v", p)
	}
}
