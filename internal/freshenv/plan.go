package freshenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	fileselection "github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/project"
)

// FreshPlanVersion names the stable static selection algorithm. It changes
// whenever the identity or ranking inputs change.
const FreshPlanVersion = mutants.FreshPlanVersion

type FreshPlan = mutants.FreshPlan
type FreshCandidate = mutants.FreshCandidate
type FreshUnit = mutants.FreshUnit

// Errors a fresh plan wraps, so a caller can tell what refused it.
var (
	// ErrFreshRepository: no Git repository or no resolvable HEAD commit.
	ErrFreshRepository = errors.New("no Git repository with a resolvable HEAD")
	// ErrFreshSince: the --since reference names no commit.
	ErrFreshSince = errors.New("bad --since reference")
	// ErrFreshPath: a path that is not root-relative or selects nothing.
	ErrFreshPath = errors.New("bad fresh plan path")
	// ErrFreshUnsupported: committed content the plan does not support.
	ErrFreshUnsupported = errors.New("unsupported committed scope")
	// ErrFreshScratch: the private scratch directory the frozen inputs go
	// in is unusable: outside both the checkout and its git directory, or
	// reached through a symlink or a file.
	ErrFreshScratch = errors.New("unusable private scratch directory")
)

// Close removes the plan's frozen committed-input tree.
func releasePlan(p *FreshPlan) error {
	if p == nil || p.FrozenRoot == "" {
		return nil
	}
	root := p.FrozenRoot
	p.FrozenRoot = ""
	return os.RemoveAll(root)
}

// PlanFresh inventories and globally selects static sites from
// committed inputs. It runs Git metadata commands only: no mutation, test or
// measurement commands. Cancellation owns those Git process trees and removes
// the private export. paths, when non-empty, narrow root-relative committed
// paths; since, when non-empty, further restricts candidates to changed
// functions. The sources the committed itos-cc.yaml excludes are no
// candidates, whatever paths name; a committed itos-cc.yaml that cannot be
// read is its *config.InvalidError.
func (e Environment) plan(ctx context.Context, repoRoot string, paths []string, since string, count int, seed string) (*FreshPlan, error) {
	if count <= 0 {
		return nil, errors.New("fresh plan count must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if repoRoot == "" {
		var err error
		repoRoot, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	rootOut, err := e.gitAt(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: resolve repository root: %w", ErrFreshRepository, err)
	}
	root, err = filepath.Abs(strings.TrimSpace(string(rootOut)))
	if err != nil {
		return nil, err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, fmt.Errorf("resolve physical repository root: %w", err)
	}
	headOut, err := e.gitAt(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, fmt.Errorf("%w: resolve HEAD: %w", ErrFreshRepository, err)
	}
	head := strings.TrimSpace(string(headOut))
	if head == "" {
		return nil, fmt.Errorf("%w: resolve HEAD: empty commit id", ErrFreshRepository)
	}
	if seed == "" {
		seed = head
	}
	selectedPaths, err := mutants.CanonicalFreshPaths(paths)
	if err != nil {
		return nil, err
	}
	var changed map[string][]mutants.LineRange
	var sinceBase string
	if since != "" {
		changed, sinceBase, err = e.changedAt(ctx, root, since, head)
		if err != nil {
			return nil, err
		}
	}
	scratchParent, scratchBase, err := e.privateScratch(ctx, root)
	if err != nil {
		return nil, err
	}
	if err := makePrivateDirs(scratchBase, scratchParent); err != nil {
		return nil, fmt.Errorf("%w: create private Git scratch directory: %w", ErrFreshScratch, err)
	}
	frozen, err := os.MkdirTemp(scratchParent, "fresh-plan-")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFreshScratch, err)
	}
	plan := &FreshPlan{Root: root, FrozenRoot: frozen, Commit: head, SinceRef: since, SinceBase: sinceBase,
		Seed: seed, Algorithm: FreshPlanVersion}
	if changed != nil {
		plan.Changed = map[string][]mutants.LineRange{}
		for rel, ranges := range changed {
			for _, r := range ranges {
				if !r.Deletion {
					plan.Changed[rel] = append(plan.Changed[rel], mutants.LineRange{Start: r.Start, End: r.End})
				}
			}
		}
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = releasePlan(plan)
		}
	}()
	tracked, err := e.exportCommit(ctx, root, head, frozen)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadFrom(frozen)
	if err != nil {
		return nil, err
	}
	exclude := fileselection.Exclusion(cfg.Selection)
	for chosen := range selectedPaths {
		found := false
		for _, rel := range tracked {
			if mutants.SelectsFreshPath(rel, map[string]bool{chosen: true}) && lang.Detect(rel) != nil {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: %q selects no committed supported source", ErrFreshPath, chosen)
		}
	}
	buildOutput, buildOutputErr, err := e.committedBuildOutput(ctx, frozen)
	if err != nil {
		return nil, err
	}
	var roots []string
	if len(selectedPaths) == 0 {
		roots = []string{frozen}
	} else {
		for chosen := range selectedPaths {
			if chosen == "." {
				roots = append(roots, frozen)
			} else {
				roots = append(roots, filepath.Join(frozen, filepath.FromSlash(chosen)))
			}
		}
		sort.Strings(roots)
	}
	discovered, err := project.DiscoverWithBuildOutput(ctx, roots, buildOutput)
	if err != nil {
		return nil, fmt.Errorf("discover committed mutation targets: %w", err)
	}
	plan.Files = discovered
	if len(roots) != 1 || roots[0] != frozen {
		if plan.Files, err = project.DiscoverWithBuildOutput(ctx, []string{frozen}, buildOutput); err != nil {
			return nil, fmt.Errorf("discover committed files: %w", err)
		}
	}
	if err := buildOutputErr(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := mutants.AdmitFresh(ctx, plan, e.Tree, e.Surroundings, discovered.Sources, tracked, exclude, changed, count); err != nil {
		return nil, err
	}
	cleanup = false
	return plan, nil
}

type gitCommandError struct {
	Args     []string
	ExitCode int
	Detail   string
}

func (e *gitCommandError) Error() string {
	return fmt.Sprintf("git %s exited %d: %s", strings.Join(e.Args, " "), e.ExitCode, e.Detail)
}

func (e Environment) gitAt(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return e.gitAtEnv(ctx, dir, nil, args...)
}

func (e Environment) gitAtEnv(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // run only the fixed Git program with the admitted metadata/export arguments under the owned executor
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	result, err := e.Command(ctx, cmd)
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), detail, err)
	}
	if result.Cancelled {
		return nil, fmt.Errorf("git %s cancelled: %w", strings.Join(args, " "), ctx.Err())
	}
	if !result.Passed {
		return nil, &gitCommandError{Args: slices.Clone(args), ExitCode: result.ExitCode, Detail: strings.TrimSpace(stderr.String())}
	}
	return stdout.Bytes(), nil
}

func (e Environment) committedBuildOutput(ctx context.Context, frozen string) (func(ctx context.Context, dir string) bool, func() error, error) {
	gitDir := filepath.Join(frozen, ".git")
	emptyTemplate := filepath.Join(frozen, ".fresh-plan-empty-template")
	if err := os.Mkdir(emptyTemplate, 0700); err != nil {
		return nil, nil, err
	}
	emptyConfig := filepath.Join(frozen, ".fresh-plan-empty-config")
	if err := os.WriteFile(emptyConfig, nil, 0600); err != nil { //nolint:gosec // isolate Git configuration in this plan's validated private scratch
		return nil, nil, err
	}
	initEnv := cleanGitEnvironment(emptyConfig)
	if _, err := e.gitAtEnv(ctx, frozen, initEnv, "init", "--quiet", "--bare", "--template", emptyTemplate, gitDir); err != nil {
		return nil, nil, fmt.Errorf("initialize isolated committed-ignore metadata: %w", err)
	}
	emptyExcludes := filepath.Join(frozen, ".fresh-plan-empty-excludes")
	if err := os.WriteFile(emptyExcludes, nil, 0600); err != nil { //nolint:gosec // supply an empty ignore file only inside this plan's private scratch
		return nil, nil, err
	}
	env := cleanGitEnvironment(emptyConfig)
	env = append(env, "GIT_DIR="+gitDir, "GIT_WORK_TREE="+frozen)
	var firstErr error
	classify := func(ctx context.Context, dir string) bool {
		rel, err := filepath.Rel(frozen, dir)
		if err != nil {
			firstErr = err
			return false
		}
		rel = filepath.ToSlash(rel)
		_, err = e.gitAtEnv(ctx, frozen, env, "-c", "core.excludesFile="+emptyExcludes,
			"check-ignore", "--quiet", "--no-index", "--", rel)
		if err == nil {
			return true
		}
		var commandErr *gitCommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
			return false
		}
		firstErr = err
		return false
	}
	return func(ctx context.Context, dir string) bool {
		return project.IsBuildOutputWithIgnore(ctx, dir, classify)
	}, func() error { return firstErr }, nil
}

func cleanGitEnvironment(emptyConfig string) []string {
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+emptyConfig,
	)
}

type treeEntry struct{ mode, kind, oid, name string }

// exportCommit writes only Git blob contents to newly-created regular files.
// Symlinks and gitlinks are rejected rather than followed or materialized.
func (e Environment) exportCommit(ctx context.Context, root, commit, destination string) ([]string, error) {
	out, err := e.gitAt(ctx, root, "ls-tree", "-rz", "-r", "--full-tree", "--full-name", commit)
	if err != nil {
		return nil, err
	}
	entries, err := parseTree(out)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		switch {
		case entry.kind == "commit" || entry.mode == "160000":
			return nil, fmt.Errorf("%w: submodule %q", ErrFreshUnsupported, entry.name)
		case entry.mode == "120000":
			return nil, fmt.Errorf("%w: committed symlink %q", ErrFreshUnsupported, entry.name)
		case entry.kind != "blob" || (entry.mode != "100644" && entry.mode != "100755"):
			return nil, fmt.Errorf("%w: Git tree entry %q (mode %s, type %s)", ErrFreshUnsupported, entry.name, entry.mode, entry.kind)
		}
		clean := path.Clean(entry.name)
		if clean == "." || clean != entry.name || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(entry.name, "\\") {
			return nil, fmt.Errorf("%w: committed path escapes repository: %q", ErrFreshUnsupported, entry.name)
		}
		blob, err := e.gitAt(ctx, root, "cat-file", "blob", entry.oid)
		if err != nil {
			return nil, fmt.Errorf("read committed blob %s (%s): %w", entry.name, entry.oid, err)
		}
		target := filepath.Join(destination, filepath.FromSlash(clean))
		if err := ensureInside(destination, target); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		mode := os.FileMode(0600)
		if entry.mode == "100755" {
			mode = 0700
		}
		if err := os.WriteFile(target, blob, mode); err != nil { //nolint:gosec // materialize an admitted committed blob only inside the validated private export, preserving executable mode
			return nil, err
		}
		names = append(names, clean)
	}
	sort.Strings(names)
	return names, nil
}

func parseTree(data []byte) ([]treeEntry, error) {
	var entries []treeEntry
	for _, record := range bytes.Split(data, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("malformed git ls-tree record %q", record)
		}
		fields := strings.Fields(string(meta))
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed git ls-tree metadata %q", meta)
		}
		entries = append(entries, treeEntry{mode: fields[0], kind: fields[1], oid: fields[2], name: string(name)})
	}
	return entries, nil
}

func ensureInside(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("export path escapes private tree: %q", target)
	}
	return nil
}

// privateScratch is where a plan's frozen inputs go, the directory git
// rev-parse --git-path itos names for the checkout at root, and the base it
// lies under, below which makePrivateDirs creates it: root itself, which
// holds .git/itos in a main checkout, or else the checkout's own git
// directory, which a linked worktree keeps under the main repository's
// .git/worktrees and which is as private as .git is. The scratch path is
// returned under that base's physical path. A path outside both is
// ErrFreshScratch.
func (e Environment) privateScratch(ctx context.Context, root string) (target, base string, err error) {
	out, err := e.gitAt(ctx, root, "rev-parse", "--git-path", "itos")
	if err != nil {
		return "", "", fmt.Errorf("resolve private Git scratch path: %w", err)
	}
	target = strings.TrimSpace(string(out))
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	if ensureInside(root, target) == nil {
		return target, root, nil
	}
	out, err = e.gitAt(ctx, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve the checkout's git directory: %w", err)
	}
	gitDir := filepath.Clean(strings.TrimSpace(string(out)))
	physical, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve the checkout's git directory: %w", ErrFreshScratch, err)
	}
	for _, dir := range []string{gitDir, physical} {
		if ensureInside(dir, target) == nil {
			rel, err := filepath.Rel(dir, target)
			if err != nil {
				return "", "", err
			}
			return filepath.Join(physical, rel), physical, nil
		}
	}
	return "", "", fmt.Errorf("%w: %q lies outside both the checkout %q and its git directory %q", ErrFreshScratch, target, root, physical)
}

func makePrivateDirs(root, target string) error {
	if err := ensureInside(root, target); err != nil {
		return fmt.Errorf("scratch path escapes repository: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("scratch path component is a symlink: %q", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("scratch path component is not a regular directory: %q", current)
		}
	}
	return nil
}

// lineRange is a range of line numbers, both ends included; deletion
// marks the two lines either side of a hunk that only deleted lines, which
// admit their functions but are no line the range added or changed.
func (e Environment) changedAt(ctx context.Context, root, ref, head string) (map[string][]mutants.LineRange, string, error) {
	if strings.HasPrefix(ref, "-") {
		return nil, "", fmt.Errorf("%w: invalid --since ref %q", ErrFreshSince, ref)
	}
	baseOut, err := e.gitAt(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return nil, "", fmt.Errorf("%w: --since %s is not a commit: %w", ErrFreshSince, ref, err)
	}
	base := strings.TrimSpace(string(baseOut))
	if base == "" {
		return nil, "", fmt.Errorf("%w: --since %s resolved to an empty commit id", ErrFreshSince, ref)
	}
	args := []string{"-c", "core.quotePath=false", "diff", "--name-status", "-z", "-M", "--no-ext-diff", "--no-textconv", base + "..." + head}
	out, err := e.gitAt(ctx, root, args...)
	if err != nil {
		return nil, "", err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	changed := make(map[string][]mutants.LineRange)
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" {
			continue
		}
		var name, oldName string
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+1 >= len(fields) {
				return nil, "", errors.New("malformed git name-status rename record")
			}
			oldName, name = fields[i], fields[i+1]
			i += 2
		} else {
			if i >= len(fields) {
				return nil, "", errors.New("malformed git name-status record")
			}
			name = fields[i]
			i++
		}
		if status[0] == 'D' {
			continue
		}
		if lang.Detect(name) == nil {
			continue
		}
		diffArgs := []string{"--literal-pathspecs", "-c", "core.quotePath=false", "diff", "-M", "--unified=0", "--no-color", "--no-ext-diff", "--no-textconv", base + "..." + head, "--"}
		if oldName != "" {
			diffArgs = append(diffArgs, oldName)
		}
		diffArgs = append(diffArgs, name)
		patch, err := e.gitAt(ctx, root, diffArgs...)
		if err != nil {
			return nil, "", err
		}
		changed[name] = mutants.HunkRanges(string(patch))
	}
	return changed, base, nil
}
