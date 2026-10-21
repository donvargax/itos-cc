package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	langtest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// HistoryCommit owns an immutable tree and its actual parent identities.
// Files are root-relative slash paths; strings own their bytes.
type HistoryCommit struct {
	Files   map[string]string
	Parents []string
	// Links are the files committed as symbolic links, by path.
	Links map[string]bool
}

// HistoryRepository is independent fixture Git state. Setup operations snapshot
// actual staged bytes; no expected mutation result, seed or query response enters.
type HistoryRepository struct {
	Commits  map[string]HistoryCommit
	Refs     map[string]string
	Branches map[string]bool
	Branch   string
	Detached string
	Index    map[string]string
	// Config is the repository's own settings, as git config sets them:
	// housekeeping a fixture turns off, which no history query reads.
	Config map[string]string
	// WorkingLinks are the working tree's symbolic links, by root-relative
	// slash path, which git add stages as links; IndexLinks those staged.
	WorkingLinks map[string]bool
	IndexLinks   map[string]bool
	sequence     uint64
}

func NewHistoryRepository() *HistoryRepository {
	return &HistoryRepository{Commits: map[string]HistoryCommit{}, Refs: map[string]string{}, Branches: map[string]bool{"main": true}, Branch: "main", Index: map[string]string{}}
}
func (r *HistoryRepository) head() string {
	if r.Branch != "" {
		return r.Refs[r.Branch]
	}
	return r.Detached
}

// Commit captures staged contents independently of subsequent source/index edits.
func (r *HistoryRepository) Commit(message string) string {
	r.sequence++
	digest := sha256.New()
	_, _ = fmt.Fprint(digest, r.head(), "\x00", message, "\x00", r.sequence)
	for _, path := range slices.Sorted(maps.Keys(r.Index)) {
		_, _ = fmt.Fprint(digest, "\x00", path, "\x00", r.Index[path])
	}
	sum := digest.Sum(nil)
	id := hex.EncodeToString(sum[:20])
	parents := []string(nil)
	if parent := r.head(); parent != "" {
		parents = []string{parent}
	}
	r.Commits[id] = HistoryCommit{Files: maps.Clone(r.Index), Parents: parents, Links: maps.Clone(r.IndexLinks)}
	if r.Branch != "" {
		r.Refs[r.Branch] = id
	} else {
		r.Detached = id
	}
	return id
}

// History reads that state at Dir, with actual current tracked bytes for working
// rename queries. Root and Dir are absolute native fixture identities.
type History struct {
	Root, Dir  string
	Repository *HistoryRepository
	Working    map[string]string
}

var _ mutants.History = History{}

func (h History) Head(ctx context.Context) (string, mutants.HistoryFailure) {
	if ctx.Err() != nil {
		return "", mutants.HistoryCancelled{Cause: context.Cause(ctx)}
	}
	if h.Repository == nil || h.Repository.head() == "" {
		return "", mutants.NoRepository{Reason: "fatal: not a git repository with a HEAD commit (in-memory history)"}
	}
	return h.Repository.head(), nil
}
func (h History) Resolve(ctx context.Context, ref string) (string, mutants.HistoryFailure) {
	if ctx.Err() != nil {
		return "", mutants.HistoryCancelled{Cause: context.Cause(ctx)}
	}
	if h.Repository == nil {
		return "", mutants.NoRepository{Reason: "fatal: not a git repository (in-memory history)"}
	}
	return h.resolveSetup(ref)
}

func (h History) resolveSetup(ref string) (string, mutants.HistoryFailure) {
	if h.Repository == nil {
		return "", mutants.NoRepository{Reason: "fatal: not a git repository (in-memory history)"}
	}
	spelling := ref
	ref = strings.TrimSuffix(ref, "^{commit}")
	base, steps, hasSteps := strings.Cut(ref, "~")
	if strings.HasSuffix(base, "^") {
		base = strings.TrimSuffix(base, "^")
		steps = "1"
		hasSteps = true
	}
	id := base
	if base == "HEAD" {
		id = h.Repository.head()
	} else if value, ok := h.Repository.Refs[base]; ok {
		id = value
	}
	if strings.HasPrefix(ref, "-") || id == "" {
		return "", mutants.BadReference{Ref: spelling}
	}
	if _, ok := h.Repository.Commits[id]; !ok {
		return "", mutants.BadReference{Ref: spelling}
	}
	if hasSteps {
		if steps == "" {
			steps = "1"
		}
		n, err := strconv.Atoi(steps)
		if err != nil || n < 0 {
			return "", mutants.BadReference{Ref: spelling}
		}
		for range n {
			parents := h.Repository.Commits[id].Parents
			if len(parents) == 0 {
				return "", mutants.BadReference{Ref: spelling}
			}
			id = parents[0]
		}
	}
	return id, nil
}
func (h History) ReadCommitted(ctx context.Context, ref, path string) ([]byte, mutants.HistoryFailure) {
	id, failure := h.Resolve(ctx, ref)
	if failure != nil {
		return nil, failure
	}
	value, ok := h.Repository.Commits[id].Files[filepath.ToSlash(path)]
	if !ok {
		return nil, mutants.HistoryFailed{Cause: fmt.Errorf("committed blob %s:%s does not exist", id, path)}
	}
	return []byte(value), nil
}

func (h History) rangeTrees(ctx context.Context, ref string) (map[string]string, map[string]string, mutants.HistoryFailure) {
	base, failure := h.Resolve(ctx, ref)
	if failure != nil {
		return nil, nil, failure
	}
	head, failure := h.Head(ctx)
	if failure != nil {
		return nil, nil, failure
	}
	common := h.mergeBase(base, head)
	if common == "" {
		return nil, nil, mutants.HistoryFailed{Cause: fmt.Errorf("git diff %s...HEAD: no merge base", ref)}
	}
	return h.Repository.Commits[common].Files, h.Repository.Commits[head].Files, nil
}
func (h History) mergeBase(a, b string) string {
	distances := func(start string) map[string]int {
		found := map[string]int{start: 0}
		queue := []string{start}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, p := range h.Repository.Commits[id].Parents {
				if _, ok := found[p]; !ok {
					found[p] = found[id] + 1
					queue = append(queue, p)
				}
			}
		}
		return found
	}
	aa, bb := distances(a), distances(b)
	best := ""
	score := int(^uint(0) >> 1)
	for _, id := range slices.Sorted(maps.Keys(aa)) {
		if d, ok := bb[id]; ok && aa[id]+d < score {
			best = id
			score = aa[id] + d
		}
	}
	return best
}
func (h History) ChangedSince(ctx context.Context, ref string) (map[string]mutants.ChangedFunctions, map[string]string, mutants.HistoryFailure) {
	before, after, failure := h.rangeTrees(ctx, ref)
	if failure != nil {
		return nil, nil, failure
	}
	moves := historyRenames(before, after)
	prior := map[string]string{}
	for old, current := range moves {
		prior[current] = old
	}
	changed, renamed := map[string]mutants.ChangedFunctions{}, map[string]string{}
	surroundings := langtest.Surroundings{Dir: h.Dir, Files: h.Working}
	for _, path := range slices.Sorted(maps.Keys(after)) {
		if ctx.Err() != nil {
			return nil, nil, mutants.HistoryCancelled{Cause: context.Cause(ctx)}
		}
		abs := filepath.Join(h.Root, filepath.FromSlash(path))
		if !historyWithin(h.Dir, abs) || lang.Detect(path) == nil {
			continue
		}
		old := path
		if previous, ok := prior[path]; ok {
			old = previous
			renamed[abs] = filepath.Join(h.Root, filepath.FromSlash(previous))
		}
		if text, exists := before[old]; exists && text == after[path] && old == path {
			continue
		}
		file, err := lang.Parse(ctx, surroundings, lang.Detect(path), abs, []byte(after[path]))
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, mutants.HistoryCancelled{Cause: context.Cause(ctx)}
			}
			return nil, nil, mutants.HistoryFailed{Cause: err}
		}
		// The file as the range started from tells a function whose
		// comments alone changed.
		var base *lang.File
		if text, exists := before[old]; exists && lang.Detect(old) == lang.Detect(path) {
			base, err = lang.Parse(ctx, surroundings, lang.Detect(old), filepath.Join(h.Root, filepath.FromSlash(old)), []byte(text))
			if err != nil {
				file.Close()
				if ctx.Err() != nil {
					return nil, nil, mutants.HistoryCancelled{Cause: context.Cause(ctx)}
				}
				return nil, nil, mutants.HistoryFailed{Cause: err}
			}
		}
		changed[abs] = mutants.ChangedFunctionsOf(file, base, historyRanges(before[old], after[path]), mutants.UnitHash)
		file.Close()
		if base != nil {
			base.Close()
		}
	}
	return changed, renamed, nil
}
func (h History) DeletedSince(ctx context.Context, ref string) ([]string, mutants.HistoryFailure) {
	before, after, failure := h.rangeTrees(ctx, ref)
	if failure != nil {
		return nil, failure
	}
	moves := historyRenames(before, after)
	var deleted []string
	for _, path := range slices.Sorted(maps.Keys(before)) {
		abs := filepath.Join(h.Root, filepath.FromSlash(path))
		if _, present := after[path]; !present && moves[path] == "" && lang.Detect(path) != nil && historyWithin(h.Dir, abs) {
			deleted = append(deleted, abs)
		}
	}
	return deleted, nil
}
func (h History) RenamesSince(ctx context.Context, ref string) (map[string]string, mutants.HistoryFailure) {
	before, after, failure := h.rangeTrees(ctx, ref)
	if failure != nil {
		return nil, failure
	}
	return h.absoluteRenames(before, after), nil
}
func (h History) RenamesUncommitted(ctx context.Context) (map[string]string, mutants.HistoryFailure) {
	id, failure := h.Head(ctx)
	if failure != nil {
		return nil, failure
	}
	tracked := map[string]string{}
	for path := range h.Repository.Index {
		if text, ok := h.Working[filepath.Join(h.Root, filepath.FromSlash(path))]; ok {
			tracked[path] = text
		}
	}
	return h.absoluteRenames(h.Repository.Commits[id].Files, tracked), nil
}
func (h History) absoluteRenames(before, after map[string]string) map[string]string {
	out := map[string]string{}
	for old, current := range historyRenames(before, after) {
		out[filepath.Join(h.Root, filepath.FromSlash(old))] = filepath.Join(h.Root, filepath.FromSlash(current))
	}
	return out
}
func historyWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
