package worktrees

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/processes"
	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// linkDirs hold dependencies: a worker links them instead of copying.
var linkDirs = map[string]bool{"node_modules": true, ".venv": true, "venv": true, "vendor": true}

// skipDirs are history and caches a worker neither copies nor shares, as
// is build output (project.IsBuildOutput) and what git ignores (slice-43,
// ignored.go); tools rebuild what they need inside the worker.
// .dart_tool holds pub's resolution, which points into the tree it was
// made for, and flutter test writes into it, so a worker neither copies nor
// links it: its copy resolves itself (Command.Ready).
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".metrics": true, ".idea": true, ".vscode": true,
	".gradle": true, "__pycache__": true, ".pytest_cache": true, ".mypy_cache": true, ".tox": true,
	".dart_tool": true,
}

// worker owns private copies of project trees, so mutants run in parallel
// without ever touching the real files.
type worker struct {
	dir    string
	copies map[string]*workerCopy // real root → copy
	// project is the working directory, whose project root bounds the
	// workspace a copy may hold (nodeWorkspace); "", each package's own
	// project root, as a counted run's frozen export has.
	project string
	runner  processes.Runner
	// prepare, when set, adjusts each command before it runs, such as its
	// environment; complete runs leave it unset.
	prepare func(*exec.Cmd)
	// restored, when set, hears each restore of a copy end: a test's
	// observation, unset in a run.
	restored func()
	// rules are what the pool's copies leave out as git ignores it and
	// bring back as mutation.copy names; nil for a counted run's, whose
	// frozen export holds tracked files alone.
	rules *copyRules
}

// tree is the tree a worker copies for c, and the members it copies of
// it: those of the pub workspace c.Root belongs to (pubWorkspace), else
// the whole root of the TypeScript workspace it belongs to within the
// project (nodeWorkspace), else c.Root's own.
func (w *worker) tree(c Command) (string, []string) {
	if tree, members := pubWorkspace(c.Root); members != nil {
		return tree, members
	}
	dir := w.project
	if dir == "" {
		dir = c.Root
	}
	return nodeWorkspace(c.Root, project.RootOf(dir)), nil
}

// copyOf returns the worker's copy of c.Root, creating it on first use:
// the tree tree says c needs, then c.Ready run once in it, before any
// command runs there, and the record of it as made. c runs in c.Root's
// place in it.
func (w *worker) copyOf(ctx context.Context, c Command) (string, error) {
	tree, members := w.tree(c)
	rel, err := filepath.Rel(tree, c.Root)
	if err != nil {
		return "", err
	}
	if cp, ok := w.copies[tree]; ok {
		return filepath.Join(cp.dir, rel), nil
	}
	h := sha256.Sum256([]byte(tree))
	place := filepath.Join(w.dir, hex.EncodeToString(h[:4]))
	dst := filepath.Join(place, filepath.Base(tree))
	// A copy discarded in part leaves its folder: the new one goes beside.
	for n := 1; exists(dst); n++ {
		dst = filepath.Join(fmt.Sprintf("%s-%d", place, n), filepath.Base(tree))
	}
	plan := &copyPlan{tree: tree, rules: w.rules, ignored: w.rules.ignored(ctx, tree), dart: regular(filepath.Join(tree, "pubspec.yaml"))}
	if err := copyPackages(ctx, plan, members, dst); err != nil {
		return "", err
	}
	root := filepath.Join(dst, rel)
	cp, err := w.made(ctx, c, tree, dst, root, plan.brought)
	if err != nil {
		cp.discard() // a later command copies afresh
		return "", err
	}
	w.copies[tree] = cp
	return root, nil
}

// made readies dst, the worker's fresh copy of tree, where c.Root's place
// is root and which holds brought, what mutation.copy brought back, and
// records it as made: a file Ready wrote is kept beside, as no project
// file gives it back.
func (w *worker) made(ctx context.Context, c Command, tree, dst, root string, brought []string) (*workerCopy, error) {
	cp := &workerCopy{dir: dst, tree: tree, kept: dst + ".ready", brought: brought}
	record, err := cp.recordOf(ctx)
	if err != nil || len(c.Ready) == 0 {
		cp.record = record
		return cp, err
	}
	if err := w.ready(ctx, c, root); err != nil {
		return cp, err
	}
	readied, err := cp.recordOf(ctx)
	if err != nil {
		return cp, err
	}
	cp.record = readied
	return cp, keepReady(dst, cp.kept, record, readied)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// ready runs c.Ready in root, the worker's fresh copy of c.Root, from
// c.Dir's place in it, failing with its output when it fails.
func (w *worker) ready(ctx context.Context, c Command, root string) error {
	cmd := exec.CommandContext(ctx, c.Ready[0], c.Ready[1:]...) //nolint:gosec // the selected package's own toolchain readying the worker's private copy
	w.prepared(cmd, c, root)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	r, err := w.runner.Run(ctx, cmd, &out)
	switch {
	case err != nil:
		return err
	case ctx.Err() != nil:
		return fmt.Errorf("ready the worker's copy with %s: %w", strings.Join(c.Ready, " "), ctx.Err())
	case !r.Passed:
		return fmt.Errorf("ready the worker's copy with %s: exit %d\n%s", strings.Join(c.Ready, " "), r.ExitCode, r.Output)
	}
	return nil
}

// copyPackages copies tree to dst: members alone, each at its place in
// tree, beside tree's pubspec.yaml and pubspec.lock, when members are a
// pub workspace's, else the whole tree.
func copyPackages(ctx context.Context, plan *copyPlan, members []string, dst string) error {
	tree := plan.tree
	if members == nil {
		return copyTree(ctx, tree, dst, plan)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil { //nolint:gosec // preserve project directory behavior inside the owner's private 0700 scratch
		return err
	}
	for _, name := range []string{"pubspec.yaml", "pubspec.lock"} {
		if err := copyFile(filepath.Join(tree, name), filepath.Join(dst, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for _, member := range members {
		rel, err := filepath.Rel(tree, member)
		if err != nil {
			return err
		}
		if err := copyTree(ctx, member, filepath.Join(dst, rel), plan); err != nil {
			return err
		}
	}
	return nil
}

// copyPlan is how a worker copies tree: what git ignores there, which
// the copy leaves out, the pool's rules, whether tree is a Dart package's,
// whose generated Dart stays though ignored, and, as the copy is made,
// each path mutation.copy brought back that the copy would leave out, by
// its path from tree.
type copyPlan struct {
	tree    string
	rules   *copyRules
	ignored ignoredPaths
	dart    bool
	brought []string
}

// leaving is why a copy leaves out an entry: not at all, as git ignores it
// alone, or as every copy leaves it out, history, caches and build output.
type leaving int

const (
	kept leaving = iota
	leftIgnored
	leftAlways
)

// leaves says why the copy leaves out the entry d at path, rel from the
// tree.
func (p *copyPlan) leaves(ctx context.Context, path, rel string, d fs.DirEntry) leaving {
	slashed := filepath.ToSlash(rel)
	switch {
	case !d.IsDir():
		if p.ignored.has(slashed, false) && (!p.dart || !project.GeneratedDart(path)) {
			return leftIgnored
		}
		return kept
	case skipDirs[d.Name()] || dartLeftOut(path):
		return leftAlways
	case p.ignored.has(slashed, true):
		return leftIgnored
	case project.IsBuildOutput(ctx, path):
		return leftAlways
	}
	return kept
}

// never says whether no copy holds the entry d, even when mutation.copy
// names it: history, a submodule's .git file among it, or .dart_tool,
// which Ready resolves anew.
func never(d fs.DirEntry) bool {
	return d.Name() == ".git" || d.IsDir() && d.Name() == ".dart_tool"
}

// copyTree copies src, the tree plan copies or a member of it, to dst
// under ctx, linking dependencies, leaving out history, caches, build
// output, what git ignores and the checkouts of other repositories git
// does not track, and bringing
// back what mutation.copy names of what it leaves out, a folder whole.
func copyTree(ctx context.Context, src, dst string, plan *copyPlan) error {
	// out and in are the folders, from the tree, the walk is in that the
	// copy leaves out, as git ignores it alone when ignored, and that
	// mutation.copy brings back; "" for none.
	var out, in string
	ignored := false
	return project.WalkDir(ctx, src, func(ctx context.Context, path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fromSrc, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, fromSrc)
		rel, _ := filepath.Rel(plan.tree, path)
		if out != "" && !within(out, rel) {
			out = ""
		}
		if in != "" && !within(in, rel) {
			in = ""
		}
		switch {
		case path == src:
			return os.MkdirAll(target, 0o755) //nolint:gosec // preserve project directory behavior inside the owner's private 0700 scratch
		case never(d):
			return skipped(d)
		case d.IsDir() && linkDirs[d.Name()]:
			if out != "" && in == "" {
				return filepath.SkipDir
			}
			return skipAfter(os.Symlink(path, target))
		case in != "":
		case plan.rules.brings(path):
			if out != "" || plan.leaves(ctx, path, rel, d) != kept {
				plan.brought = append(plan.brought, rel)
			}
			if d.IsDir() {
				in = rel
			}
		case out != "":
			return plan.outside(path, d, ignored, target)
		default:
			switch plan.leaves(ctx, path, rel, d) {
			case kept:
			case leftIgnored:
				if d.IsDir() && (plan.dart || plan.rules.mayBring(path)) {
					out, ignored = rel, true
					return nil
				}
				return skipped(d)
			case leftAlways:
				if d.IsDir() && plan.rules.mayBring(path) {
					out, ignored = rel, false
					return nil
				}
				return skipped(d)
			}
		}
		return copyEntry(path, target, d, out != "")
	})
}

// outside copies the entry d at path to target when it lies in a folder
// the copy leaves out, ignored when as git ignores it alone, and
// mutation.copy does not name it: only generated Dart, in a Dart package's
// ignored folder, is copied, and only a folder that may hold what
// mutation.copy names or such Dart is walked.
func (p *copyPlan) outside(path string, d fs.DirEntry, ignored bool, target string) error {
	switch {
	case d.IsDir() && p.rules.mayBring(path):
		return nil
	case d.IsDir() && ignored && p.dart && !skipDirs[d.Name()] && !dartLeftOut(path):
		return nil
	case !d.IsDir() && ignored && p.dart && project.GeneratedDart(path):
		return copyEntry(path, target, d, true)
	}
	return skipped(d)
}

// skipped is what the walk returns to leave out the entry d.
func skipped(d fs.DirEntry) error {
	if d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

// copyEntry copies the entry d at path to target, a folder made, a link
// made again, a file copied; with parents, the folders above target first.
func copyEntry(path, target string, d fs.DirEntry, parents bool) error {
	if parents {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // preserve project directory behavior inside the owner's private 0700 scratch
			return err
		}
	}
	switch {
	case d.IsDir():
		return os.MkdirAll(target, 0o755) //nolint:gosec // preserve project directory behavior inside the owner's private 0700 scratch
	case d.Type()&fs.ModeSymlink != 0:
		link, err := os.Readlink(path)
		if err != nil {
			return err
		}
		return os.Symlink(link, target)
	case d.Type().IsRegular():
		return copyFile(path, target)
	}
	return nil
}

func skipAfter(err error) error {
	if err != nil {
		return err
	}
	return filepath.SkipDir
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // copying the person's selected project file is this adapter's purpose
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // copy into the owned worker tree while preserving the source mode
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

type result = mutants.CommandResult

// run executes c inside the worker's copy of c.Root, under parent, within
// limits. A zero limit bounds nothing: a zero timeout waits as long as the
// tests take.
func (w *worker) run(parent context.Context, c Command, limits mutants.Limits) (result, error) {
	root, err := w.copyOf(parent, c)
	if err != nil {
		return result{}, err
	}
	return w.runIn(parent, c, root, limits)
}

// runIn executes c in root, c.Root's place where it runs, under parent,
// within limits: the worker's copy of it, or c.Root itself for a run in
// the project. The runner keeps the output bounded, and kills a run over a
// cap.
func (w *worker) runIn(parent context.Context, c Command, root string, limits mutants.Limits) (result, error) {
	ctx := parent
	if limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(parent, limits.Timeout, processes.ErrCommandTimeout)
		defer cancel()
	}
	var cmd *exec.Cmd
	if c.Shell != "" {
		cmd = shellCommand(ctx, c.Shell, processes.PlatformAtBoundary(w.runner.Platform))
	} else {
		cmd = exec.CommandContext(ctx, c.Args[0], c.Args[1:]...) //nolint:gosec // execute the selected test command with its unchanged argv
	}
	w.prepared(cmd, c, root)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	r, err := w.runner.RunLimited(ctx, cmd, &out, processes.Limits{Output: limits.Output, Memory: limits.Memory})
	return result(r), err
}

// prepared runs cmd, one of c's, in root, the worker's copy of c.Root, from
// c.Dir's place in it, with c's path variable set and the worker's own
// preparation applied.
func (w *worker) prepared(cmd *exec.Cmd, c Command, root string) {
	cmd.Dir = filepath.Join(root, strings.TrimPrefix(c.Dir, c.Root))
	cmd.Env = project.NoBytecodeEnv(nil)
	if c.PathEnv != "" {
		var paths []string
		for _, p := range c.PathDirs {
			paths = append(paths, filepath.Join(root, p))
		}
		if old := os.Getenv(c.PathEnv); old != "" {
			paths = append(paths, old)
		}
		cmd.Env = append(cmd.Env, c.PathEnv+"="+strings.Join(paths, string(os.PathListSeparator)))
	}
	if w.prepare != nil {
		w.prepare(cmd)
	}
}

// withMutant writes src to the worker's copy of path, runs fn under ctx,
// and restores the original text.
func (w *worker) withMutant(ctx context.Context, c Command, path string, original, mutated []byte, fn func(context.Context) (result, error)) (result, error) {
	copyRoot, err := w.copyOf(ctx, c)
	if err != nil {
		return result{}, err
	}
	rel, _ := filepath.Rel(c.Root, path)
	target := filepath.Join(copyRoot, rel)
	if err := os.WriteFile(target, mutated, 0o644); err != nil { //nolint:gosec // preserve the legacy trial mode inside private worker scratch
		return result{}, err
	}
	defer func() {
		_ = os.WriteFile(target, original, 0o644) //nolint:gosec // restore the owned copy with the exact legacy file mode
	}()
	return fn(ctx)
}

func shellCommand(ctx context.Context, line, platform string) *exec.Cmd {
	if platform == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", line) //nolint:gosec // the person explicitly selected a shell test command
	}
	return exec.CommandContext(ctx, "sh", "-c", line) //nolint:gosec // the person explicitly selected a shell test command
}
