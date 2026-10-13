package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	covporttest "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	"github.com/donvargax/itos-cc/internal/lang"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// The feature files of the commands moved onto kong run in process
// (decision 44), the ones features.InProcess lists: the steps of package
// features, through a driver that keeps every folder and file in memory
// with a fake git, and runs itos-cc through run with buffers and porttest's
// fakes built from that memory. The binary driver (features.TestFeatures)
// runs the same scenarios against the built binary, and alone those of the
// converted files whose command has not moved, a selection naming only
// theirs running nothing here; a scenario only one of them can run, for
// good, carries its tag, never a copy.
//
//	go test ./cmd/itos-cc -count=1 -run '^TestScenarios$'                       every live scenario
//	go test ./cmd/itos-cc -count=1 -run '^TestScenarios$' -scenarios=<regexp>   those with a tag it matches

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")

func TestScenarios(t *testing.T) {
	dir := filepath.Join("..", "..", "features")
	sel, err := features.TagFilter(*scenarios, dir, features.InProcess, features.BinaryOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Unconverted) > 0 {
		t.Logf("not run here, their feature files having no steps yet (q-52): %s", strings.Join(sel.Unconverted, " "))
	}
	if len(sel.Unmoved) > 0 {
		t.Logf("binary driver only until its command moves: %s", strings.Join(sel.Unmoved, " "))
	}
	if sel.Filter == "" {
		return
	}
	var paths []string
	for _, name := range features.InProcess {
		paths = append(paths, filepath.Join(dir, name))
	}
	suite := godog.TestSuite{
		Name:                "itos-cc in process",
		ScenarioInitializer: func(sc *godog.ScenarioContext) { features.Initialize(sc, newMemory(t)) },
		Options: &godog.Options{
			Format:      "pretty",
			Paths:       paths,
			Tags:        sel.Filter,
			Strict:      true,
			Concurrency: 1,
			TestingT:    t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("a scenario failed")
	}
}

// memory is the in-process driver: one scenario's folders and files, held
// in memory by absolute path, and its fake repositories, by the folder git
// init ran in. Nothing it does reaches the disk.
type memory struct {
	t       *testing.T
	files   map[string]string
	dirs    map[string]bool
	repos   map[string]*repo
	folders int
	// coverage is what each folder's own tools measure, as the scenario
	// declared it, by folder, which the fake measurer is handed (measurer).
	coverage map[string]features.Coverage
}

// repo is a fake git repository: what add staged and the last commit holds,
// by path relative to its folder, slash-separated.
type repo struct {
	index, committed map[string]string
	commits          int
}

var _ features.Driver = (*memory)(nil)

func newMemory(t *testing.T) *memory {
	return &memory{
		t: t, files: map[string]string{}, dirs: map[string]bool{}, repos: map[string]*repo{},
		coverage: map[string]features.Coverage{},
	}
}

// Folder is a fresh absolute folder, on the volume of the working directory
// where the system has volumes: its path alone, no disk.
func (m *memory) Folder() (string, error) {
	m.folders++
	path, err := filepath.Abs(filepath.FromSlash("/itos-cc-in-process/" + strconv.Itoa(m.folders)))
	if err != nil {
		return "", err
	}
	m.dirs[path] = true
	return path, nil
}

func (m *memory) Write(path string, data []byte) error {
	m.files[filepath.Clean(path)] = string(data)
	return nil
}

func (m *memory) Read(path string) ([]byte, error) {
	text, ok := m.files[filepath.Clean(path)]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return []byte(text), nil
}

func (m *memory) Mkdir(path string) error {
	m.dirs[filepath.Clean(path)] = true
	return nil
}

// Exists is whether path is a file, a folder made or a folder above either.
func (m *memory) Exists(path string) bool {
	path = filepath.Clean(path)
	if _, ok := m.files[path]; ok || m.dirs[path] {
		return true
	}
	for _, all := range []map[string]bool{m.dirs, pathSet(m.files)} {
		for p := range all {
			if under(p, path) {
				return true
			}
		}
	}
	return false
}

func (m *memory) Remove(path string) error {
	path = filepath.Clean(path)
	for _, all := range []map[string]bool{m.dirs, pathSet(m.files)} {
		for p := range all {
			if p == path || under(p, path) {
				delete(m.dirs, p)
				delete(m.files, p)
			}
		}
	}
	for p := range m.repos {
		if p == path || under(p, path) {
			delete(m.repos, p)
		}
	}
	return nil
}

// Coverage records what the project's own tools measure in dir.
func (m *memory) Coverage(dir string, c features.Coverage) error {
	m.coverage[filepath.Clean(dir)] = c
	return nil
}

// Link makes no link: the memory holds no symbolic links, so a scenario
// that needs one is the binary driver's.
func (m *memory) Link(target, path string) error {
	return fmt.Errorf("the in-process driver makes no symbolic link (%s to %s): tag the scenario %s", path, target, features.BinaryOnly)
}

// Build builds nothing: how a binary is built is the go command's, so a
// scenario that needs a build is the binary driver's.
func (m *memory) Build(dir string, b features.Build) (features.Built, error) {
	return features.Built{}, fmt.Errorf("the in-process driver builds no binary (%s in %s): tag the scenario %s", b.How, dir, features.BinaryOnly)
}

// Occupy holds no port: the memory has none, so a scenario that needs one
// in use is the binary driver's.
func (m *memory) Occupy() (int, func(), error) {
	return 0, nil, fmt.Errorf("the in-process driver holds no port: tag the scenario %s", features.BinaryOnly)
}

// Git is the fake git in dir: init makes it a repository, add stages its
// files, every one with -A or the paths named, and commit records what is
// staged. Anything else fails, naming the tag of a scenario that needs the
// real one.
func (m *memory) Git(dir string, args ...string) error {
	dir = filepath.Clean(dir)
	verb, rest := "", []string(nil)
	if len(args) > 0 {
		verb, rest = args[0], args[1:]
	}
	if verb == "init" {
		m.repos[dir] = &repo{index: map[string]string{}, committed: map[string]string{}}
		return nil
	}
	root, r := m.repoOf(dir)
	switch {
	case r == nil:
		return fmt.Errorf("git %s: %s is no repository of the in-process driver", verb, dir)
	case verb == "add":
		all := slices.Contains(rest, "-A") || slices.Contains(rest, ".")
		var named []string
		for _, a := range rest {
			if !strings.HasPrefix(a, "-") && a != "." {
				named = append(named, filepath.ToSlash(a))
			}
		}
		for rel := range r.index {
			if all || slices.Contains(named, rel) {
				delete(r.index, rel)
			}
		}
		for rel, text := range m.under(root) {
			if all || slices.Contains(named, rel) {
				r.index[rel] = text
			}
		}
		return nil
	case verb == "commit":
		r.committed = maps.Clone(r.index)
		r.commits++
		return nil
	}
	return fmt.Errorf("git %s: the in-process driver's git does init, add and commit; tag the scenario %s", strings.Join(args, " "), features.BinaryOnly)
}

// Run runs itos-cc through run, in process, against porttest's fakes over
// the memory at r.Dir: the tree of its files, what surrounds them, the
// snapshots under .metrics/ at its project root and the itos-cc.yaml there.
// The snapshots the run writes go back into the memory. A command still
// hand-parsed would reach the real disk, and a variable set in the
// environment reaches nothing here, so either is refused, naming the tag
// of a scenario only the binary can run.
func (m *memory) Run(r features.Run) (features.Result, error) {
	if name := commandOf(r.Args); commands[name] != nil {
		return features.Result{}, fmt.Errorf("itos-cc %s is hand-parsed and reaches the disk itself: tag the scenario %s", strings.Join(r.Args, " "), features.BinaryOnly)
	}
	if len(r.Env) > 0 {
		return features.Result{}, fmt.Errorf("the in-process driver sets no environment (%s): tag the scenario %s", strings.Join(r.Env, " "), features.BinaryOnly)
	}
	if len(r.Without) > 0 {
		return features.Result{}, fmt.Errorf("the in-process driver has no PATH to leave %s off: tag the scenario %s", strings.Join(r.Without, " "), features.BinaryOnly)
	}
	dir := filepath.Clean(r.Dir)
	root, git := m.repoOf(dir)
	if git == nil {
		root = dir
	}
	tree := selporttest.TreeOf(dir, m.from(dir), m.changed(dir, root, git))
	if git == nil {
		tree.NoGit = &selport.NoGit{Reason: "fatal: not a git repository (the in-process driver's)"}
	}
	store := &snapporttest.Store{RootDir: root, Snapshots: map[string]string{}}
	metricsDir := filepath.Join(root, ".metrics")
	for rel, text := range m.under(metricsDir) {
		store.Snapshots[rel] = text
	}
	file := heldConfig{root: root}
	if text, ok := m.files[filepath.Join(root, config.File)]; ok {
		file.text = []byte(text)
	}
	a := adapters{
		Dir:          dir,
		Tree:         tree,
		Surroundings: langporttest.Surroundings{Files: maps.Clone(m.files), Dir: dir},
		Store:        store,
		Coverage:     m.measurer(dir),
		Config:       file,
	}
	var stdout, stderr bytes.Buffer
	code := run(m.t.Context(), r.Args, strings.NewReader(r.Stdin), &stdout, &stderr, a)
	// What the run removed from the store is gone from the memory too, and
	// what it wrote is there.
	for rel := range m.under(metricsDir) {
		if _, ok := store.Snapshots[rel]; !ok {
			delete(m.files, filepath.Join(metricsDir, filepath.FromSlash(rel)))
		}
	}
	for name, text := range store.Snapshots {
		m.files[filepath.Join(metricsDir, filepath.FromSlash(name))] = text
	}
	return features.Result{Exit: code, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

// commandOf is the command args name, past the flags before it, as run
// reads them; --version is the version command.
func commandOf(args []string) string {
	for _, a := range args {
		switch {
		case a == "--version":
			return "version"
		case a == "--":
			return ""
		case !strings.HasPrefix(a, "-"):
			return a
		}
	}
	return ""
}

// repoOf is the fake repository dir is in, and its folder: the nearest at
// or above dir, as git finds its top level; nil when there is none.
func (m *memory) repoOf(dir string) (string, *repo) {
	for p := dir; ; p = filepath.Dir(p) {
		if r := m.repos[p]; r != nil {
			return p, r
		}
		if filepath.Dir(p) == p {
			return "", nil
		}
	}
}

// changed is what git reports changed in the repository at root, as
// project.Changed asks it, from dir: each file of a supported language
// under dir that differs from the last commit or is not in it (the index
// standing in before the first commit), relative to dir, slash-separated.
func (m *memory) changed(dir, root string, r *repo) []string {
	if r == nil {
		return nil
	}
	base := r.committed
	if r.commits == 0 {
		base = map[string]string{}
	}
	var out []string
	for rel, text := range m.under(root) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if !under(abs, dir) || lang.Detect(rel) == nil {
			continue
		}
		if old, ok := base[rel]; ok && old == text {
			continue
		}
		fromDir, err := filepath.Rel(dir, abs)
		if err != nil {
			continue
		}
		out = append(out, filepath.ToSlash(fromDir))
	}
	slices.Sort(out)
	return out
}

// under is the files below dir, by path relative to it, slash-separated.
func (m *memory) under(dir string) map[string]string {
	files := map[string]string{}
	for path, text := range m.files {
		if under(path, dir) {
			rel, err := filepath.Rel(dir, path)
			if err == nil {
				files[filepath.ToSlash(rel)] = text
			}
		}
	}
	return files
}

// under is whether path is below dir.
func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// pathSet is the paths of files, as a set.
func pathSet(files map[string]string) map[string]bool {
	set := map[string]bool{}
	for p := range files {
		set[p] = true
	}
	return set
}

// markers are the files that make a folder a build root of each language,
// as the coverage runner finds them.
var markers = map[string][]string{
	"go":         {"go.mod"},
	"typescript": {"package.json"},
	"python":     {"pyproject.toml", "setup.py", "setup.cfg"},
	"kotlin":     {"build.gradle.kts", "build.gradle", "pom.xml"},
}

// measurer is the coverage port's fake over the memory, from dir: every
// file, the reports a run names among them, and the report of each build
// root, as the runner groups the sources by language and the nearest
// folder holding that language's marker, or their own folder. A root whose
// Coverage the scenario declared is measured as it says: by the LCOV report
// its tools write, every source of its language proven, or nothing, a
// report its tools do not write; a root declared for none is a tool missing.
// An earlier run left no report of any root, so reading what one left finds
// none.
func (m *memory) measurer(dir string) covporttest.Measurer {
	disk := covporttest.Files{Dir: dir, Regular: slices.Collect(maps.Keys(m.files))}
	type group struct{ language, root string }
	groups := map[group][]string{}
	for _, path := range slices.Sorted(maps.Keys(m.files)) {
		spec := lang.Detect(path)
		if spec == nil || spec.IsTest(path) || markers[spec.Name] == nil || under(path, filepath.Join(m.scratch(path), ".metrics")) {
			continue
		}
		g := group{spec.Name, m.buildRoot(path, markers[spec.Name])}
		groups[g] = append(groups[g], path)
	}
	roots, left := map[string]*coverage.Report{}, map[string]*coverage.Report{}
	for g, sources := range groups {
		report := &coverage.Report{}
		declared, ok := m.coverage[g.root]
		switch {
		case !ok || declared.Language != g.language:
			report.SetMissing(coverage.Unmeasured{Dir: g.root, Language: g.language, Cause: covport.ToolMissing{},
				Reason: "no tool the in-process driver declares measures it"})
		case declared.Report == "":
			report.SetMissing(coverage.Unmeasured{Dir: g.root, Language: g.language, Cause: covport.MeasuredNothing{},
				Reason: "its coverage run measured none of its files"})
		default:
			entries, err := coverage.ParseLCOV(strings.NewReader(declared.Report))
			if err != nil {
				m.t.Errorf("the report declared in %s is no LCOV: %v", g.root, err)
			}
			report = coverage.Build(m.t.Context(), disk, sources, g.root, entries)
			report.MarkMeasured(g.language)
			report.Prove(g.language, g.root, sources)
		}
		roots[g.root] = coverage.Merge(roots[g.root], report)
		existing := &coverage.Report{}
		existing.SetMissing(coverage.Unmeasured{Dir: g.root, Language: g.language, Cause: covport.NoReport{}, Reason: "no report on disk measures its files"})
		left[g.root] = coverage.Merge(left[g.root], existing)
	}
	return covporttest.Measurer{Dir: dir, Files: maps.Clone(m.files), Roots: roots, Left: left}
}

// buildRoot is the nearest folder above path holding one of names, or
// path's own folder when none does, up to the scratch folder path is in.
func (m *memory) buildRoot(path string, names []string) string {
	top := m.scratch(path)
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		for _, name := range names {
			if _, ok := m.files[filepath.Join(dir, name)]; ok {
				return dir
			}
		}
		if dir == top || filepath.Dir(dir) == dir {
			return filepath.Dir(path)
		}
	}
}

// scratch is the scratch folder path is in, a folder Folder made.
func (m *memory) scratch(path string) string {
	return scratchOf(filepath.Dir(path))
}

// scratchOf is the scratch folder dir is, or is in: a folder Folder made.
func scratchOf(dir string) string {
	for d := dir; filepath.Dir(d) != d; d = filepath.Dir(d) {
		if filepath.Base(filepath.Dir(d)) == "itos-cc-in-process" {
			return d
		}
	}
	return dir
}

// from is every file of the scratch folder dir is in, as the disk holds
// it around a run there, by path relative to dir, slash-separated, ../
// leading out of it.
func (m *memory) from(dir string) map[string]string {
	files := map[string]string{}
	for path, text := range m.files {
		if under(path, scratchOf(dir)) {
			if rel, err := filepath.Rel(dir, path); err == nil {
				files[filepath.ToSlash(rel)] = text
			}
		}
	}
	return files
}
