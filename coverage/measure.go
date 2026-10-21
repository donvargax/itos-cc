package coverage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/project"
)

// Run executes each plan and returns the coverage of sources. A plan whose
// commands fail still contributes any report it wrote, because failing tests
// still measure the code they ran; one that measured none of its files is
// Missing. A plan whose commands all succeed and write their reports
// Measures its language, and is never Missing: its files the report never
// names are untested, since no test loaded them. Problems are written to
// log.
//
// A file of a language whose reports list every class of their module
// (classInventories) that no test reaches has its executable lines listed,
// all unexecuted, from the report of a successful plan of its module.
//
// execute runs each coverage command, in a process group it owns, nil
// running it as it is; a command whose owned group's cleanup did not
// finish leaves its plan's files with that gap, port.CleanupFailed. A
// done context kills the command that runs.
func Run(ctx context.Context, plans []Plan, sources []string, log io.Writer, execute CommandExecutor) *coverage.Report {
	var reports []*coverage.Report
	// classes is, by language and module, the entries of a successful
	// report that lists every class of the module; unlisted is each
	// Unreached plan whose files they can list, by its report's index.
	classes := map[[2]string][]coverage.Entry{}
	unlisted := map[int]Plan{}
	for _, p := range plans {
		// A done context starts no command: the caller reports the stop.
		if ctx.Err() != nil {
			break
		}
		if p.Unreached {
			if classInventories[p.Language] {
				unlisted[len(reports)] = p
			}
			reports = append(reports, p.unreached(log))
			continue
		}
		if p.Unsupported != "" {
			logf(log, "itos-cc: coverage: %s: %s\n", p.Dir, p.Unsupported)
			reports = append(reports, p.missing(port.ToolMissing{}, p.Unsupported))
			continue
		}
		for _, r := range append(p.Reports, p.Integration) {
			// One that stays is written over, or read as it always was;
			// RunSupervised reports it.
			_ = os.Remove(r)
		}
		// A folder it cannot make fails the commands below, which are logged.
		_ = os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755) //nolint:gosec // the report's folder keeps its 0755; narrowing it would change what the run creates
		env := project.NoBytecodeEnv(os.Environ())
		var noIntegration []coverage.Unmeasured
		if p.CollectorMissing != "" {
			noIntegration = append(noIntegration, p.noIntegration(p.CollectorMissing, log))
		}
		if p.Notice != "" {
			logf(log, "itos-cc: coverage: %s: %s\n", p.Dir, p.Notice)
		}
		if p.CoverDir != "" {
			// This unsupervised run has always gone on when its scratch cannot
			// be removed; RunSupervised reports it.
			_ = p.removeCoverDir()
			why, _, err := p.start(ctx, env, log, nil)
			if err != nil {
				why = err.Error()
			}
			switch {
			case why != "":
				noIntegration = append(noIntegration, p.noIntegration(why, log))
				_ = p.removeCoverDir()
				p.CoverDir = ""
			case os.MkdirAll(p.CoverDir, 0o755) == nil: //nolint:gosec // the integration coverage folder keeps its 0755; narrowing it would change what the run creates
				env = setEnv(env, p.CoverEnv...)
			default:
				p.CoverDir = ""
			}
		}
		failed := ""
		var cleanup *port.CleanupFailed
		for _, args := range p.Commands {
			if ctx.Err() != nil {
				failed = fmt.Sprintf("; %v", context.Cause(ctx))
				break
			}
			logf(log, "itos-cc: coverage %s$ %s\n", p.Dir, displayArgs(args))
			cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the plan's coverage commands, which this package composes for the language
			cmd.Dir = p.Dir
			cmd.Env = env
			cmd.Stdout = log
			cmd.Stderr = log
			if err := runCommand(ctx, cmd, execute); err != nil {
				logf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
				failed = fmt.Sprintf("; %s: %v", args[0], err)
				var c port.CleanupFailed
				if errors.As(err, &c) {
					cleanup = &c
				}
			}
		}
		integration, why := p.integrate(log)
		if why != "" {
			noIntegration = append(noIntegration, p.noIntegration(why, log))
		}
		r := load(ctx, p.Reports, integration, p.Dir, p.measures(sources), log)
		r.SetIntegrationMissing(noIntegration...)
		// Told by exit status and the report alone, never by what the
		// runner prints: a run that succeeded and wrote its report measured
		// its language, whichever files the report names.
		if failed == "" && len(r.Missing()) == 0 {
			r = p.notCoverable(ctx, r)
			r.MarkMeasured(p.Language)
			p.prove(r)
			reports = append(reports, r)
			if classInventories[p.Language] && !p.Unattested && len(p.Reports) == 1 {
				// Read again, whole: r holds only the files p measures,
				// and the next plan of the module writes over the report.
				if entries, err := Load(p.Reports[0]); err == nil {
					classes[[2]string{p.Language, p.Dir}] = entries
				}
			}
			continue
		}
		if cleanup != nil {
			reports = append(reports, p.measured(r, *cleanup, "its coverage command's owned process group did not end"+failed))
			continue
		}
		reports = append(reports, p.measured(r, port.MeasuredNothing{}, "its coverage run measured none of its files"+failed))
	}
	for i, p := range unlisted {
		if entries, ok := classes[[2]string{p.Language, p.Dir}]; ok {
			reports[i].List(ctx, Disk{}, p.Sources, p.Dir, entries)
		}
	}
	return coverage.Merge(reports...)
}

// unreached is the report of an Unreached plan: its language measured, and
// none of its sources loaded, as no test reaches them.
func (p Plan) unreached(log io.Writer) *coverage.Report {
	logf(log, "itos-cc: coverage %s: no test reaches %s, so none runs\n", p.Dir, strings.Join(relativeTo(p.Dir, p.Sources), " "))
	r := &coverage.Report{}
	r.MarkMeasured(p.Language)
	p.prove(r)
	return r
}

// prove records, of r, the report of p's commands that all succeeded and
// wrote their reports, which of p's sources it lists completely, at its
// format's line precision (Report.Lines), and which no test loaded, so it
// names none of their lines (Report.Unloaded): none of either for an
// Unattested plan.
func (p Plan) prove(r *coverage.Report) {
	if p.Unattested {
		r.Prove(p.Language, p.Dir, nil)
		return
	}
	r.Prove(p.Language, p.Dir, p.Sources)
}

// missing is a report of nothing, p's build root Missing for cause, why.
func (p Plan) missing(cause port.Gap, why string) *coverage.Report {
	r := &coverage.Report{}
	r.SetMissing(coverage.Unmeasured{Dir: p.Dir, Language: p.Language, Cause: cause, Reason: why})
	return r
}

// measures is the sources p's report speaks for, of sources: its own
// Sources when it measures them alone (OwnSources).
func (p Plan) measures(sources []string) []string {
	if p.OwnSources {
		return p.Sources
	}
	return sources
}

// measured is r, Missing p when r measured none of p's files: the run
// failed, wrote an empty report, or loaded none of them. A report that
// measured some of them leaves the rest untested, not missing.
func (p Plan) measured(r *coverage.Report, cause port.Gap, why string) *coverage.Report {
	for _, s := range p.Sources {
		if r.Has(s) {
			r.SetMissing()
			return r
		}
	}
	if len(p.Sources) > 0 {
		r.SetMissing(coverage.Unmeasured{Dir: p.Dir, Language: p.Language, Cause: cause, Reason: why})
	}
	return r
}

// IgnoreDir creates dir with a .gitignore that ignores everything in it, so
// raw reports stay out of version control while the rest of .metrics is
// committed.
func IgnoreDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // a .metrics/ folder keeps the 0755 a checkout gives it; narrowing it would change what the run creates
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644) //nolint:gosec // the .gitignore is a project file every checkout reads, kept at 0644; narrowing it would change what the run creates
}

// Existing reads reports already on disk, the first one found per plan.
func Existing(ctx context.Context, plans []Plan, sources []string, log io.Writer) *coverage.Report {
	var reports []*coverage.Report
	for _, p := range plans {
		if p.Unreached {
			reports = append(reports, p.unreached(log))
			continue
		}
		r := &coverage.Report{}
		for _, path := range p.Existing {
			if exists(path) {
				// A run's integration profile goes with its report.
				integration := ""
				if path == p.Reports[0] && p.Integration != "" && exists(p.Integration) {
					integration = p.Integration
				}
				r = p.notCoverable(ctx, load(ctx, []string{path}, integration, p.Dir, p.measures(sources), log))
				break
			}
		}
		reports = append(reports, p.measured(r, port.NoReport{}, "no report on disk measures its files"))
	}
	return coverage.Merge(reports...)
}

// Files reads reports named on the command line, relative to the working
// directory.
func Files(ctx context.Context, paths []string, sources []string, log io.Writer) *coverage.Report {
	wd, _ := os.Getwd()
	return load(ctx, paths, "", wd, sources, log)
}

// load reads the reports at paths; each that cannot be read is Missing. The
// profile at integration, unless it is "", is read beside them as
// Integration data, kept to what they count (Report.Integrated); one that
// cannot be read is only logged.
func load(ctx context.Context, paths []string, integration, base string, sources []string, log io.Writer) *coverage.Report {
	var all [][]coverage.Entry
	var missing []coverage.Unmeasured
	for _, path := range paths {
		entries, err := Load(path)
		if err != nil {
			logf(log, "itos-cc: coverage: %v\n", err)
			missing = append(missing, coverage.Unmeasured{Report: path, Cause: port.Unreadable{}, Reason: err.Error()})
			continue
		}
		all = append(all, entries)
	}
	if integration != "" {
		entries, err := Load(integration)
		if err != nil {
			logf(log, "itos-cc: coverage: %v\n", err)
		}
		in := coverage.Build(ctx, Disk{}, sources, base, all...)
		all = append(all, in.Integrated(ctx, Disk{}, sources, base, entries))
	}
	r := coverage.Build(ctx, Disk{}, sources, base, all...)
	r.SetMissing(missing...)
	return r
}

// runCommand runs cmd through execute, or as it is when execute is nil.
func runCommand(ctx context.Context, cmd *exec.Cmd, execute CommandExecutor) error {
	if execute != nil {
		return execute(ctx, cmd)
	}
	return cmd.Run()
}
