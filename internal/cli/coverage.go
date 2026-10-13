package cli

// CommandNeedsReport is the problem of --coverage-command given without
// --coverage-report, which says where the report it writes lands: a usage
// error. Every command that measures coverage takes both flags, crap and
// mutation run, so the problem is the UI's to word once.
func CommandNeedsReport() *Problem {
	return NewProblem(CodeUsage, "coverage.command-needs-report", "--coverage-command needs --coverage-report to say where its report lands",
		"Add --coverage-report FILE.")
}
