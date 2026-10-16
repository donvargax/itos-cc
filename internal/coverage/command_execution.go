package coverage

// CommandExecution records an actual command attempt. A failed setup, start,
// wait, or cleanup is represented by Err and never inferred from a report.
type CommandExecution struct {
	Args []string
	Dir  string
	Err  error
}
