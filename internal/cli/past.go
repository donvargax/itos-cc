package cli

// Past is --at, the flag of the commands that read sources alone: the
// commit whose tree they measure, as if it were checked out, from git's
// objects. A command's kong struct embeds it after Selection, and its
// handler is given the commit's ports through internal/past. A slice copies
// the value into its own Query.
type Past struct {
	At string `name:"at" placeholder:"COMMIT" help:"measure COMMIT's tree, read from git's objects, instead of the working tree; any revision git names, never with --changed"`
}

// Conflict is the problem of --at given with --changed, whose files are the
// working tree's changes, which a commit has none of; nil when the line has
// no such pair.
func (p Past) Conflict(changed bool) *Problem {
	if p.At == "" || !changed {
		return nil
	}
	return NewProblem(CodeUsage, "flags.conflict", "--at and --changed cannot be used together: --changed names the working tree's changes, which a commit has none of",
		"Leave one out: --at measures a commit's whole tree, or name the paths to measure in it.").With("flag", "--at")
}

// NoPast is --at on the commands that cannot measure a past commit, the
// ones that run the project's tests: it is hidden from their help, and the
// command line that gives it is refused (RefusePast) naming why, instead of
// as an unknown flag.
type NoPast struct {
	At string `name:"at" hidden:""`
}

// RefusePast is the problem of --at given to command, which runs the
// project's tests.
func RefusePast(command string) *Problem {
	return NewProblem(CodeUsage, "at.unsupported", command+" needs the project's tests run in a working tree, so it cannot measure a past commit",
		"Leave --at off, or check the commit out in a worktree and run "+command+" there; units, dry and scrap take --at.")
}
