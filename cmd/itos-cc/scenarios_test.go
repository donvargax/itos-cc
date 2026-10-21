package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// The converted feature files run in process (decision 44), the ones
// features.Converted lists, every command they run having moved onto kong
// (T-114): the steps of package features, through a driver that keeps every folder and file in memory
// with a fake git, and runs itos-cc through run with buffers and porttest's
// fakes built from that memory. The binary driver (features.TestFeatures)
// runs the same scenarios against the built binary, every converted file's
// now, and alone those features.Deferred leaves to it until their owner's
// work, which a selection here logs with their reasons; a scenario only one
// of them can run, for good, carries its tag, never a copy.
//
//	go test ./cmd/itos-cc -count=1 -run '^TestScenarios$'                       every live scenario
//	go test ./cmd/itos-cc -count=1 -run '^TestScenarios$' -scenarios=<regexp>   those with a tag it matches

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")

func TestScenarios(t *testing.T) {
	dir := filepath.Join("..", "..", "features")
	sel, err := features.TagFilter(*scenarios, dir, features.Converted, features.BinaryOnly)
	if err != nil {
		t.Fatal(err)
	}
	sel, deferred, err := features.Defer(sel, *scenarios, dir, features.Deferred)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range deferred {
		t.Logf("not run here, the binary driver's for now (features.Deferred): %s %s", tag, features.Deferred[tag])
	}
	if len(sel.Skipped) > 0 {
		t.Logf("not run here, left out by ITOS_CC_SKIP_FEATURES: %s", strings.Join(sel.Skipped, ", "))
	}
	if len(sel.Unconverted) > 0 {
		t.Logf("not run here, their feature files having no steps yet (q-52): %s", strings.Join(sel.Unconverted, " "))
	}
	if len(sel.Unstepped) > 0 {
		t.Logf("not run here, their steps not written yet (features.Unstepped): %s", strings.Join(sel.Unstepped, " "))
	}
	if len(sel.Unmoved) > 0 {
		t.Logf("binary driver only until its command moves: %s", strings.Join(sel.Unmoved, " "))
	}
	if sel.Filter == "" {
		return
	}
	suite := godog.TestSuite{
		Name: "itos-cc in process",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			features.Initialize(sc, newMemory(t))
			logSkips(t, sc)
		},
		Options: &godog.Options{
			Format:      "pretty",
			Paths:       sel.Paths,
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

// logSkips logs why a scenario skipped in process, as the step that skipped
// it said (godog.ErrSkip wrapped with its reason), since godog's pretty
// output shows the skipped steps alone: a skip is never silent here.
func logSkips(t *testing.T, sc *godog.ScenarioContext) {
	var scenario string
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		scenario = s.Name
		return ctx, nil
	})
	sc.StepContext().After(func(ctx context.Context, st *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
		if status == godog.StepSkipped && err != nil {
			t.Logf("skipped in process: %s, at %q: %v", scenario, st.Text, err)
		}
		return ctx, err
	})
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
	// tests are what each folder's own tests do, as the scenario declared
	// them, by folder and command, for the fake that runs a project's tests once the
	// mutation group moves (porttest.Workers, S7).
	tests map[string]map[string]features.Tests
	// listed is the latest explicit independent harness setup for a root.
	// Command-keyed Workers outcomes never choose or erase measurement facts.
	listed       map[string]*features.ListedTests
	versions     map[string]uint64
	toolSetup    map[mutantstest.PytestProbe]bool
	readVersions map[string]uint64
	imports      map[string]map[string][]string
	// links are the symbolic links Link made, by path, to their targets.
	links map[string]string
}

// repo is independent staged/committed Git state owned by the History fake.
type repo = mutantstest.HistoryRepository

var _ features.Driver = (*memory)(nil)

func newMemory(t *testing.T) *memory {
	return &memory{
		t: t, files: map[string]string{}, dirs: map[string]bool{}, repos: map[string]*repo{},
		coverage: map[string]features.Coverage{}, listed: map[string]*features.ListedTests{}, tests: map[string]map[string]features.Tests{}, versions: map[string]uint64{}, readVersions: map[string]uint64{}, toolSetup: map[mutantstest.PytestProbe]bool{}, imports: map[string]map[string][]string{}, links: map[string]string{},
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
	m.versions[filepath.Clean(path)]++
	return nil
}

func (m *memory) WriteExecutable(path string, data []byte) error {
	return m.Write(path, data)
}

// Read is the text of the file at path; a folder there cannot be read, as
// on a disk, and nothing there is not there.
func (m *memory) Read(path string) ([]byte, error) {
	text, ok := m.files[filepath.Clean(path)]
	if !ok {
		if m.Exists(path) {
			return nil, &fs.PathError{Op: "read", Path: path, Err: errIsDirectory}
		}
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return []byte(text), nil
}

// errIsDirectory is why the memory cannot read a folder as a file.
var errIsDirectory = errors.New("is a directory")

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
				m.versions[p]++
			}
		}
	}
	for p := range m.repos {
		if p == path || under(p, path) {
			delete(m.repos, p)
		}
	}
	for p := range m.links {
		if p == path || under(p, path) {
			delete(m.links, p)
		}
	}
	// A file removed resolves no import: its adjacency, and every edge to
	// it, go with it.
	for _, edges := range m.imports {
		for file, targets := range edges {
			if file == path || under(file, path) {
				delete(edges, file)
				continue
			}
			edges[file] = slices.DeleteFunc(targets, func(target string) bool { return target == path || under(target, path) })
		}
	}
	return nil
}

// Files inventories actual in-memory files, with the binary driver's relative paths.
func (m *memory) Files(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir = filepath.Clean(dir)
	if !m.Exists(dir) {
		return nil, &fs.PathError{Op: "files", Path: dir, Err: fs.ErrNotExist}
	}
	var files []string
	for path := range m.files {
		if under(path, dir) {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return nil, err
			}
			files = append(files, filepath.ToSlash(rel))
		}
	}
	slices.Sort(files)
	return files, nil
}

// Lock requires actual OS liveness and is never simulated in process.
func (m *memory) Lock(ctx context.Context, path string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("Lock %s requires real OS locking (@binary-only)", path)
}

// Coverage records what the project's own tools measure in dir.
func (m *memory) Coverage(dir string, c features.Coverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := measurementOf(c).Validate(); err != nil {
		return fmt.Errorf("coverage declaration in %s: %w", dir, err)
	}
	m.coverage[filepath.Clean(dir)] = c
	if c.Language == "typescript" {
		// The binary driver links the pinned node_modules into dir
		// (testdata/tools/node): its installed entries are vitest's and
		// c8's, c8's left out of a project said to lack it.
		bins := []string{"vitest", "c8"}
		if c.WithoutC8 {
			bins = bins[:1]
		}
		if err := m.Tools(m.t.Context(), dir, features.ToolSetup{NodeBins: bins}); err != nil {
			return err
		}
	}
	return nil
}

// Tests records what the project's own tests do in dir.
func (m *memory) Tests(dir string, t features.Tests) error {
	dir = filepath.Clean(dir)
	if err := m.admitCoverageTrace(dir, t); err != nil {
		return err
	}
	if t.Command == "" && t.Imports != nil && (t.Baseline != "" || len(t.Surviving) > 0 || len(t.TimingOut) > 0 || t.Elapsed != 0 || t.Trace != nil || t.Listed != nil) {
		return fmt.Errorf("imports-only setup cannot declare execution outcomes")
	}

	if t.Imports != nil {
		if m.imports[dir] == nil {
			m.imports[dir] = map[string][]string{}
		}
		normalized := map[string][]string{}
		for file, edges := range t.Imports {
			path := filepath.Join(dir, filepath.FromSlash(file))
			if filepath.IsAbs(file) {
				path = filepath.Clean(file)
			}
			if _, known := m.files[path]; !known {
				return fmt.Errorf("unknown import declaration file %s", file)
			}
			for _, edge := range edges {
				target := filepath.Join(dir, filepath.FromSlash(edge))
				if filepath.IsAbs(edge) {
					target = filepath.Clean(edge)
				}
				if _, known := m.files[target]; !known {
					return fmt.Errorf("unknown referenced import file %s", edge)
				}
				normalized[path] = append(normalized[path], target)
			}
			if _, exists := normalized[path]; !exists {
				normalized[path] = []string{}
			}
			slices.Sort(normalized[path])
			if old, known := m.imports[dir][path]; known && !slices.Equal(old, normalized[path]) {
				return fmt.Errorf("conflicting import adjacency for %s", file)
			}
		}
		for file, edges := range normalized {
			m.imports[dir][file] = slices.Clone(edges)
		}
	}
	dir = filepath.Clean(dir)
	if t.Command == "" && t.Imports != nil {
		if t.Baseline != "" || len(t.Surviving) > 0 || len(t.TimingOut) > 0 || t.Elapsed != 0 || t.Trace != nil || t.Listed != nil {
			return fmt.Errorf("imports-only setup cannot declare execution outcomes")
		}
		return nil
	}

	if m.tests[dir] == nil {
		m.tests[dir] = map[string]features.Tests{}
	}
	if t.Listed != nil {
		if m.listed == nil {
			m.listed = map[string]*features.ListedTests{}
		}
		m.listed[dir] = ownListed(t.Listed)
		t.Listed = ownListed(t.Listed)
	}
	m.tests[dir][t.Command] = t
	// A program named by a path relative to dir is the same program by
	// its absolute path, which itos-cc runs an installed tool by
	// (project.NodeBin): both name one command, which this declares.
	// On Windows a program named without its extension is the installed
	// .cmd shim of it, as an npm install writes one beside it.
	if words := strings.Fields(t.Command); len(words) > 0 && strings.ContainsAny(words[0], `/\`) && !filepath.IsAbs(words[0]) {
		words[0] = filepath.Join(dir, filepath.FromSlash(words[0]))
		m.tests[dir][strings.Join(words, " ")] = t
		if m.Platform() == "windows" && filepath.Ext(words[0]) == "" {
			words[0] += ".cmd"
			m.tests[dir][strings.Join(words, " ")] = t
		}
	}
	return nil
}

// Head observes the same independent immutable history bound to actual runs.
func (m *memory) Head(ctx context.Context, dir string) (string, error) {
	value, failure := m.history(dir).Head(ctx)
	return value, failure
}
func (m *memory) history(dir string) mutantstest.History {
	root, repository := m.repoOf(filepath.Clean(dir))
	return mutantstest.History{Root: root, Dir: filepath.Clean(dir), Repository: repository, Working: m.files}
}

// Counted skips the scenario where itos-cc makes no counted run, in
// process as it does in its own: the system decides, which the run here
// shares.
func (m *memory) Counted() error {
	if !features.CountedOn(runtime.GOOS) {
		return fmt.Errorf("%w: itos-cc makes counted runs on Linux and macOS alone (#29)", godog.ErrSkip)
	}
	return nil
}

// Link makes path a symbolic link to target, a file the memory holds: it
// reads as its target, and git stages and commits it as a link. The memory
// links no folder, so a scenario that needs one is the binary driver's.
func (m *memory) Link(target, path string) error {
	target, path = filepath.Clean(target), filepath.Clean(path)
	text, ok := m.files[target]
	if !ok {
		return fmt.Errorf("the in-process driver makes no symbolic link to a folder (%s to %s): tag the scenario %s", path, target, features.BinaryOnly)
	}
	m.files[path] = text
	m.links[path] = target
	m.versions[path]++
	return nil
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

// Graph will use the serving adapter when that command moves in S8.
func (m *memory) Graph(_ context.Context, _ features.Run) ([]byte, error) {
	return nil, fmt.Errorf("serve has not moved in process yet")
}

// Git applies the Given's actual setup operations to the independent History
// state. Commands still unsupported by the memory fixture fail explicitly.
func (m *memory) Git(dir string, args ...string) error {
	dir = filepath.Clean(dir)
	if len(args) > 0 && args[0] == "init" {
		repository := mutantstest.NewHistoryRepository()
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "-q", "--quiet":
			case "-b", "--initial-branch":
				if i+1 == len(args) {
					return fmt.Errorf("fixture Git initial branch missing")
				}
				i++
				repository.Branch = args[i]
				repository.Branches = map[string]bool{args[i]: true}
			default:
				return fmt.Errorf("unsupported fixture Git init argument %s", args[i])
			}
		}
		m.repos[dir] = repository
		return nil
	}
	root, repository := m.repoOf(dir)
	if repository == nil {
		return fmt.Errorf("git in %s: no fixture repository", dir)
	}
	before := maps.Clone(m.files)
	repository.WorkingLinks = map[string]bool{}
	for path := range m.links {
		if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
			repository.WorkingLinks[filepath.ToSlash(rel)] = true
		}
	}
	if err := repository.Git(root, dir, m.files, args...); err != nil {
		return err
	}
	for path, text := range m.files {
		if previous, ok := before[path]; !ok || previous != text {
			m.versions[path]++
		}
	}
	for path := range before {
		if _, present := m.files[path]; !present {
			m.versions[path]++
		}
	}
	return nil
}

// Run runs itos-cc through run, in process, against porttest's fakes over
// the memory at r.Dir: the tree of its files, what surrounds them, the
// snapshots under .metrics/ at its project root and the itos-cc.yaml there.
// The snapshots the run writes go back into the memory. The run's
// variables reach the fakes as the environment facts the real adapters
// read (inputs). serve, still on its legacy handler, would reach the real
// disk and network, so it is refused: a scenario that runs it is the binary
// driver's until serve's work (features.Deferred).
func (m *memory) Run(r features.Run) (features.Result, error) {
	if name := commandOf(r.Args); legacy[name] != nil {
		return features.Result{}, fmt.Errorf("itos-cc %s is on its legacy handler and reaches the disk and network itself: list the scenario in features.Deferred", strings.Join(r.Args, " "))
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
	if text, err := m.Read(filepath.Join(root, config.File)); err == nil {
		file.text = text
	} else if !errors.Is(err, fs.ErrNotExist) {
		file.err = err
	}
	input := m.inputs(r)
	observations := &selporttest.Observations{}
	tree.Observations = observations
	defer func() {
		for _, path := range append(append(input.Reads, input.Lists...), append(observations.Reads, observations.Lists...)...) {
			m.readVersions[filepath.Clean(path)]++
		}
	}()
	var stdout, stderr bytes.Buffer
	measurer := &memoryCoverage{memory: m, base: m.measurer(dir), inputs: input}
	measurer.base.Log = &stderr
	observed := observedStore{Store: store, memory: m, dir: metricsDir}
	source := m.sourceEdits()
	workers := m.workers(r)
	m.listedSelections(workers, file.text)
	lister := m.lister(root, file.text, &stderr)
	c := m.counted(dir, &tree, input, measurer.base, workers, lister)
	a := adapters{
		Settings:   config.Reader{Source: memoryConfig{memory: m, root: root}},
		Exceptions: func(root string) mutation.Exceptions { return memoryExceptions{memory: m, root: root} }, SourceStatus: source,
		SnapshotReader: snapshots.Mutations{Store: observed}, Catalog: snapshots.Mutations{Store: observed}, Presence: source, Snapshots: snapshots.Mutations{Store: observed}, SourceEdits: source, TestLister: lister,
		Toolchain: input, Fingerprints: input, Reach: input, History: m.history(dir),
		Workers:      workers,
		Dir:          dir,
		Tree:         &tree,
		Surroundings: input,
		Store:        observedStore{Store: store, memory: m, dir: metricsDir},
		Coverage:     measurer, MutationCoverage: measurer,
		Selection: config.Reader{Source: file},
		Fresh:     c.Fresh, FreshWorkers: c.FreshWorkers, Repository: c.Repository, PhysicalDir: c.PhysicalDir, Platform: c.Platform,
	}
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
	if measurer.observationErr != nil {
		return features.Result{}, measurer.observationErr
	}
	return features.Result{Exit: code, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

// ObserveReads marks actual bound port operations. Positive controls use those
// same virtual reads/discovery; fixture assertions through Driver.Read do not.
func (m *memory) ObserveReads(paths ...string) (func() ([]string, error), error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("read observation needs nonempty targets")
	}
	before := map[string]uint64{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, file := m.files[path]; file {
			input := m.inputs(features.Run{Dir: filepath.Dir(path)})
			if _, ok := input.Read(m.t.Context(), path); !ok {
				return nil, fmt.Errorf("positive read failed: %s", path)
			}
			for _, read := range input.Reads {
				m.readVersions[read]++
			}
		} else if m.Exists(path) {
			tree := selporttest.TreeOf(path, m.from(path), nil)
			observations := &selporttest.Observations{}
			tree.Observations = observations
			if _, _, failure := tree.Discover(m.t.Context(), []string{path}); failure != nil {
				return nil, failure
			}
			if !slices.Contains(observations.Lists, path) {
				return nil, fmt.Errorf("positive listing failed: %s", path)
			}
			for _, listed := range observations.Lists {
				m.readVersions[listed]++
			}
		} else {
			return nil, &fs.PathError{Op: "observe reads", Path: path, Err: fs.ErrNotExist}
		}
		before[path] = m.readVersions[path]
	}
	return func() ([]string, error) {
		var read []string
		for path, revision := range before {
			if !m.Exists(path) {
				return nil, &fs.PathError{Op: "observe reads", Path: path, Err: fs.ErrNotExist}
			}
			if m.readVersions[path] != revision {
				read = append(read, path)
			}
		}
		slices.Sort(read)
		return read, nil
	}, nil
}

// ObserveWrites observes memory write revisions rather than byte differences.
func (m *memory) ObserveWrites(paths ...string) (func() ([]string, error), error) {
	before := map[string]uint64{}
	for _, path := range paths {
		if _, err := m.Read(path); err != nil {
			return nil, err
		}
		before[filepath.Clean(path)] = m.versions[filepath.Clean(path)]
	}
	return func() ([]string, error) {
		var written []string
		for _, path := range paths {
			if m.versions[filepath.Clean(path)] != before[filepath.Clean(path)] {
				written = append(written, path)
			}
		}
		return written, nil
	}, nil
}

// observedStore records successful writes at the moment they happen, including
// identical JSON, instead of inferring them from the later memory writeback.
// A file where the .metrics folder should be holds no snapshot, as on the
// disk, where making the folder fails: a write there is Unwritable.
type observedStore struct {
	*snapporttest.Store
	memory *memory
	dir    string
}

// Read is the fake's, but a snapshot that does not decode is worded as
// metricsdir words it on the disk, its key and JSON kinds, since the fake
// gives its own plain reason and a scenario hears the binary's line.
func (s observedStore) Read(ctx context.Context, name string, v any) (bool, snapport.Failure) {
	found, failure := s.Store.Read(ctx, name, v)
	var unreadable *snapport.Unreadable
	if found && errors.As(failure, &unreadable) {
		if err := json.Unmarshal([]byte(s.Snapshots[filepath.ToSlash(name)]), v); err != nil {
			return found, metricsdir.Undecodable(name, err)
		}
	}
	return found, failure
}

func (s observedStore) Write(ctx context.Context, name string, v any) snapport.Failure {
	if _, file := s.memory.files[s.dir]; file && ctx.Err() == nil {
		return &snapport.Unwritable{Name: name, Reason: fmt.Sprintf("mkdir %s: not a directory", s.dir)}
	}
	if failure := s.Store.Write(ctx, name, v); failure != nil {
		return failure
	}
	s.memory.versions[filepath.Join(s.dir, filepath.FromSlash(name))]++
	return nil
}

func (s observedStore) Remove(ctx context.Context, name string) snapport.Failure {
	_, existed := s.Snapshots[filepath.ToSlash(name)]
	if failure := s.Store.Remove(ctx, name); failure != nil {
		return failure
	}
	if existed {
		s.memory.versions[filepath.Join(s.dir, filepath.FromSlash(name))]++
	}
	return nil
}
