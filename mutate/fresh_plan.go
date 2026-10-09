package mutate

import (
	"bytes"
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
)

// FreshPlanVersion names the stable static selection algorithm. It changes
// whenever the identity or ranking inputs change.
const FreshPlanVersion = "fresh-plan-v1"

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

// PlanFresh inventories and globally selects static sites from committed
// inputs. It does not inspect mutation snapshots, run commands, or mutate
// source files. paths, when non-empty, narrow root-relative committed paths;
// since, when non-empty, further restricts candidates to changed functions.
func PlanFresh(repoRoot string, paths []string, since string, count int, seed string) (*FreshPlan, error) {
	if count <= 0 {
		return nil, errors.New("fresh plan count must be positive")
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
	rootOut, err := gitAt(root, "rev-parse", "--show-toplevel")
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
	headOut, err := gitAt(root, "rev-parse", "--verify", "HEAD^{commit}")
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
	selectedPaths, err := canonicalPaths(root, paths)
	if err != nil {
		return nil, err
	}
	var changed map[string][]lineRange
	if since != "" {
		changed, err = changedAt(root, since, head)
		if err != nil {
			return nil, err
		}
	}
	gitScratch, err := gitAt(root, "rev-parse", "--git-path", "itos")
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
	tracked, err := exportCommit(root, head, frozen)
	if err != nil {
		return nil, err
	}
	for chosen := range selectedPaths {
		found := false
		for _, rel := range tracked {
			if (rel == chosen || strings.HasPrefix(rel, strings.TrimSuffix(chosen, "/")+"/")) && lang.Detect(rel) != nil {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("fresh plan path %q selects no committed supported source", chosen)
		}
	}
	var candidates []FreshCandidate
	for _, rel := range tracked {
		if lang.Detect(rel) == nil || !selectedPath(rel, selectedPaths) {
			continue
		}
		ranges, isChanged := changed[rel]
		if changed != nil && !isChanged {
			continue
		}
		file, err := lang.ParseFile(filepath.Join(frozen, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("parse committed %s: %w", rel, err)
		}
		sites := Sites(file)
		for _, site := range sites {
			unit := file.Units[site.Unit]
			if changed != nil && !overlapsRanges(ranges, unit.StartLine, unit.EndLine) {
				continue
			}
			function := unit.Namespace + "#" + unit.Name
			unitIdentity := fmt.Sprintf("%s@%d:%d:%s", function, unit.StartLine, unit.EndLine, UnitHash(file, unit))
			identity := fmt.Sprintf("%s:%s:%s", rel, unitIdentity, site.Key())
			rank := freshRank(seed, rel, identity)
			candidates = append(candidates, FreshCandidate{Identity: identity, Path: rel, Function: function,
				UnitIdentity: unitIdentity, Site: site, Rank: rank})
		}
		file.Close()
	}
	sortFreshCandidates(candidates)
	plan.Eligible = slices.Clone(candidates)
	limit := min(count, len(candidates))
	plan.Selected = slices.Clone(candidates[:limit])
	plan.Omitted = slices.Clone(candidates[limit:])
	cleanup = false
	return plan, nil
}

func gitAt(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
	}
	return out, nil
}

func canonicalPaths(root string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make(map[string]bool, len(paths))
	for _, raw := range paths {
		if filepath.IsAbs(raw) {
			return nil, fmt.Errorf("fresh plan path must be root-relative: %q", raw)
		}
		rel := filepath.ToSlash(filepath.Clean(raw))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") {
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
		if rel == chosen || strings.HasPrefix(rel, strings.TrimSuffix(chosen, "/")+"/") {
			return true
		}
	}
	return false
}

type treeEntry struct{ mode, kind, oid, name string }

// exportCommit writes only Git blob contents to newly-created regular files.
// Symlinks and gitlinks are rejected rather than followed or materialized.
func exportCommit(root, commit, destination string) ([]string, error) {
	out, err := gitAt(root, "ls-tree", "-rz", "-r", "--full-tree", "--full-name", commit)
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
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(entry.name, "\\") {
			return nil, fmt.Errorf("committed path escapes repository: %q", entry.name)
		}
		blob, err := gitAt(root, "cat-file", "blob", entry.oid)
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

func changedAt(root, ref, head string) (map[string][]lineRange, error) {
	if strings.HasPrefix(ref, "-") {
		return nil, fmt.Errorf("invalid --since ref %q", ref)
	}
	if _, err := gitAt(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--since %s is not a commit", ref)
	}
	args := []string{"-c", "core.quotePath=false", "diff", "--name-status", "-z", "-M", "--no-ext-diff", "--no-textconv", ref + "..." + head}
	out, err := gitAt(root, args...)
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
		patch, err := gitAt(root, diffArgs...)
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
