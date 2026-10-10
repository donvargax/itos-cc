package graph

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/crap"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// Builder rebuilds the graph of some repositories, reparsing only files that
// changed since the last build.
type Builder struct {
	roots     []string // absolute
	projects  []string // the project root of each root, whose .metrics it reads (project.RootOf)
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
	snapshot *mutate.Snapshot
}

// NewBuilder prepares a builder for repository roots.
func NewBuilder(roots []string) (*Builder, error) {
	b := &Builder{cache: map[string]cached{}, snapshots: map[string]cachedSnapshot{}}
	seen := map[string]int{}
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(abs)
		if seen[name]++; seen[name] > 1 {
			name = fmt.Sprintf("%s-%d", name, seen[name])
		}
		b.roots = append(b.roots, abs)
		b.projects = append(b.projects, project.RootOf(abs))
		b.names = append(b.names, name)
	}
	return b, nil
}

// Build returns the current graph and whether it changed since the last
// call. Sources, tests, and the snapshots under the .metrics of each
// repository's project root, where every command keeps them, are all
// watched: a mutation snapshot holds while the tests that import its file
// are as it recorded them.
func (b *Builder) Build() (*Graph, bool, error) {
	var stamps []string
	type repo struct {
		name, root, top string
		files           project.Files
		support         map[string]string // the hashes of its listed tests' support files
	}
	var repos []repo
	for i, root := range b.roots {
		files, err := project.Discover([]string{root})
		if err != nil {
			return nil, false, err
		}
		support := supportHashes(b.projects[i])
		repos = append(repos, repo{b.names[i], root, b.projects[i], files, support})
		for _, f := range append(files.Sources, files.Tests...) {
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
		overlay := loadOverlay(r.top)
		var infos, tests []*fileInfo
		for _, path := range r.files.Sources {
			live[path] = true
			info, err := b.parse(r.root, path)
			if err != nil {
				return nil, false, err
			}
			infos = append(infos, info)
		}
		for _, path := range r.files.Tests {
			live[path] = true
			info, err := b.parseTest(r.root, path)
			if err != nil {
				return nil, false, err
			}
			tests = append(tests, info)
		}
		importing := importingTests(r.root, infos, tests)
		hashes := testHashes(r.top)
		for i, info := range infos {
			now, err := hashes(importing[info.abs])
			if err != nil {
				return nil, false, err
			}
			snap := b.snapshot(r.top, info.abs, live)
			infos[i] = overlay.apply(info, now, snap, snap.ListedChanged(r.top, r.support), r.top, r.support)
		}
		rg := newRepoGraph(r.name, r.root, infos)
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
func testHashes(root string) func(tests []string) (map[string]string, error) {
	seen := map[string]map[string]string{}
	return func(tests []string) (map[string]string, error) {
		out := map[string]string{}
		for _, t := range tests {
			h, ok := seen[t]
			if !ok {
				var err error
				if h, err = mutate.TestHashesUnder(root, []string{t}); err != nil {
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
	dir := metrics.DirOf(root)
	for _, name := range []string{"crap.json", "dry.json"} {
		p := filepath.Join(dir, name)
		out = append(out, p+"\x00"+stamp(p))
	}
	filepath.WalkDir(filepath.Join(dir, "mutate"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p+"\x00"+stamp(p))
		}
		return nil
	})
	return out
}

// parse returns the file's facts, from the cache when it has not changed.
// Live complexity always reflects the file on disk.
func (b *Builder) parse(root, path string) (*fileInfo, error) {
	st := stamp(path)
	if c, ok := b.cache[path]; ok && c.stamp == st {
		return c.info.clone(), nil
	}
	f, err := lang.ParseFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info := dependencies(root, f)
	info.serves, info.calls = f.Endpoints()
	sites := map[int][]mutate.Site{}
	for _, s := range mutate.Sites(f) {
		sites[s.Unit] = append(sites[s.Unit], s)
	}
	for i, u := range f.Units {
		info.units = append(info.units, Unit{
			Name: u.Name, Namespace: u.Namespace, File: info.rel, Line: u.StartLine, EndLine: u.EndLine,
			Complexity: f.Complexity(u), hash: mutate.UnitHash(f, u), sites: sites[i],
		})
	}
	b.cache[path] = cached{st, info}
	return info.clone(), nil
}

// parseTest returns what the graph needs of a test file to find the sources
// it tests, from the cache when it has not changed.
func (b *Builder) parseTest(root, path string) (*fileInfo, error) {
	st := stamp(path)
	if c, ok := b.cache[path]; ok && c.stamp == st {
		return c.info, nil
	}
	f, err := lang.ParseFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info := dependencies(root, f)
	b.cache[path] = cached{st, info}
	return info, nil
}

// snapshot is the mutation snapshot of the source file at path, in the
// project whose root is top, looked up where mutation check looks it up
// (mutate.LoadSnapshotOf), or nil when there is none. One that cannot be
// read is none too, so the graph still builds. It is read again only when
// its file changed since the last build. Its path is marked live.
func (b *Builder) snapshot(top, path string, live map[string]bool) *mutate.Snapshot {
	rel := project.FromRootOf(top, path)
	at := filepath.Join(metrics.DirOf(top), mutate.SnapshotName(rel))
	live[at] = true
	st := stamp(at)
	if c, ok := b.snapshots[at]; ok && c.stamp == st {
		return c.snapshot
	}
	var snap *mutate.Snapshot
	if st != "" {
		snap, _ = mutate.LoadSnapshotOf(top, rel)
	}
	b.snapshots[at] = cachedSnapshot{st, snap}
	return snap
}

// dependencies is what the graph needs of a parsed file to resolve what it
// imports, and what imports it.
func dependencies(root string, f *lang.File) *fileInfo {
	info := &fileInfo{rel: rel(root, f.Path), abs: f.Path, language: f.Spec.Name, namespace: f.Namespace, imports: f.Imports()}
	if f.Spec.Name == "kotlin" {
		info.topLevel = lang.TopLevelNames(f)
		info.refs = lang.ReferencedNames(f)
	}
	return info
}

func (f *fileInfo) clone() *fileInfo {
	c := *f
	c.units = append([]Unit(nil), f.units...)
	return &c
}

// overlay is what the snapshots in .metrics know about each function,
// coverage and duplicates; a function's mutation results come from its
// file's own snapshot (Builder.snapshot). Coverage is matched by file,
// namespace, name, and which same-named function of the file it is: a Go
// package has an init per file, and overloads share a name.
type overlay struct {
	coverage   keyed[float64]
	duplicates map[string]int // file#line → candidate pairs it is in
}

// mutationEntries is the entry of snap, info's file's snapshot, of each of
// info's units, nil for none: paired as mutation check pairs them, by name
// and hash (mutate.EntriesOf).
func mutationEntries(info *fileInfo, snap *mutate.Snapshot) []*mutate.UnitResult {
	out := make([]*mutate.UnitResult, len(info.units))
	if snap == nil {
		return out
	}
	ids, hashes := make([]string, len(info.units)), make([]string, len(info.units))
	for i, u := range info.units {
		ids[i], hashes[i] = u.Namespace+"#"+u.Name, u.hash
	}
	for i, j := range mutate.EntriesOf(ids, hashes, snap.Units) {
		if j >= 0 {
			out[i] = &snap.Units[j]
		}
	}
	return out
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

func loadOverlay(root string) overlay {
	o := overlay{coverage: newKeyed[float64](), duplicates: map[string]int{}}
	dir := metrics.DirOf(root)
	var crapSnap struct {
		Entries []crap.Entry `json:"entries"`
	}
	if readJSON(filepath.Join(dir, "crap.json"), &crapSnap) {
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
	if readJSON(filepath.Join(dir, "dry.json"), &drySnap) {
		for _, c := range drySnap.Candidates {
			o.duplicates[fmt.Sprintf("%s#%d", c.Left.File, c.Left.StartLine)]++
			o.duplicates[fmt.Sprintf("%s#%d", c.Right.File, c.Right.StartLine)]++
		}
	}
	return o
}

// apply joins the snapshots onto a file's units, snap being its mutation
// snapshot, nil for none. CRAP is recomputed from the live complexity and
// the last measured coverage, so it moves as you edit. A function's
// mutation entry is paired as mutation check pairs it, by name and hash
// (mutate.EntriesOf), so of functions sharing a name, reordering them
// changes nothing and editing one makes only that one stale. Its results
// are stale exactly when mutation check calls them stale, by check's own
// rule (mutate.FreshnessOf), tests being the hashes of the test files that
// import its file now, and listed how the files its listed outcomes rest
// on changed; a function with no entry, which check calls missing, carries
// none.
func (o overlay) apply(info *fileInfo, tests map[string]string, snap *mutate.Snapshot, listed *mutate.ListedChange, root string, support map[string]string) *fileInfo {
	entries := mutationEntries(info, snap)
	var moduleTests map[string]string
	if info.language == "go" {
		moduleTests, _ = mutate.GoModuleTestHashes(info.abs, root)
	}
	occurrence := map[string]int{}
	for i := range info.units {
		u := &info.units[i]
		k := u.Namespace + "#" + u.Name
		n := occurrence[k]
		occurrence[k]++
		if cov, ok := o.coverage.get(u.File, u.Namespace, u.Name, n); ok && cov >= 0 {
			c := cov
			score := float64(int(crap.Score(u.Complexity, c/100)*10+0.5)) / 10
			u.Coverage, u.CRAP = &c, &score
		}
		if m := entries[i]; m != nil {
			u.Mutated = true
			u.Stale = mutate.FreshnessOf(m, u.hash, snap.TestsChanged(tests), listed,
				mutate.BroadChangesForUnit(m.Mutants, moduleTests, support), u.sites).State == mutate.Stale
			u.Killed, u.Survived, u.Uncovered = m.Killed, m.Survived, m.Uncovered
		}
		u.Duplicates = o.duplicates[fmt.Sprintf("%s#%d", u.File, u.Line)]
	}
	return info
}

// supportHashes is the hashes of the support files of the tests the
// itos-cc.yaml at root lists, hashed as a snapshot records them; nil when
// it lists none or cannot be read.
func supportHashes(root string) map[string]string {
	cfg, err := config.LoadFrom(root)
	if err != nil || cfg.Tests == nil {
		return nil
	}
	support, err := mutate.SupportHashes(root, cfg.Tests.Support)
	if err != nil {
		return nil
	}
	return support
}

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, v) == nil
}
