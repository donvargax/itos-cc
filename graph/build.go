package graph

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/crap"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// Builder rebuilds the graph of some repositories, reparsing only files that
// changed since the last build.
type Builder struct {
	roots     []string // absolute
	names     []string
	cache     map[string]cached
	signature string
	version   int
	graph     *Graph
}

type cached struct {
	stamp string
	info  *fileInfo
}

// NewBuilder prepares a builder for repository roots.
func NewBuilder(roots []string) (*Builder, error) {
	b := &Builder{cache: map[string]cached{}}
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
		b.names = append(b.names, name)
	}
	return b, nil
}

// Build returns the current graph and whether it changed since the last
// call. Sources, tests, and the snapshots under each repository's .metrics
// are all watched: a mutation snapshot holds while the tests that import its
// file are as it recorded them.
func (b *Builder) Build() (*Graph, bool, error) {
	var stamps []string
	type repo struct {
		name, root string
		files      project.Files
	}
	var repos []repo
	for i, root := range b.roots {
		files, err := project.Discover([]string{root})
		if err != nil {
			return nil, false, err
		}
		repos = append(repos, repo{b.names[i], root, files})
		for _, f := range append(files.Sources, files.Tests...) {
			stamps = append(stamps, f+"\x00"+stamp(f))
		}
		stamps = append(stamps, metricStamps(root)...)
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
		overlay := loadOverlay(r.root)
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
		hashes := testHashes(r.root)
		for i, info := range infos {
			now, err := hashes(importing[info.abs])
			if err != nil {
				return nil, false, err
			}
			infos[i] = overlay.apply(info, now)
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
// repository at root, as mutation check computes it, hashing each file once
// however many sources its tests import.
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
	dir := filepath.Join(root, ".metrics")
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
	for _, u := range f.Units {
		info.units = append(info.units, Unit{
			Name: u.Name, Namespace: u.Namespace, File: info.rel, Line: u.StartLine, EndLine: u.EndLine,
			Complexity: f.Complexity(u), hash: mutate.UnitHash(f, u),
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

// overlay is what the snapshots in .metrics know about each function.
// Functions are matched by file, namespace, name, and which same-named
// function of the file they are: a Go package has an init per file, and
// overloads share a name.
type overlay struct {
	coverage   keyed[float64]
	mutation   keyed[recorded]
	duplicates map[string]int // file#line → candidate pairs it is in
}

// recorded is one function's entry in a mutation snapshot, with the
// snapshot that holds it.
type recorded struct {
	unit     *mutate.UnitResult
	snapshot *mutate.Snapshot
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
	o := overlay{coverage: newKeyed[float64](), mutation: newKeyed[recorded](), duplicates: map[string]int{}}
	dir := filepath.Join(root, ".metrics")
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
	filepath.WalkDir(filepath.Join(dir, "mutate"), func(p string, d os.DirEntry, err error) error {
		var snap mutate.Snapshot
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") && readJSON(p, &snap) {
			for i := range snap.Units {
				u := &snap.Units[i]
				o.mutation.add(snap.File, u.Namespace, u.Name, recorded{u, &snap})
			}
		}
		return nil
	})
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

// apply joins the snapshots onto a file's units. CRAP is recomputed from the
// live complexity and the last measured coverage, so it moves as you edit.
// Mutation results are stale as mutation check calls them: the function's
// source changed since, or tests, the hashes of the test files that import
// its file now, differ from those its snapshot recorded.
func (o overlay) apply(info *fileInfo, tests map[string]string) *fileInfo {
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
		if r, ok := o.mutation.get(u.File, u.Namespace, u.Name, n); ok {
			m := r.unit
			u.Mutated = true
			u.Stale = m.Hash != u.hash || r.snapshot.TestsChanged(tests) != nil
			u.Killed, u.Survived, u.Uncovered = m.Killed, m.Survived, m.Uncovered
		}
		u.Duplicates = o.duplicates[fmt.Sprintf("%s#%d", u.File, u.Line)]
	}
	return info
}

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, v) == nil
}
