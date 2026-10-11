package cli

import "github.com/donvargax/itos-cc/internal/counting/port"

// The port's failures are the sealed set this UI classifies. Each kind below
// is one the count command's scenarios read: a missing file and an
// unreadable one, a directory among the latter, both the environment's and
// exit 3, and a read an interrupt cancelled, exit 130 (decision 31). None has
// a default; a kind with no case here is the 70 of a bug (classify's
// comment).
func readProblem(failure port.ReadFailure) (code int, problems []Problem) {
	code, problems = CodeInternal, internal(failure)
	switch e := failure.(type) {
	case *port.Missing:
		code, problems = CodeEnvironment, one("COUNT_INPUT_MISSING",
			"the file %s is not there", "check the path, or give - to count standard input", e.Path)
	case *port.Unreadable:
		code, problems = CodeEnvironment, one("COUNT_INPUT_UNREADABLE",
			"the file %s cannot be read as text", "give a file this program can read, not a folder", e.Path)
	case *port.Cancelled:
		code, problems = interrupted()
	}
	return code, problems
}
