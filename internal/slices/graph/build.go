package graph

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/imports"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Reading is what a Builder reads repositories through, which cmd binds:
// the tree it finds and reads their sources and tests in, what surrounds a
// file, which lang asks to parse it, the snapshots under .metrics of each
// project root (Store), mutation's judgment (Mutation), Top, the project
// root of a repository's root, whose snapshots it reads, and Name, the name
// of a repository's node, from its root; nil names it by its folder.
type Reading struct {
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Store        func(top string) snapport.Store
	Mutation     Mutation
	Top          func(root string) string
	Name         func(root string) string
}

// Builder rebuilds the graph of some repositories, reparsing only files that
// changed since the last build, and asking mutation through its port what
// mutation check knows of each function.
type Builder struct {
	reading   Reading
	mutation  Mutation
	roots     []string // absolute
	projects  []string // the project root of each root, whose .metrics it reads (Reading.Top)
	names     []string
	cache     map[string]cached
	snapshots map[string]cachedSnapshot // by the snapshot's path
	signature string
	version   int
	graph     *Graph
}

type cached struct {
	stamp string
	info  *fileInfo
}

type cachedSnapshot struct {
	stamp    string
	snapshot any
}

// NewBuilder prepares a builder for repository roots, read through
// reading. A missing binding is refused, never replaced by the disk.
func NewBuilder(roots []string, reading Reading) (*Builder, error) {
	if reading.Mutation == nil {
		return nil, errors.New("graph mutation observation bindings are missing")
	}
	if reading.Tree == nil || reading.Surroundings == nil || reading.Store == nil || reading.Top == nil {
		return nil, errors.New("graph reading bindings are missing")
	}
	b := &Builder{reading: reading, mutation: reading.Mutation, cache: map[string]cached{}, snapshots: map[string]cachedSnapshot{}}
	seen := map[string]int{}
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(abs)
		if reading.Name != nil {
			name = reading.Name(abs)
		}
		if seen[name]++; seen[name] > 1 {
			name = fmt.Sprintf("%s-%d", name, seen[name])
		}
		b.roots = append(b.roots, abs)
		b.projects = append(b.projects, reading.Top(abs))
		b.names = append(b.names, name)
	}
	return b, nil
}

// Build returns the current graph and whether it changed since the last
// call, built under ctx. Sources, tests, and the snapshots under the
// .metrics of each repository's project root, where every command keeps
// them, are all watched: a mutation snapshot holds while the tests that
// import its file are as it recorded them. Once ctx is done it returns its
// cause and keeps the last graph.
func (b *Builder) Build(ctx context.Context) (*Graph, bool, error) {
	var stamps []string
	type repo struct {
		name, root, top string
		sources, tests  []string
		support         map[string]string // the hashes of its listed tests' support files
	}
	var repos []repo
	for i, root := range b.roots {
		sources, tests, failure := b.reading.Tree.Discover(ctx, []string{root})
		if failure != nil {
			return nil, false, stopped(ctx, failure)
		}
		support := b.mutation.Support(ctx, b.projects[i])
		repos = append(repos, repo{b.names[i], root, b.projects[i], sources, tests, support})
		for _, f := range append(sources, tests...) {
			stamps = append(stamps, f+"\x00"+stamp(f))
		}
		for f, hash := range support {
			stamps = append(stamps, f+"\x00"+hash)
		}
		stamps = append(stamps, metricStamps(b.projects[i])...)
	}
	sort.Strings(stamps)
	signature := strings.Join(stamps, "\n")
	if signature == b.signature && b.graph != nil {
		return b.graph, false, nil
	}

	g := &Graph{Nodes: []*Node{}, Edges: []Edge{}}
	live := map[string]bool{}
	var built []*repoGraph
	for _, r := range repos {
		overlay := loadOverlay(ctx, b.reading.Store(r.top))
		var infos, tests []*fileInfo
		for _, path := range r.sources {
			live[path] = true
			info, err := b.parse(ctx, r.root, path)
			if err != nil {
				return nil, false, stopped(ctx, err)
			}
			infos = append(infos, info)
		}
		for _, path := range r.tests {
			live[path] = true
			info, err := b.parseTest(ctx, r.root, path)
			if err != nil {
				return nil, false, stopped(ctx, err)
			}
			tests = append(tests, info)
		}
		importing := imports.Reach(ctx, r.root, importFiles(infos), importFiles(tests))
		hashes := testHashes(ctx, b.mutation, r.top)
		for i, info := range infos {
			now, err := hashes(importing[info.Abs])
			if err != nil {
				return nil, false, err
			}
			snap, snapshotErr := b.snapshot(ctx, r.top, info.Abs, live)
			if snapshotErr != nil {
				return nil, false, snapshotErr
			}
			infos[i] = overlay.apply(info, b.mutation.Judge(ctx, Judged{Top: r.top, Path: info.Abs, Snapshot: snap, Facts: info.mutation, Tests: now, Support: r.support}))
		}
		rg := newRepoGraph(ctx, r.name, r.root, infos)
		// A stopped build resolves no further: what it left unresolved is
		// the stop, never a missing file.
		if err := stopped(ctx, nil); err != nil {
			return nil, false, err
		}
		rg.summarize()
		rg.output(g)
		built = append(built, rg)
	}
	g.Edges = append(g.Edges, httpEdges(built)...)
	for path := range b.cache {
		if !live[path] {
			delete(b.cache, path)
		}
	}
	for path := range b.snapshots {
		if !live[path] {
			delete(b.snapshots, path)
		}
	}
	b.version++
	g.Version = b.version
	b.signature, b.graph = signature, g
	return g, true, nil
}

// stopped is the cause of ctx once it is done, or else err, so a call cut
// short by a stopped run reports the stop, never what it left undone.
func stopped(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}

// Roots are the absolute repository roots, by name.
func (b *Builder) Roots() map[string]string {
	out := map[string]string{}
	for i, name := range b.names {
		out[name] = b.roots[i]
	}
	return out
}

func stamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

// testHashes returns what a snapshot records of some test files of the
// project whose root is root, as mutation check computes it, hashing each
// file once however many sources its tests import.
func testHashes(ctx context.Context, mutation Mutation, root string) func(tests []string) (map[string]string, error) {
	seen := map[string]map[string]string{}
	return func(tests []string) (map[string]string, error) {
		out := map[string]string{}
		for _, t := range tests {
			h, ok := seen[t]
			if !ok {
				var err error
				if h, err = mutation.Tests(ctx, root, []string{t}); err != nil {
					return nil, err
				}
				seen[t] = h
			}
			maps.Copy(out, h)
		}
		return out, nil
	}
}

// metricStamps watches the snapshots the overlay reads.
func metricStamps(root string) []string {
	var out []string
	dir := metricsdir.DirOf(root)
	for _, name := range []string{"crap.json", "dry.json"} {
		p := filepath.Join(dir, name)
		out = append(out, p+"\x00"+stamp(p))
	}
	// The callback never stops the walk, a missing folder included, so it
	// returns no error.
	_ = filepath.WalkDir(filepath.Join(dir, "mutate"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p+"\x00"+stamp(p))
		}
		return nil
	})
	return out
}

// parse returns the file's facts, from the cache when it has not changed.
// Live complexity always reflects the file on disk.
func (b *Builder) parse(ctx context.Context, root, path string) (*fileInfo, error) {
	st := stamp(path)
	if c, ok := b.cache[path]; ok && c.stamp == st {
		return c.info.clone(), nil
	}
	f, err := selection.Parse(ctx, b.reading.Tree, b.reading.Surroundings, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info := &fileInfo{File: imports.Of(root, f), mutation: b.mutation.Facts(f)}
	info.serves, info.calls = f.Endpoints()
	for _, u := range f.Units {
		info.units = append(info.units, Unit{
			Name: u.Name, Namespace: u.Namespace, File: info.Rel, Line: u.StartLine, EndLine: u.EndLine,
			Complexity: f.Complexity(u),
		})
	}
	b.cache[path] = cached{st, info}
	return info.clone(), nil
}

// parseTest returns what the graph needs of a test file to find the sources
// it tests, from the cache when it has not changed.
func (b *Builder) parseTest(ctx context.Context, root, path string) (*fileInfo, error) {
	st := stamp(path)
	if c, ok := b.cache[path]; ok && c.stamp == st {
		return c.info, nil
	}
	f, err := selection.Parse(ctx, b.reading.Tree, b.reading.Surroundings, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info := &fileInfo{File: imports.Of(root, f)}
	b.cache[path] = cached{st, info}
	return info, nil
}

// snapshot is the mutation snapshot of the source file at path, absolute,
// in the project whose root is top, as the Mutation port reads it where
// mutation check does, or nil when there is none. It is read again only
// when its file changed since the last build. Its path is marked live.
func (b *Builder) snapshot(ctx context.Context, top, path string, live map[string]bool) (any, error) {
	rel := snapshot.Key(top, path)
	at := b.mutation.SnapshotPath(top, rel)
	live[at] = true
	st := stamp(at)
	if c, ok := b.snapshots[at]; ok && c.stamp == st {
		return c.snapshot, nil
	}
	snap, err := b.mutation.Snapshot(ctx, top, rel)
	if err != nil {
		return nil, err
	}
	b.snapshots[at] = cachedSnapshot{st, snap}
	return snap, nil
}

func (f *fileInfo) clone() *fileInfo {
	c := *f
	c.units = append([]Unit(nil), f.units...)
	return &c
}

// overlay is what the snapshots in .metrics know about each function,
// coverage and duplicates; a function's mutation results are what the
// Mutation port judges of its file's own snapshot (Builder.snapshot). Coverage is matched by file,
// namespace, name, and which same-named function of the file it is: a Go
// package has an init per file, and overloads share a name.
type overlay struct {
	coverage   keyed[float64]
	duplicates map[string]int // file#line → candidate pairs it is in
}

type keyed[T any] struct {
	values map[string]T
	seen   map[string]int // occurrences so far, per file#namespace#name
	files  map[string]bool
}

func newKeyed[T any]() keyed[T] {
	return keyed[T]{values: map[string]T{}, seen: map[string]int{}, files: map[string]bool{}}
}

// add records v for the next function named namespace#name in file, in
// source order.
func (k keyed[T]) add(file, namespace, name string, v T) {
	base := file + "#" + namespace + "#" + name
	k.values[fmt.Sprintf("%s#%d", base, k.seen[base])] = v
	k.seen[base]++
	k.files[file] = true
}

// get finds a function's value. A snapshot written from another directory
// names the same file with a longer or shorter path, so a file the snapshot
// does not name exactly is matched by path suffix.
func (k keyed[T]) get(file, namespace, name string, occurrence int) (T, bool) {
	if !k.files[file] {
		for f := range k.files {
			if strings.HasSuffix(f, "/"+file) || strings.HasSuffix(file, "/"+f) {
				file = f
				break
			}
		}
	}
	v, ok := k.values[fmt.Sprintf("%s#%s#%s#%d", file, namespace, name, occurrence)]
	return v, ok
}

// crapEntry is what graph reads of a function crap.json scores: where it
// is and its coverage, nil when none applied. graph decodes the snapshot
// itself, so it imports nothing of the crap slice (ADR-0051), whose format
// this follows.
type crapEntry struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	File      string   `json:"file"`
	Coverage  *float64 `json:"coverage"` // percent, 0–100
}

// crapScore is the CRAP formula, with coverage as a fraction from 0 to 1,
// as the crap command scores a function: CC² × (1 − coverage)³ + CC. graph
// keeps this copy of it rather than import the crap slice (ADR-0051).
func crapScore(cc int, coverage float64) float64 {
	c, u := float64(cc), 1-coverage
	return c*c*(u*u*u) + c
}

func loadOverlay(ctx context.Context, store snapport.Store) overlay {
	o := overlay{coverage: newKeyed[float64](), duplicates: map[string]int{}}
	var crapSnap struct {
		Entries []crapEntry `json:"entries"`
	}
	if readSnapshot(ctx, store, "crap.json", &crapSnap) {
		for _, e := range crapSnap.Entries {
			cov := -1.0 // keeps occurrences aligned for functions without coverage
			if e.Coverage != nil {
				cov = *e.Coverage
			}
			o.coverage.add(filepath.ToSlash(e.File), e.Namespace, e.Name, cov)
		}
	}
	var drySnap struct {
		Candidates []struct {
			Left, Right struct {
				File      string `json:"file"`
				StartLine int    `json:"start_line"`
			}
		} `json:"candidates"`
	}
	if readSnapshot(ctx, store, "dry.json", &drySnap) {
		for _, c := range drySnap.Candidates {
			o.duplicates[fmt.Sprintf("%s#%d", c.Left.File, c.Left.StartLine)]++
			o.duplicates[fmt.Sprintf("%s#%d", c.Right.File, c.Right.StartLine)]++
		}
	}
	return o
}

// apply joins the snapshots onto a file's units, marks being what the
// Mutation port judged of each, in their order. CRAP is recomputed from the
// live complexity and the last measured coverage, so it moves as you edit.
// A function's mutation results, and whether they are stale, are mutation
// check's own verdict (Mutation.Judge); a function no snapshot entry
// records carries none.
func (o overlay) apply(info *fileInfo, marks []UnitMutation) *fileInfo {
	occurrence := map[string]int{}
	for i := range info.units {
		u := &info.units[i]
		k := u.Namespace + "#" + u.Name
		n := occurrence[k]
		occurrence[k]++
		if cov, ok := o.coverage.get(u.File, u.Namespace, u.Name, n); ok && cov >= 0 {
			c := cov
			score := float64(int(crapScore(u.Complexity, c/100)*10+0.5)) / 10
			u.Coverage, u.CRAP = &c, &score
		}
		if m := marks[i]; m.Mutated {
			u.Mutated, u.Stale = true, m.Stale
			u.Killed, u.Survived, u.Uncovered = m.Killed, m.Survived, m.Uncovered
		}
		u.Duplicates = o.duplicates[fmt.Sprintf("%s#%d", u.File, u.Line)]
	}
	return info
}

// readSnapshot decodes the snapshot name of store into v and says whether
// it holds one: one that is not there, or cannot be read or decoded, is
// none, so the graph still builds.
func readSnapshot(ctx context.Context, store snapport.Store, name string, v any) bool {
	found, failure := store.Read(ctx, name, v)
	return found && failure == nil
}
