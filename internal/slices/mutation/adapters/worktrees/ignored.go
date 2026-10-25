package worktrees

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/donvargax/itos-cc/internal/project"
)

// What a worker's copy leaves out as git ignores it (slice-43, q-69 and
// q-92): a file a coverage run left in the project, which runs in the
// project's own folder, or a stray build or tool, never reaches a copy.
// Ignored is what the committed .gitignore files ignore and git does not
// track, so copies are the same on every machine: a tracked file in an
// ignored folder stays. The dependency folders a copy links stay linked,
// generated Dart stays (q-77), and mutation.copy in itos-cc.yaml brings
// back the paths a project's tests need. A tree in no git repository, or
// where git fails, is copied as before. A folder holding its own .git, a
// folder or a file as a linked worktree has, that git does not track is
// another repository, left out as an ignored folder is; a submodule, which
// git tracks, is copied, never its .git. A counted run's pool asks git nothing: its frozen export already
// holds the commit's tracked files alone.

// copyRules are what a pool's copies leave out as git ignores it, asked
// once for each tree, by tree, and what they bring back of what they leave
// out: mutation.copy's globs, slash-separated from root, the project root.
type copyRules struct {
	root  string
	globs []string
	mu    sync.Mutex
	trees map[string]ignoredPaths
}

// ignoredPaths are what git ignores in a tree, by each path slash-separated
// from it, a folder's ending in /, as git ls-files folds them; nil when git
// cannot say, as in no repository.
type ignoredPaths map[string]bool

// ignored is what git ignores in tree, asked the first time a copy of it
// is made.
func (r *copyRules) ignored(ctx context.Context, tree string) ignoredPaths {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if paths, ok := r.trees[tree]; ok {
		return paths
	}
	paths := gitIgnored(ctx, tree)
	r.trees[tree] = paths
	return paths
}

// gitIgnored is what the committed .gitignore files ignore in tree and git
// does not track, as git ls-files lists it, and each checkout of another
// repository in it that git does not track, as a folder; nil when git
// fails, such as outside a repository, or ignores tree itself. Neither the
// repository's info/exclude nor a global excludes file counts, so every
// machine leaves out the same paths. A submodule, which git tracks, is no
// such checkout.
func gitIgnored(ctx context.Context, tree string) ignoredPaths {
	paths, ok := gitListed(ctx, tree, "--ignored", "--directory")
	if !ok {
		return nil
	}
	// Where git cannot list them, no checkout is known: copied as before.
	untracked, _ := gitListed(ctx, tree)
	for rel := range untracked {
		if dir, found := outerCheckout(tree, rel); found {
			paths[dir] = true
		}
	}
	return paths
}

// gitListed is what git ls-files lists of the untracked files in tree that
// the committed .gitignore files leave as args say, by each path
// slash-separated from tree, and whether git could list them: it fails, or
// names tree itself, when it ignores it whole.
func gitListed(ctx context.Context, tree string, args ...string) (ignoredPaths, bool) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"ls-files", "-z", "--others", "--exclude-per-directory=.gitignore"}, args...)...) //nolint:gosec // only the fixed git program, asked about the tree a worker copies
	cmd.Dir, cmd.Env = tree, project.InheritedEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	paths := ignoredPaths{}
	for name := range bytes.SplitSeq(out, []byte{0}) {
		switch p := string(name); p {
		case "":
		case "./":
			return nil, false
		default:
			paths[p] = true
		}
	}
	return paths, true
}

// outerCheckout is the outermost folder of tree holding rel, an untracked
// path git lists slash-separated from tree, that is another repository's
// checkout, ending in /, and whether there is one: git lists a checkout it
// recognises as its folder, and the files of one whose .git it cannot
// read.
func outerCheckout(tree, rel string) (string, bool) {
	segments := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	for i := range segments {
		dir := path.Join(segments[:i+1]...)
		if checkout(filepath.Join(tree, filepath.FromSlash(dir))) {
			return dir + "/", true
		}
	}
	return "", false
}

// has says whether git ignores rel, a path slash-separated from the tree,
// a folder when dir.
func (p ignoredPaths) has(rel string, dir bool) bool {
	if dir {
		return p[rel+"/"]
	}
	return p[rel]
}

// brings says whether one of mutation.copy's globs matches path, absolute.
func (r *copyRules) brings(path string) bool {
	rel, ok := r.fromRoot(path)
	if !ok {
		return false
	}
	for _, glob := range r.globs {
		if match, err := doublestar.Match(glob, rel); err == nil && match {
			return true
		}
	}
	return false
}

// mayBring says whether one of mutation.copy's globs may match a path in
// the folder path, absolute, or the folder itself: the folders before the
// glob's first pattern lead to it or lie within it.
func (r *copyRules) mayBring(path string) bool {
	rel, ok := r.fromRoot(path)
	if !ok {
		return false
	}
	for _, glob := range r.globs {
		lead := staticLead(glob)
		if lead == "" || rel == lead || strings.HasPrefix(rel, lead+"/") || strings.HasPrefix(lead, rel+"/") {
			return true
		}
	}
	return false
}

// fromRoot is path, absolute, slash-separated from the project root, and
// whether it is within it and mutation.copy names anything.
func (r *copyRules) fromRoot(path string) (string, bool) {
	if r == nil || len(r.globs) == 0 {
		return "", false
	}
	rel, err := filepath.Rel(r.root, path)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// staticLead is glob's folders before its first segment holding a pattern,
// all of it when it holds none.
func staticLead(glob string) string {
	segments := strings.Split(strings.TrimSuffix(glob, "/"), "/")
	for i, s := range segments {
		if strings.ContainsAny(s, `*?[{\`) {
			return strings.Join(segments[:i], "/")
		}
	}
	return strings.Join(segments, "/")
}

// leftOut is each path of tree that git ignores, or checkout of another
// repository git does not track, that a copy of it left out,
// slash-separated from the project root, as mutation.copy names it, else
// from tree, a folder's ending in /, sorted: none in a linked folder, in
// .git, .dart_tool or the caches every copy leaves out, none mutation.copy
// brought back, nor generated Dart, none outside the project root, and
// none within a folder it names.
func (r *copyRules) leftOut(ctx context.Context, tree string) []string {
	base := tree
	if r != nil && r.root != "" {
		base = r.root
	}
	var out []string
	for rel := range r.ignored(ctx, tree) {
		named, err := filepath.Rel(base, filepath.Join(tree, filepath.FromSlash(rel)))
		if err != nil || !filepath.IsLocal(named) || r.kept(tree, rel) {
			continue
		}
		if strings.HasSuffix(rel, "/") {
			named += "/"
		}
		out = append(out, filepath.ToSlash(named))
	}
	slices.Sort(out)
	// A folder before what it holds: git may name both.
	var folded []string
	for _, rel := range out {
		if len(folded) > 0 && strings.HasSuffix(folded[len(folded)-1], "/") && strings.HasPrefix(rel, folded[len(folded)-1]) {
			continue
		}
		folded = append(folded, rel)
	}
	return folded
}

// kept says whether rel, a path git ignores in tree, slash-separated from
// it, is not one a copy left out as ignored: in a folder every copy links
// or leaves out by its name, brought back by mutation.copy, or generated
// Dart.
func (r *copyRules) kept(tree, rel string) bool {
	segments := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	for _, s := range segments {
		if linkDirs[s] || skipDirs[s] {
			return true
		}
	}
	for i := range segments {
		if r.brings(filepath.Join(tree, filepath.FromSlash(path.Join(segments[:i+1]...)))) {
			return true
		}
	}
	return !strings.HasSuffix(rel, "/") && project.GeneratedDart(filepath.Join(tree, filepath.FromSlash(rel)))
}

// checkout says whether the folder dir is a repository of its own: it
// holds .git, a folder or a file as a linked worktree has.
func checkout(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}
