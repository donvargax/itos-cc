package mutants

// FailedSelection is a selection of listed tests that fails without any
// mutant.
type FailedSelection struct {
	IDs      []string // in the order the mutants name them
	Command  string   // the shell command line that ran them
	ExitCode int      // the command's exit code, -1 when it gave none
	Output   string
}
