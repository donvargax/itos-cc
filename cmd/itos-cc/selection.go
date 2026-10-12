package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

// selectionFlags choose a command's files.
var selectionFlags = []flagSpec{sw("changed", "only files git reports as added or modified")}

// files is the command's files, as the selection domain chooses them under
// ctx from its paths and --changed in the working directory
// (internal/workdir), less the sources itos-cc.yaml excludes, whose count
// it says on stderr when there is any. A file that cannot be read is
// config.invalid.
// Every command selects its files so, which is why cmd holds this and the
// domain's classifier until the commands move into slices.
func files(ctx context.Context, in *invocation) (project.Files, error) {
	exclude, err := exclusion()
	if err != nil {
		return project.Files{}, err
	}
	chosen, err := selection.Select(ctx, workdir.Tree{}, in.args, in.set("changed"), exclude)
	if err != nil {
		return project.Files{}, err
	}
	sayExcluded(os.Stderr, chosen.Excluded)
	return project.Files{Sources: chosen.Sources, Tests: chosen.Tests}, nil
}

// exclusion is the sources itos-cc.yaml's exclude leaves out, its globs
// from the project root. A file that cannot be read is config.invalid.
func exclusion() (selection.Exclusion, error) {
	cfg, err := loadConfig()
	if err != nil {
		return selection.Exclusion{}, err
	}
	return selection.Exclusion{Root: project.Root(), Globs: cfg.Exclude}, nil
}

// sayExcluded says on w how many sources itos-cc.yaml's exclude left out of
// a selection, when it left out any: plain output, for a person.
func sayExcluded(w io.Writer, n int) {
	if n > 0 {
		_, _ = fmt.Fprintf(w, "itos-cc: %d sources left out by exclude in %s\n", n, config.File)
	}
}

// classifySelection is the selection domain's classifier, which cmd hands to
// internal/cli: a failure of the port's sealed set is its problem, and any
// other error is not selection's to claim.
func classifySelection(err error) ([]*cli.Problem, bool) {
	var failure port.Failure
	if !errors.As(err, &failure) {
		return nil, false
	}
	return []*cli.Problem{selectionProblem(failure)}, true
}

// selectionProblem is failure's problem: --changed with no git is the
// environment's, exit 3, and an argument that names nothing a usage error,
// exit 2, as file_selection.feature reads them; a walk or a read that
// failed is the 70 it has always been, its own message whole; an interrupt
// is 130 (decision 31). None has a default; a kind with no case here is the 70 of a bug.
func selectionProblem(failure port.Failure) *cli.Problem {
	p := cli.Internal(failure)
	switch e := failure.(type) {
	case *port.NoGit:
		p = cli.NewProblem(cli.CodeEnvironment, "changed.no-git", "--changed needs a git repository: "+e.Reason,
			"Run it inside a git repository, or name the paths instead.")
	case *port.Unmatched:
		p = cli.NewProblem(cli.CodeUsage, "paths.unmatched", fmt.Sprintf("%q is no file, and no file's path under the working directory contains it", e.Fragment),
			"Name a file, a directory, or a fragment of a source's path.").With("argument", e.Fragment)
	case *port.Unwalkable:
		p = cli.Internal(e)
	case *port.Unreadable:
		p = cli.Internal(e)
	case *port.Cancelled:
		p = cli.Interrupted()
	}
	return p
}
