package mutate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// FreshPlanVersion names the stable static selection algorithm. It changes
// whenever the identity or ranking inputs change.
const FreshPlanVersion = "fresh-plan-v2"

// FreshPlan is a cache-independent selection of committed mutation sites.
// FrozenRoot holds the repository's committed tracked inputs only; external
// tools, dependencies and environment are outside this freeze. FrozenRoot is
// private scratch space owned by the plan and is removed by Close.
type FreshPlan struct {
	Root       string
	FrozenRoot string
	Commit     string
	Seed       string
	Algorithm  string
	Eligible   []FreshCandidate
	Selected   []FreshCandidate
	Omitted    []FreshCandidate
}

// FreshCandidate identifies a static mutation site without relying on cached
// outcomes or on the unit's display name alone.
type FreshCandidate struct {
	Identity     string
	Path         string
	Function     string
	UnitIdentity string
	Site         Site
	Rank         string
}

// Close removes the plan's frozen committed-input tree.
func (p *FreshPlan) Close() error {
	if p == nil || p.FrozenRoot == "" {
		return nil
	}
	root := p.FrozenRoot
	p.FrozenRoot = ""
	return os.RemoveAll(root)
}

// PlanFresh is the background-context adapter for PlanFreshContext.
func PlanFresh(repoRoot string, paths []string, since string, count int, seed string) (*FreshPlan, error) {
	return PlanFreshContext(context.Background(), repoRoot, paths, since, count, seed)
}

// PlanFreshContext inventories and globally selects static sites from
// committed inputs. It runs Git metadata commands only: no mutation, test or
// measurement commands. Cancellation owns those Git process trees and removes
// the private export. paths, when non-empty, narrow root-relative committed
// paths; since, when non-empty, further restricts candidates to changed
// functions.
func PlanFreshContext(ctx context.Context, repoRoot string, paths []string, since string, count int, seed string) (*FreshPlan, error) {
	if ctx == nil {
		return nil, errors.New("fresh plan context must not be nil")
	}
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
	rootOut, err := gitAt(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	root, err = filepath.Abs(strings.TrimSpace(string(rootOut)))
	if err != nil {
		return nil, err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, fmt.Errorf("resolve physical repository root: %w", err)
	}
	headOut, err := gitAt(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	head := strings.TrimSpace(string(headOut))
	if head == "" {
		return nil, errors.New("resolve HEAD: empty commit id")
	}
	if seed == "" {
		seed = head
	}
	selectedPaths, err := canonicalPaths(paths)
	if err != nil {
		return nil, err
	}
	var changed map[string][]lineRange
	if since != "" {
		changed, err = changedAt(ctx, root, since, head)
		if err != nil {
			return nil, err
		}
	}
	gitScratch, err := gitAt(ctx, root, "rev-parse", "--git-path", "itos")
	if err != nil {
		return nil, fmt.Errorf("resolve private Git scratch path: %w", err)
	}
	scratchParent := strings.TrimSpace(string(gitScratch))
	if !filepath.IsAbs(scratchParent) {
		scratchParent = filepath.Join(root, scratchParent)
	}
	if err := makePrivateDirs(root, scratchParent); err != nil {
		return nil, fmt.Errorf("create private Git scratch directory: %w", err)
	}
	frozen, err := os.MkdirTemp(scratchParent, "fresh-plan-")
	if err != nil {
		return nil, err
	}
	plan := &FreshPlan{Root: root, FrozenRoot: frozen, Commit: head, Seed: seed, Algorithm: FreshPlanVersion}
	cleanup := true
	defer func() {
		if cleanup {
			_ = plan.Close()
		}
	}()
	tracked, err := exportCommit(ctx, root, head, frozen)
	if err != nil {
		return nil, err
	}
	for chosen := range selectedPaths {
		found := false
		for _, rel := range tracked {
			if selectedPath(rel, map[string]bool{chosen: true}) && lang.Detect(rel) != nil {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("fresh plan path %q selects no committed supported source", chosen)
		}
	}
	buildOutput, buildOutputErr, err := committedBuildOutput(ctx, frozen)
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
	discovered, err := project.DiscoverWithBuildOutput(roots, buildOutput)
	if err != nil {
		return nil, fmt.Errorf("discover committed mutation targets: %w", err)
	}
	if err := buildOutputErr(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	committed := make(map[string]bool, len(tracked))
	for _, rel := range tracked {
		committed[rel] = true
	}
	var candidates []FreshCandidate
	for _, discoveredPath := range discovered.Sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(frozen, discoveredPath)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if !committed[rel] {
			return nil, fmt.Errorf("discovery escaped committed tree: %q", rel)
		}
		ranges, isChanged := changed[rel]
		if changed != nil && !isChanged {
			continue
		}
		frozenPath := filepath.Join(frozen, filepath.FromSlash(rel))
		source, err := os.ReadFile(frozenPath)
		if err != nil {
			return nil, fmt.Errorf("read committed %s: %w", rel, err)
		}
		file, err := lang.Parse(lang.Detect(rel), frozenPath, source)
		if err != nil {
			return nil, fmt.Errorf("parse committed %s: %w", rel, err)
		}
		sites := Sites(file)
		for _, site := range sites {
			unit := file.Units[site.Unit]
			if changed != nil && !overlapsRanges(ranges, unit.StartLine, unit.EndLine) {
				continue
			}
			function := stableFunction(file, unit, rel, committed)
			unitIdentity := fmt.Sprintf("%s@%d:%d:%s", function, unit.StartLine, unit.EndLine, UnitHash(file, unit))
			identity := fmt.Sprintf("%s:%s:%s", rel, unitIdentity, site.Key())
			rank := freshRank(seed, rel, identity)
			candidates = append(candidates, FreshCandidate{Identity: identity, Path: rel, Function: function,
				UnitIdentity: unitIdentity, Site: site, Rank: rank})
		}
		file.Close()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sortFreshCandidates(candidates)
	plan.Eligible = slices.Clone(candidates)
	limit := min(count, len(candidates))
	plan.Selected = slices.Clone(candidates[:limit])
	plan.Omitted = slices.Clone(candidates[limit:])
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

func gitAt(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitAtEnv(ctx, dir, nil, args...)
}

func gitAtEnv(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	var runner commandRunner
	result, err := runner.run(ctx, cmd, &stderr)
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), detail, err)
	}
	if result.cancelled {
		return nil, fmt.Errorf("git %s cancelled: %w", strings.Join(args, " "), ctx.Err())
	}
	if !result.passed {
		return nil, &gitCommandError{Args: slices.Clone(args), ExitCode: result.exitCode, Detail: strings.TrimSpace(stderr.String())}
	}
	return stdout.Bytes(), nil
}

func canonicalPaths(paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make(map[string]bool, len(paths))
	for _, raw := range paths {
		if filepath.IsAbs(raw) {
			return nil, fmt.Errorf("fresh plan path must be root-relative: %q", raw)
		}
		rel := filepath.ToSlash(filepath.Clean(raw))
		if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") {
			return nil, fmt.Errorf("fresh plan path escapes or ambiguously names the repository: %q", raw)
		}
		out[path.Clean(rel)] = true
	}
	return out, nil
}

func selectedPath(rel string, selected map[string]bool) bool {
	if len(selected) == 0 {
		return true
	}
	for chosen := range selected {
		if chosen == "." || rel == chosen || strings.HasPrefix(rel, strings.TrimSuffix(chosen, "/")+"/") {
			return true
		}
	}
	return false
}

func committedBuildOutput(ctx context.Context, frozen string) (func(string) bool, func() error, error) {
	gitDir := filepath.Join(frozen, ".git")
	emptyTemplate := filepath.Join(frozen, ".fresh-plan-empty-template")
	if err := os.Mkdir(emptyTemplate, 0700); err != nil {
		return nil, nil, err
	}
	emptyConfig := filepath.Join(frozen, ".fresh-plan-empty-config")
	if err := os.WriteFile(emptyConfig, nil, 0600); err != nil {
		return nil, nil, err
	}
	initEnv := cleanGitEnvironment(emptyConfig)
	if _, err := gitAtEnv(ctx, frozen, initEnv, "init", "--quiet", "--bare", "--template", emptyTemplate, gitDir); err != nil {
		return nil, nil, fmt.Errorf("initialize isolated committed-ignore metadata: %w", err)
	}
	emptyExcludes := filepath.Join(frozen, ".fresh-plan-empty-excludes")
	if err := os.WriteFile(emptyExcludes, nil, 0600); err != nil {
		return nil, nil, err
	}
	env := cleanGitEnvironment(emptyConfig)
	env = append(env, "GIT_DIR="+gitDir, "GIT_WORK_TREE="+frozen)
	var firstErr error
	classify := func(dir string) bool {
		rel, err := filepath.Rel(frozen, dir)
		if err != nil {
			firstErr = err
			return false
		}
		rel = filepath.ToSlash(rel)
		_, err = gitAtEnv(ctx, frozen, env, "-c", "core.excludesFile="+emptyExcludes,
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
	return func(dir string) bool {
		return project.IsBuildOutputWithIgnore(dir, classify)
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

func stableFunction(file *lang.File, unit lang.Unit, rel string, committed map[string]bool) string {
	namespace := unit.Namespace
	switch file.Spec.Name {
	case "python":
		namespace = stableModuleNamespace(rel, committed, "pyproject.toml", "setup.py", "setup.cfg")
		namespace = strings.TrimSuffix(namespace, ".__init__")
	case "typescript":
		namespace = stableModuleNamespace(rel, committed, "package.json", "tsconfig.json")
	}
	return namespace + "#" + unit.Name
}

func stableModuleNamespace(rel string, committed map[string]bool, markers ...string) string {
	base := rel
	for dir := path.Dir(rel); ; dir = path.Dir(dir) {
		found := false
		for _, marker := range markers {
			if committed[path.Join(dir, marker)] {
				base = strings.TrimPrefix(rel, dir+"/")
				found = true
				break
			}
		}
		if found {
			break
		}
		if dir == "." {
			break
		}
	}
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.TrimPrefix(base, "src/")
	base = strings.TrimPrefix(base, "lib/")
	return strings.ReplaceAll(base, "/", ".")
}

type treeEntry struct{ mode, kind, oid, name string }

// exportCommit writes only Git blob contents to newly-created regular files.
// Symlinks and gitlinks are rejected rather than followed or materialized.
func exportCommit(ctx context.Context, root, commit, destination string) ([]string, error) {
	out, err := gitAt(ctx, root, "ls-tree", "-rz", "-r", "--full-tree", "--full-name", commit)
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
			return nil, fmt.Errorf("fresh plan does not support submodule %q", entry.name)
		case entry.mode == "120000":
			return nil, fmt.Errorf("fresh plan does not support committed symlink %q", entry.name)
		case entry.kind != "blob" || (entry.mode != "100644" && entry.mode != "100755"):
			return nil, fmt.Errorf("fresh plan does not support Git tree entry %q (mode %s, type %s)", entry.name, entry.mode, entry.kind)
		}
		clean := path.Clean(entry.name)
		if clean == "." || clean != entry.name || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(entry.name, "\\") {
			return nil, fmt.Errorf("committed path escapes repository: %q", entry.name)
		}
		blob, err := gitAt(ctx, root, "cat-file", "blob", entry.oid)
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
		if err := os.WriteFile(target, blob, mode); err != nil {
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

type lineRange struct{ start, end int }

func changedAt(ctx context.Context, root, ref, head string) (map[string][]lineRange, error) {
	if strings.HasPrefix(ref, "-") {
		return nil, fmt.Errorf("invalid --since ref %q", ref)
	}
	if _, err := gitAt(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--since %s is not a commit", ref)
	}
	args := []string{"-c", "core.quotePath=false", "diff", "--name-status", "-z", "-M", "--no-ext-diff", "--no-textconv", ref + "..." + head}
	out, err := gitAt(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	changed := make(map[string][]lineRange)
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" {
			continue
		}
		var name, oldName string
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+1 >= len(fields) {
				return nil, errors.New("malformed git name-status rename record")
			}
			oldName, name = fields[i], fields[i+1]
			i += 2
		} else {
			if i >= len(fields) {
				return nil, errors.New("malformed git name-status record")
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
		diffArgs := []string{"--literal-pathspecs", "-c", "core.quotePath=false", "diff", "-M", "--unified=0", "--no-color", "--no-ext-diff", "--no-textconv", ref + "..." + head, "--"}
		if oldName != "" {
			diffArgs = append(diffArgs, oldName)
		}
		diffArgs = append(diffArgs, name)
		patch, err := gitAt(ctx, root, diffArgs...)
		if err != nil {
			return nil, err
		}
		changed[name] = hunkRanges(string(patch))
	}
	return changed, nil
}

func hunkRanges(patch string) []lineRange {
	var ranges []lineRange
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "@@ ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
			continue
		}
		startText, countText, hasCount := strings.Cut(fields[2][1:], ",")
		start, err := strconv.Atoi(startText)
		if err != nil {
			continue
		}
		count := 1
		if hasCount {
			count, err = strconv.Atoi(countText)
			if err != nil {
				continue
			}
		}
		if count == 0 {
			ranges = append(ranges, lineRange{start, start + 1})
		} else {
			ranges = append(ranges, lineRange{start, start + count - 1})
		}
	}
	return ranges
}

func overlapsRanges(ranges []lineRange, start, end int) bool {
	for _, r := range ranges {
		if r.start <= end && r.end >= start {
			return true
		}
	}
	return false
}

func freshRank(seed, rel, identity string) string {
	sum := sha256.Sum256([]byte(FreshPlanVersion + "\x00" + seed + "\x00" + filepath.ToSlash(rel) + "\x00" + identity))
	return hex.EncodeToString(sum[:])
}

func sortFreshCandidates(candidates []FreshCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		return candidates[i].Identity < candidates[j].Identity
	})
}
