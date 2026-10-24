package covrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/project"
)

// CommandExecutor executes an already configured coverage command. The
// callback is instance-local so the coverage package does not depend on the
// mutation package that supplies Linux process ownership.
type CommandExecutor func(context.Context, *exec.Cmd) error

type CommandExecution = coverage.CommandExecution

// RunSupervised measures plans with an explicit executor and returns every
// command outcome. Unlike Run, it is fail-closed: unsupported plans, failed
// commands, unreadable/missing reports, and failed Go covdata conversion are
// returned as errors. Run remains the compatibility path for complete runs.
func RunSupervised(ctx context.Context, plans []Plan, sources []string, log io.Writer, execute CommandExecutor) (*coverage.Report, []CommandExecution, error) {
	if execute == nil {
		return nil, nil, errors.New("supervised coverage needs a command executor")
	}
	if len(plans) == 0 && len(sources) > 0 {
		return nil, nil, errors.New("supervised coverage has sources but no measurement plans")
	}
	var reports []*coverage.Report
	var executions []CommandExecution
	var failures []error
	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		planFailed := false
		if p.Unreached {
			reports = append(reports, p.unreached(log))
			continue
		}
		if p.Unsupported != "" {
			err := fmt.Errorf("%s: unsupported coverage scope: %s", p.Dir, p.Unsupported)
			failures = append(failures, err)
			reports = append(reports, p.missing(port.ToolMissing{}, p.Unsupported))
			continue
		}
		if len(p.Commands) == 0 {
			err := fmt.Errorf("%s: coverage plan has no executable measurement command", p.Dir)
			failures = append(failures, err)
			planFailed = true
		}
		if len(p.Reports) == 0 {
			failures = append(failures, fmt.Errorf("%s: coverage plan has no report", p.Dir))
			reports = append(reports, p.missing(port.Unreadable{}, "coverage plan has no report"))
			continue
		}
		for _, r := range append(append([]string{}, p.Reports...), p.Integration) {
			if r != "" {
				if err := os.Remove(r); err != nil && !errors.Is(err, os.ErrNotExist) {
					failures = append(failures, fmt.Errorf("remove stale report %s: %w", r, err))
					planFailed = true
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755); err != nil { //nolint:gosec // the report's folder keeps its 0755; narrowing it would change what the run creates
			failures = append(failures, fmt.Errorf("prepare report directory for %s: %w", p.Language, err))
			continue
		}
		env := project.NoBytecodeEnv(os.Environ())
		var noIntegration []coverage.Unmeasured
		if p.CollectorMissing != "" {
			noIntegration = append(noIntegration, p.noIntegration(p.CollectorMissing, log))
		}
		if p.Notice != "" {
			logf(log, "itos-cc: coverage: %s: %s\n", p.Dir, p.Notice)
		}
		if p.CoverDir != "" {
			if err := p.removeCoverDir(); err != nil {
				failures = append(failures, fmt.Errorf("clear integration coverage directory: %w", err))
				continue
			}
			why, call, err := p.start(ctx, env, log, execute)
			if len(p.Prepare) > 0 {
				executions = append(executions, call)
			}
			if err != nil {
				failures = append(failures, err)
				_ = p.removeCoverDir()
				continue
			}
			if why != "" {
				// A collector the project lacks measures nothing, as
				// before, and the run goes on without it.
				noIntegration = append(noIntegration, p.noIntegration(why, log))
				if err := p.removeCoverDir(); err != nil {
					failures = append(failures, fmt.Errorf("remove integration coverage directory: %w", err))
				}
				p.CoverDir = ""
			} else {
				if err := os.MkdirAll(p.CoverDir, 0o755); err != nil { //nolint:gosec // the integration coverage folder keeps its 0755; narrowing it would change what the run creates
					failures = append(failures, fmt.Errorf("prepare integration coverage directory: %w", err))
					continue
				}
				env = setEnv(env, p.CoverEnv...)
			}
		}
		for _, args := range p.Commands {
			if err := ctx.Err(); err != nil {
				failures = append(failures, err)
				break
			}
			logf(log, "itos-cc: coverage %s$ %s\n", p.Dir, displayArgs(args))
			cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the plan's coverage commands, which this package composes for the language
			cmd.Dir, cmd.Env = p.Dir, env
			cmd.Stdout, cmd.Stderr = log, log
			err := execute(ctx, cmd)
			executions = append(executions, CommandExecution{Args: append([]string{}, args...), Dir: p.Dir, Err: err})
			if err != nil {
				failures = append(failures, fmt.Errorf("%s coverage command %q: %w", p.Language, args, err))
				planFailed = true
				logf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
			}
		}
		aborted := ctx.Err() != nil
		if aborted {
			failures = append(failures, ctx.Err())
			if p.CoverDir != "" {
				_ = os.RemoveAll(p.CoverDir)
			}
		}
		integration := ""
		if !aborted {
			var conversions []CommandExecution
			var conversion error
			var why string
			integration, why, conversions, conversion = p.integrateSupervised(ctx, log, execute)
			executions = append(executions, conversions...)
			if why != "" {
				// Data the project has no converter for: the run goes on
				// without it, as without a collector.
				noIntegration = append(noIntegration, p.noIntegration(why, log))
			}
			if conversion != nil {
				failures = append(failures, conversion)
				planFailed = true
			}
		}
		r := load(ctx, p.Reports, integration, p.Dir, p.measures(sources), log)
		r.SetIntegrationMissing(noIntegration...)
		for _, missing := range r.Missing() {
			failures = append(failures, fmt.Errorf("coverage report: %s", missing.String()))
			planFailed = true
		}
		if r.Empty() && len(p.Sources) > 0 {
			r = p.measured(r, port.MeasuredNothing{}, "the supervised run measured none of its files")
			failures = append(failures, fmt.Errorf("%s: coverage measured none of its admitted sources", p.Language))
			planFailed = true
		}
		if !planFailed {
			r = p.notCoverable(ctx, r)
			r.MarkMeasured(p.Language)
			p.prove(r)
		}
		reports = append(reports, r)
		if aborted {
			break
		}
	}
	merged := coverage.Merge(reports...)
	return merged, executions, errors.Join(failures...)
}

// MeasureTestsSupervised measures every listed test through the same owned
// executor used for language coverage and conversion. It fails closed when a
// listed test has no per-ID coverage artifact or an artifact cannot be read.
func MeasureTestsSupervised(ctx context.Context, p PerTest, dir string, sources []string, log io.Writer, execute CommandExecutor) (result *coverage.TestCoverage, calls []CommandExecution, resultErr error) {
	if execute == nil {
		return nil, nil, errors.New("supervised listed coverage needs a command executor")
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, nil, fmt.Errorf("clear listed coverage directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove listed coverage data: %w", err))
		}
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the listed tests' coverage folder keeps its 0755; narrowing it would change what the run creates
		return nil, nil, fmt.Errorf("create listed coverage directory: %w", err)
	}
	tc := &coverage.TestCoverage{}
	ids := make([]string, 0, len(p.Tests))
	for _, test := range p.Tests {
		if test.ID == "" {
			return nil, nil, errors.New("listed coverage contains an empty test ID")
		}
		ids = append(ids, test.ID)
	}
	var executions []CommandExecution
	run := func(ctx context.Context, id, line string, env []string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		logf(log, "itos-cc: coverage %s$ %s\n", p.Root, line)
		name, flag := "sh", "-c"
		if runtime.GOOS == "windows" {
			name, flag = "cmd", "/C"
		}
		cmd := exec.CommandContext(ctx, name, flag, line) //nolint:gosec // the person's own listed-test command line, run through the platform shell
		cmd.Dir = p.Root
		cmd.Env = setEnv(project.NoBytecodeEnv(os.Environ()), env...)
		cmd.Stdout, cmd.Stderr = log, log
		err := execute(ctx, cmd)
		args := append([]string{name, flag, line}, env...)
		executions = append(executions, CommandExecution{Args: args, Dir: p.Root, Err: err})
		if err != nil {
			return fmt.Errorf("listed test %s command: %w", id, err)
		}
		return nil
	}
	collectors, calls, err := startTestCollectors(ctx, testCollectors(ctx, p.Root, dir, sources), log, execute, true)
	executions = append(executions, calls...)
	if err != nil {
		return nil, executions, err
	}
	written := map[string]string{}
	if err := run(ctx, "all", p.All, wholeEnv(collectors, dir)); err != nil {
		return nil, executions, err
	}
	for _, id := range ids {
		candidate := filepath.Join(dir, id)
		if anyWritten(collectors, candidate) {
			written[id] = candidate
		}
	}
	if len(written) == 0 && len(ids) > 0 {
		for i, id := range ids {
			if p.Select == nil {
				return nil, executions, fmt.Errorf("listed test %s has no selector", id)
			}
			d := filepath.Join(dir, ".each", strconv.Itoa(i))
			if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // a listed test's own coverage folder keeps its 0755; narrowing it would change what the run creates
				return nil, executions, fmt.Errorf("prepare coverage for listed test %s: %w", id, err)
			}
			if err := run(ctx, id, p.Select([]string{id}), eachEnv(collectors, d)); err != nil {
				return nil, executions, err
			}
			if anyWritten(collectors, d) {
				written[id] = d
			}
		}
	}
	for i, id := range ids {
		d, ok := written[id]
		if !ok {
			return nil, executions, fmt.Errorf("listed test %s produced no per-ID coverage data", id)
		}
		var reports []*coverage.Report
		for _, c := range collectors {
			if !c.written(d) {
				continue
			}
			if err := os.MkdirAll(filepath.Join(dir, ".profiles"), 0o755); err != nil { //nolint:gosec // the converted profiles' folder keeps its 0755; narrowing it would change what the run creates
				return nil, executions, fmt.Errorf("prepare profile for listed test %s: %w", id, err)
			}
			args, profile := c.convert(d, filepath.Join(dir, ".profiles", strconv.Itoa(i)+"."+c.language))
			cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the collector's conversion of a listed test's data, which this package names
			cmd.Dir, cmd.Stdout, cmd.Stderr = c.dir, log, log
			cmd.Env = unsetEnv(project.NoBytecodeEnv(os.Environ()), c.vars...)
			err := execute(ctx, cmd)
			executions = append(executions, CommandExecution{Args: args, Dir: c.dir, Err: err})
			if err != nil {
				return nil, executions, fmt.Errorf("convert listed test %s coverage: %w", id, err)
			}
			entries, err := Load(profile)
			if err != nil {
				return nil, executions, fmt.Errorf("read listed test %s coverage: %w", id, err)
			}
			reports = append(reports, coverage.Build(ctx, Disk{}, sources, c.dir, entries))
		}
		tc.Add(id, coverage.Merge(reports...))
	}
	return tc, executions, nil
}

// integrateSupervised converts what the processes the tests started wrote
// under CoverDir, returning the conversions it actually ran, if any, and
// why, when data was written that p has no converter for.
func (p Plan) integrateSupervised(ctx context.Context, log io.Writer, execute CommandExecutor) (report, why string, calls []CommandExecution, resultErr error) {
	if p.CoverDir == "" {
		return "", "", nil, nil
	}
	defer func() {
		if err := p.removeCoverDir(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove %s integration coverage data: %w", p.Language, err))
		}
	}()
	if _, err := filepath.Glob(filepath.Join(p.CoverDir, p.Written)); err != nil {
		return "", "", nil, fmt.Errorf("inspect integration coverage data: %w", err)
	}
	if err := p.dropRunnerData(); err != nil {
		return "", "", nil, err
	}
	if !p.written() {
		return "", "", nil, nil
	}
	if p.ConverterMissing != "" {
		return "", p.ConverterMissing, nil, nil
	}
	for _, args := range p.conversions() {
		logf(log, "itos-cc: coverage %s$ %s\n", p.Dir, displayArgs(args))
		cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the plan's conversion of its coverage data, which this package names
		cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = p.Dir, p.convertEnv(), log, log
		err := execute(ctx, cmd)
		calls = append(calls, CommandExecution{Args: append([]string{}, args...), Dir: p.Dir, Err: err})
		if err != nil {
			return "", "", calls, fmt.Errorf("convert %s integration coverage: %w", p.Language, err)
		}
	}
	if _, err := Load(p.Integration); err != nil {
		return "", "", calls, fmt.Errorf("%s integration conversion produced no readable report: %w", p.Language, err)
	}
	return p.Integration, "", calls, nil
}
