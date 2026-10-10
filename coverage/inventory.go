package coverage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/project"
)

// inventories is, by language, the command that lists the executable lines
// of files no test loaded without running a test, run in their build root
// dir, and the LCOV report it writes in out, every line unexecuted, or ""
// for one it writes on its standard output. No command, for a language or
// a build root without one, leaves such files without proof
// (Report.Lines), so strict coverage reports them missing rather than
// passing them.
var inventories = map[string]func(dir string, files []string, out string) (args []string, report string){
	"python":     pythonInventory,
	"typescript": typescriptInventory,
}

// pythonInventoryScript lists each file's statements as coverage.py's own
// analysis finds them, the lines its LCOV report names DA lines of, its
// configured exclusions honoured, without collecting any data.
const pythonInventoryScript = `import sys, coverage
cov = coverage.Coverage(data_file=None)
for path in sys.argv[1:]:
    lines = cov.analysis2(path)[1]
    sys.stdout.write("SF:%s\n%send_of_record\n" % (path, "".join("DA:%d,0\n" % n for n in lines)))
`

// pythonInventory runs pythonInventoryScript over files with the
// interpreter a coverage plan in dir runs.
func pythonInventory(dir string, files []string, _ string) ([]string, string) {
	return append([]string{pythonFor(dir), "-c", pythonInventoryScript}, files...), ""
}

// typescriptNoTest is a test filter no test file's path holds, so Vitest
// runs no test.
const typescriptNoTest = "itos-cc-static-inventory-runs-no-test"

// typescriptInventory has Vitest, where the package at dir measures with
// it, run no test and report files, which coverage.include names, as its
// v8 provider reports a file no test loaded: every line its syntax-aware
// remapping finds executable, none executed, the same lines a run that
// loads the file names. Jest and c8 have no such command here.
func typescriptInventory(dir string, files []string, out string) ([]string, string) {
	vitest := project.NodeBin(dir, "vitest")
	if !readPackageJSON(dir).has("vitest") || vitest == "" || vitestTooOld(installedVersion(dir, "vitest")) ||
		project.NodeModule(dir, "@vitest/coverage-v8") == "" {
		return nil, ""
	}
	args := []string{vitest, "run", typescriptNoTest, "--passWithNoTests", "--coverage.enabled",
		"--coverage.reporter=lcov", "--coverage.reportsDirectory=" + out}
	for _, file := range relativeTo(dir, files) {
		args = append(args, "--coverage.include="+file)
	}
	return args, filepath.Join(out, "lcov.info")
}

// Inventory lists, with language's static inventory command, the
// executable lines of each of its sources no test loaded (Unloaded), all
// of them unexecuted, and proves them complete (Lines). It runs no test.
// A command that fails leaves its files without proof. execute runs each
// command, or, when nil, it runs as Run runs coverage commands; each
// command is logged and returned. The error joins each command's failure.
func (r *Report) Inventory(ctx context.Context, language string, log io.Writer, execute CommandExecutor) ([]CommandExecution, error) {
	inventory := inventories[language]
	if r == nil || inventory == nil {
		return nil, nil
	}
	byDir := map[string][]string{}
	for _, file := range r.Unloaded(language) {
		byDir[r.unloaded[file].Dir] = append(byDir[r.unloaded[file].Dir], file)
	}
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	var executions []CommandExecution
	var failures []error
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		files := byDir[dir]
		var abs []string
		for _, file := range files {
			path, err := filepath.Abs(file)
			if err != nil {
				path = file
			}
			abs = append(abs, path)
		}
		out, err := os.MkdirTemp("", "itos-cc-inventory-")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		args, report := inventory(dir, abs, out)
		if len(args) == 0 {
			os.RemoveAll(out)
			continue
		}
		fmt.Fprintf(log, "itos-cc: coverage %s$ %s (static inventory, no test runs) %s\n", dir, args[0], strings.Join(relativeTo(dir, files), " "))
		var stdout bytes.Buffer
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir, cmd.Env = dir, project.NoBytecodeEnv(os.Environ())
		cmd.Stdout, cmd.Stderr = &stdout, log
		if report != "" {
			cmd.Stdout = log
		}
		if execute != nil {
			err = execute(ctx, cmd)
		} else {
			err = cmd.Run()
		}
		executions = append(executions, CommandExecution{Args: slices.Clone(args), Dir: dir, Err: err})
		var entries []Entry
		switch {
		case err != nil:
		case report != "":
			entries, err = Load(report)
		default:
			entries, err = ParseLCOV(&stdout)
		}
		os.RemoveAll(out)
		if err != nil {
			fmt.Fprintf(log, "itos-cc: coverage: %s: static inventory: %v\n", language, err)
			failures = append(failures, fmt.Errorf("%s static coverage inventory in %s: %w", language, dir, err))
			continue
		}
		m := newMatcher(files)
		for _, e := range entries {
			file := m.match(e.Path, dir)
			if file == "" {
				continue
			}
			if r.inventory == nil {
				r.inventory = map[string][]Segment{}
			}
			if r.proven == nil {
				r.proven = map[string]bool{}
			}
			for _, seg := range e.Segments {
				seg.Covered = 0
				r.inventory[file] = append(r.inventory[file], seg)
			}
			r.proven[file] = true
		}
	}
	return executions, errors.Join(failures...)
}
