package coverage

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
	"strings"
)

// CommandExecutor executes an already configured coverage command. The
// callback is instance-local so the coverage package does not depend on the
// mutation package that supplies Linux process ownership.
type CommandExecutor func(context.Context, *exec.Cmd) error

// CommandExecution records an actual command attempt. A failed setup, start,
// wait, or cleanup is represented by Err and never inferred from a report.
type CommandExecution struct {
	Args []string
	Dir  string
	Err  error
}

// RunSupervised measures plans with an explicit executor and returns every
// command outcome. Unlike Run, it is fail-closed: unsupported plans, failed
// commands, unreadable/missing reports, and failed Go covdata conversion are
// returned as errors. Run remains the compatibility path for complete runs.
func RunSupervised(ctx context.Context, plans []Plan, sources []string, log io.Writer, execute CommandExecutor) (*Report, []CommandExecution, error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("supervised coverage needs a context and command executor")
	}
	if len(plans) == 0 && len(sources) > 0 {
		return nil, nil, errors.New("supervised coverage has sources but no measurement plans")
	}
	var reports []*Report
	var executions []CommandExecution
	var failures []error
	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		planFailed := false
		if p.Unsupported != "" {
			err := fmt.Errorf("%s: unsupported coverage scope: %s", p.Dir, p.Unsupported)
			failures = append(failures, err)
			reports = append(reports, &Report{missing: []Unmeasured{{Dir: p.Dir, Language: p.Language, Cause: ToolMissing, Reason: p.Unsupported}}})
			continue
		}
		if len(p.Commands) == 0 {
			err := fmt.Errorf("%s: coverage plan has no executable measurement command", p.Dir)
			failures = append(failures, err)
			planFailed = true
		}
		if len(p.Reports) == 0 {
			failures = append(failures, fmt.Errorf("%s: coverage plan has no report", p.Dir))
			reports = append(reports, &Report{missing: []Unmeasured{{Dir: p.Dir, Language: p.Language, Cause: Unreadable, Reason: "coverage plan has no report"}}})
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
		if err := os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755); err != nil {
			failures = append(failures, fmt.Errorf("prepare report directory for %s: %w", p.Language, err))
			continue
		}
		var env []string
		if p.CoverDir != "" {
			if err := os.MkdirAll(p.CoverDir, 0o755); err != nil {
				failures = append(failures, fmt.Errorf("prepare integration coverage directory: %w", err))
				continue
			}
			env = append(os.Environ(), coverDirEnv+"="+p.CoverDir)
		}
		for _, args := range p.Commands {
			if err := ctx.Err(); err != nil {
				failures = append(failures, err)
				break
			}
			fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", p.Dir, strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir, cmd.Env = p.Dir, env
			cmd.Stdout, cmd.Stderr = log, log
			err := execute(ctx, cmd)
			executions = append(executions, CommandExecution{Args: append([]string{}, args...), Dir: p.Dir, Err: err})
			if err != nil {
				failures = append(failures, fmt.Errorf("%s coverage command %q: %w", p.Language, args, err))
				planFailed = true
				fmt.Fprintf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
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
			integration, conversions, conversion = p.integrateSupervised(ctx, log, execute)
			executions = append(executions, conversions...)
			if conversion != nil {
				failures = append(failures, conversion)
				planFailed = true
			}
		}
		r := load(p.Reports, integration, p.Dir, sources, log)
		if len(r.missing) > 0 {
			for _, missing := range r.missing {
				failures = append(failures, fmt.Errorf("coverage report: %s", missing.String()))
				planFailed = true
			}
		}
		if len(r.files) == 0 && len(p.Sources) > 0 {
			r = p.measured(r, MeasuredNothing, "the supervised run measured none of its files")
			failures = append(failures, fmt.Errorf("%s: coverage measured none of its admitted sources", p.Language))
			planFailed = true
		}
		if !planFailed {
			r.languages = map[string]bool{p.Language: true}
		}
		reports = append(reports, r)
		if aborted {
			break
		}
	}
	merged := Merge(reports...)
	return merged, executions, errors.Join(failures...)
}

// MeasureTestsSupervised measures every listed test through the same owned
// executor used for language coverage and conversion. It fails closed when a
// listed test has no per-ID coverage artifact or an artifact cannot be read.
func MeasureTestsSupervised(ctx context.Context, p PerTest, dir string, sources []string, log io.Writer, execute CommandExecutor) (result *TestCoverage, calls []CommandExecution, resultErr error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("supervised listed coverage needs a context and command executor")
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, nil, fmt.Errorf("clear listed coverage directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove listed coverage data: %w", err))
		}
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create listed coverage directory: %w", err)
	}
	tc := &TestCoverage{reports: map[string]*Report{}}
	ids := make([]string, 0, len(p.Tests))
	for _, test := range p.Tests {
		if test.ID == "" {
			return nil, nil, errors.New("listed coverage contains an empty test ID")
		}
		ids = append(ids, test.ID)
	}
	var executions []CommandExecution
	run := func(id, line string, env []string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", p.Root, line)
		name, flag := "sh", "-c"
		if runtime.GOOS == "windows" {
			name, flag = "cmd", "/C"
		}
		cmd := exec.CommandContext(ctx, name, flag, line)
		cmd.Dir = p.Root
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout, cmd.Stderr = log, log
		err := execute(ctx, cmd)
		args := append([]string{name, flag, line}, env...)
		executions = append(executions, CommandExecution{Args: args, Dir: p.Root, Err: err})
		if err != nil {
			return fmt.Errorf("listed test %s command: %w", id, err)
		}
		return nil
	}
	written := map[string]string{}
	if err := run("all", p.All, []string{TestCoverDirEnv + "=" + dir}); err != nil {
		return nil, executions, err
	}
	for _, id := range ids {
		candidate := filepath.Join(dir, id)
		if hasCoverData(candidate) {
			written[id] = candidate
		}
	}
	if len(written) == 0 && len(ids) > 0 {
		for i, id := range ids {
			if p.Select == nil {
				return nil, executions, fmt.Errorf("listed test %s has no selector", id)
			}
			d := filepath.Join(dir, ".each", strconv.Itoa(i))
			if err := os.MkdirAll(d, 0o755); err != nil {
				return nil, executions, fmt.Errorf("prepare coverage for listed test %s: %w", id, err)
			}
			if err := run(id, p.Select([]string{id}), eachTestEnv(d)); err != nil {
				return nil, executions, err
			}
			if hasCoverData(d) {
				written[id] = d
			}
		}
	}
	for i, id := range ids {
		d, ok := written[id]
		if !ok {
			return nil, executions, fmt.Errorf("listed test %s produced no per-ID coverage data", id)
		}
		profile := filepath.Join(dir, ".profiles", strconv.Itoa(i)+".out")
		if err := os.MkdirAll(filepath.Dir(profile), 0o755); err != nil {
			return nil, executions, fmt.Errorf("prepare profile for listed test %s: %w", id, err)
		}
		args := []string{"go", "tool", "covdata", "textfmt", "-i=" + d, "-o=" + profile}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = p.Root, log, log
		err := execute(ctx, cmd)
		executions = append(executions, CommandExecution{Args: args, Dir: p.Root, Err: err})
		if err != nil {
			return nil, executions, fmt.Errorf("convert listed test %s coverage: %w", id, err)
		}
		entries, err := Load(profile)
		if err != nil {
			return nil, executions, fmt.Errorf("read listed test %s coverage: %w", id, err)
		}
		tc.ids = append(tc.ids, id)
		tc.reports[id] = Build(sources, p.Root, entries)
	}
	return tc, executions, nil
}

// integrateSupervised converts what Go binaries built with -cover wrote under
// CoverDir, returning the conversion it actually ran, if any.
func (p Plan) integrateSupervised(ctx context.Context, log io.Writer, execute CommandExecutor) (profile string, calls []CommandExecution, resultErr error) {
	if p.CoverDir == "" {
		return "", nil, nil
	}
	defer func() {
		if err := os.RemoveAll(p.CoverDir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove Go integration coverage data: %w", err))
		}
	}()
	written, err := filepath.Glob(filepath.Join(p.CoverDir, "covmeta.*"))
	if err != nil {
		return "", nil, fmt.Errorf("inspect integration coverage data: %w", err)
	}
	if len(written) == 0 {
		return "", nil, nil
	}
	args := []string{"go", "tool", "covdata", "textfmt", "-i=" + p.CoverDir, "-o=" + p.Integration}
	fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", p.Dir, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = p.Dir, log, log
	err = execute(ctx, cmd)
	calls = []CommandExecution{{Args: args, Dir: p.Dir, Err: err}}
	if err != nil {
		return "", calls, fmt.Errorf("convert Go integration coverage: %w", err)
	}
	if _, err := Load(p.Integration); err != nil {
		return "", calls, fmt.Errorf("Go integration conversion produced no readable profile: %w", err)
	}
	return p.Integration, calls, nil
}
