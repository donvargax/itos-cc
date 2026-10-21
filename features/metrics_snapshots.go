// Snapshot scenarios observe only public commands and their actual artifacts.
package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type snapshotState struct {
	w                    *world
	id, source, snapshot string
	before               []byte
	versions             map[string]map[string]any
	overlap              *snapshotOverlap
}

// metricsSnapshotsFeature declares metrics_snapshots.feature's steps, snapshotSteps, which
// tools/bin/assemble lists in steps_gen.go.
var metricsSnapshotsFeature = featureFile{Name: "metrics_snapshots.feature", Steps: snapshotSteps}

func snapshotSteps(sc *godog.ScenarioContext, w *world) {
	q := &snapshotState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*q = snapshotState{w: w, source: "board.go", versions: map[string]map[string]any{}}
		if filepath.Base(s.Uri) != "metrics_snapshots.feature" {
			return ctx, nil
		}
		for _, tag := range s.Tags {
			if strings.HasPrefix(tag.Name, "@ID-SNAP-") {
				q.id = strings.TrimPrefix(tag.Name, "@ID-SNAP-")
			}
		}
		w.variables = nil
		w.prepare = nil
		if slices.Contains([]string{"08", "09", "10", "11"}, q.id) {
			q.source = "src/board.go"
		}
		if q.id == "04" || q.id == "06" {
			return ctx, q.concurrentProject(ctx)
		}
		dry := false
		for _, step := range s.Steps {
			if strings.Contains(step.Text, `itos-cc dry`) {
				dry = true
			}
		}
		return ctx, q.project(dry)
	})
	sc.Step(`^(\.metrics/\S+) is written$`, q.written)
	sc.Step(`^a command has written its snapshot$`, func() error {
		if err := w.runWith(w.env(), "crap"); err != nil {
			return err
		}
		q.snapshot = ".metrics/crap.json"
		var err error
		q.before, err = w.d.Read(filepath.Join(w.dir, q.snapshot))
		return err
	})
	sc.Step(`^I run it again and nothing in the code has changed$`, func() error { return w.runWith(w.env(), "crap") })
	sc.Step(`^the snapshot file is byte-for-byte the same$`, q.sameBytes)
	sc.Step(`^any snapshot is written$`, q.everySnapshot)
	sc.Step(`^it has "version": 1$`, q.versionOne)
	sc.Step(`^a snapshot is written$`, func() error { q.snapshot = ".metrics/crap.json"; return w.runWith(w.env(), "crap") })
	sc.Step(`^the stored snapshot is one complete valid JSON result$`, q.complete)
	sc.Step(`^two runs writing the same snapshot at once leave one whole snapshot$`, q.concurrentWriters)
	sc.Step(`^raw reports go to .metrics/coverage/, which holds a .gitignore of "\*"$`, q.rawReports)
	sc.Step(`^an agent runs "itos-cc mutation run" while a commit hook runs "itos-cc crap"$`, q.startIsolation)
	sc.Step(`^both run coverage$`, q.captureCollectors)
	sc.Step(`^each writes its reports to a run-\* directory of its own under .metrics/coverage/$`, q.separateReports)
	sc.Step(`^each reads only its own$`, q.consumedReports)
	sc.Step(`^the directory is deleted when the run ends, after its reports replace the previous run's$`, q.latestReports)
	sc.Step(`^a run-\* directory left by a run that was killed a day or more ago$`, q.agedDirectory)
	sc.Step(`^that directory is deleted$`, q.agedRemoved)
	sc.Step(`^a git repository with src/board.go and its tests$`, q.git)
	sc.Step(`^I run "itos-cc ([^"]*)" from src/$`, func(args string) error {
		words, err := commandWords(args)
		if err != nil {
			return err
		}
		return w.runEnv(filepath.Join(w.dir, "src"), w.env(), words...)
	})
	sc.Step(`^the snapshot is (\S+) at the repository's root, and it names the file "([^"]*)"$`, q.names)
	sc.Step(`^no .metrics directory is created under src/$`, q.noNestedMetrics)
	sc.Step(`^(\S+) at the repository's root names (\S+) as "([^"]*)"$`, func(path, file, want string) error {
		if file != want {
			return fmt.Errorf("snapshot expected file mismatch %q %q", file, want)
		}
		if err := q.names(path, want); err != nil {
			return err
		}
		return q.noNestedMetrics()
	})
	sc.Step(`^a run from the repository's root recorded every mutant of src/board.go killed$`, q.killedAtRoot)
	sc.Step(`^"itos-cc mutation check src/board.go" from the root exits 0 too$`, q.checkAtRoot)
	sc.Step(`^stdout's summary names the file "([^"]*)"$`, q.summaryName)
	sc.Step(`^with --json, the file in "files" is "([^"]*)"$`, q.jsonName)
	sc.Step(`^a directory with board.go that is in no git repository$`, q.noGit)
	sc.Step(`^I run "itos-cc crap" there$`, func() error { return w.runWith(w.env(), "crap") })
	sc.Step(`^.metrics/crap.json is written in that directory, naming the file "([^"]*)"$`, func(file string) error { return q.names(".metrics/crap.json", file) })
}

const snapshotSource = "package board\n\nfunc Value(x int) int {\n if x > 0 {\n  return 1\n }\n return 0\n}\n"
const snapshotTests = "package board\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n if Value(1)!=1 { t.Fatal(\"positive\") }\n if Value(0)!=0 { t.Fatal(\"zero\") }\n}\n"

func snapshotProfile(module string, hit bool) string {
	last := "0"
	if hit {
		last = "1"
	}
	return "mode: set\n" + module + "/board.go:4.2,4.11 1 1\n" + module + "/board.go:5.3,6.1 1 1\n" + module + "/board.go:7.2,7.10 1 " + last + "\n"
}

func (q *snapshotState) project(dry bool) error {
	w := q.w
	source := snapshotSource
	if dry {
		body := ""
		for i := range 16 {
			body += fmt.Sprintf(" x += %d\n", i+1)
		}
		source += "\nfunc First(x int) int {\n" + body + " return x\n}\n\nfunc Second(x int) int {\n" + body + " return x\n}\n"
	}
	for file, text := range map[string]string{"go.mod": "module example.com/snapshot\n\ngo 1.22\n", q.source: source, strings.TrimSuffix(q.source, ".go") + "_test.go": snapshotTests} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	// Dry parses the clone fixture without requesting tests or coverage.
	if dry {
		return nil
	}
	dir := filepath.Join(w.dir, filepath.Dir(q.source))
	// itos-cc runs the tests of a module's root package as ./.
	command := "go test -count=1 -failfast ."
	if dir == w.dir {
		command = "go test -count=1 -failfast ./."
	}
	if err := w.d.Tests(dir, Tests{Language: "go", Command: command, Elapsed: 100 * time.Millisecond}); err != nil {
		return err
	}
	if dir != w.dir {
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./src", Elapsed: 100 * time.Millisecond}); err != nil {
			return err
		}
	}
	module := "example.com/snapshot"
	if filepath.Dir(q.source) != "." {
		module += "/src"
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: snapshotProfile(module, true)})
}

func (q *snapshotState) git() error {
	for _, args := range [][]string{{"init", "-q"}, {"add", "go.mod", q.source, strings.TrimSuffix(q.source, ".go") + "_test.go"}, {"commit", "-qm", "snapshot fixture"}} {
		if err := q.w.d.Git(q.w.dir, args...); err != nil {
			return err
		}
	}
	return nil
}
func (q *snapshotState) object(path string) (map[string]any, error) {
	data, err := q.w.d.Read(filepath.Join(q.w.dir, path))
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err = json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("snapshot JSON %s: %w", path, err)
	}
	if v == nil || v["version"] != float64(1) {
		return nil, fmt.Errorf("snapshot object/version %s: %v", path, v)
	}
	return v, nil
}
func (q *snapshotState) written(path string) error {
	path = strings.ReplaceAll(path, "SOURCE_PATH", q.source)
	if q.w.exit != 0 {
		return fmt.Errorf("snapshot command exit %d: %s", q.w.exit, q.w.report())
	}
	_, err := q.object(path)
	return err
}
func (q *snapshotState) sameBytes() error {
	if q.w.exit != 0 {
		return fmt.Errorf("unchanged rerun exit %d: %s", q.w.exit, q.w.report())
	}
	after, err := q.w.d.Read(filepath.Join(q.w.dir, q.snapshot))
	if err != nil {
		return err
	}
	if len(q.before) == 0 || !bytes.Equal(after, q.before) {
		return fmt.Errorf("snapshot bytes changed: before %q after %q", q.before, after)
	}
	return nil
}
func (q *snapshotState) everySnapshot() error {
	for command, path := range map[string]string{"crap": ".metrics/crap.json", "dry": ".metrics/dry.json", "scrap": ".metrics/scrap.json", "mutation run": ".metrics/mutate/board.go.json"} {
		if err := q.w.runWith(q.w.env(), command); err != nil {
			return err
		}
		if q.w.exit != 0 {
			return fmt.Errorf("snapshot version setup %s: %s", command, q.w.report())
		}
		v, err := q.object(path)
		if err != nil {
			return err
		}
		q.versions[path] = v
	}
	return nil
}
func (q *snapshotState) versionOne() error {
	if len(q.versions) != 4 {
		return fmt.Errorf("snapshot version population %d", len(q.versions))
	}
	for path, v := range q.versions {
		if v["version"] != float64(1) {
			return fmt.Errorf("snapshot version %s: %v", path, v["version"])
		}
	}
	return nil
}
func (q *snapshotState) complete() error {
	v, err := q.object(q.snapshot)
	if err != nil {
		return err
	}
	entries := objectList(v, "entries")
	if len(entries) != 2 {
		return fmt.Errorf("complete snapshot expected two actual functions: %v", v)
	}
	want := []string{"board.go", "other/board.go"}
	var got []string
	for _, entry := range entries {
		e := entry
		if e["name"] != "Value" || e["coverage"] != float64(66.7) {
			return fmt.Errorf("complete snapshot entry %v", e)
		}
		got = append(got, fmt.Sprint(e["file"]))
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("complete snapshot files %v", got)
	}
	if err = q.crapPopulation(v, 66.7); err != nil {
		return fmt.Errorf("complete snapshot pairing: %w", err)
	}
	return nil
}
func (q *snapshotState) rawReports(ctx context.Context) error {
	data, err := q.w.d.Read(filepath.Join(q.w.dir, ".metrics/coverage/.gitignore"))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != "*" {
		return fmt.Errorf("raw coverage ignore %q", data)
	}
	files, err := q.w.d.Files(ctx, q.w.dir)
	if err != nil {
		return err
	}
	var reports int
	for _, file := range files {
		if strings.HasSuffix(file, "/coverage.out") {
			if !strings.HasPrefix(file, ".metrics/coverage/go-") {
				return fmt.Errorf("raw coverage outside owned root: %s", file)
			}
			profile, err := q.w.d.Read(filepath.Join(q.w.dir, file))
			if err != nil {
				return err
			}
			if !bytes.Equal(profile, []byte(snapshotProfile("example.com/snapshot", true))) {
				return fmt.Errorf("raw measured profile %s: %q", file, profile)
			}
			reports++
		}
	}
	if reports != 1 {
		return fmt.Errorf("raw report inventory %d", reports)
	}
	return nil
}

func snapshotFiles(v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if k == "file" {
				if s, ok := e.(string); ok {
					out = append(out, s)
				}
			}
			out = append(out, snapshotFiles(e)...)
		}
	case []any:
		for _, e := range x {
			out = append(out, snapshotFiles(e)...)
		}
	}
	return out
}
func (q *snapshotState) names(path, file string) error {
	v, err := q.object(path)
	if err != nil {
		return err
	}
	files := snapshotFiles(v)
	if strings.HasPrefix(path, ".metrics/mutate/") && len(files) != 1 {
		return fmt.Errorf("mutation snapshot exact file population %v", files)
	}
	if len(files) == 0 || !slices.Contains(files, file) {
		return fmt.Errorf("snapshot names %s: %v, want %q", path, files, file)
	}
	for _, got := range files {
		if got != file && !strings.HasPrefix(got, "src/") {
			return fmt.Errorf("snapshot root-relative file %q", got)
		}
	}
	return nil
}
func (q *snapshotState) noNestedMetrics() error {
	if q.w.d.Exists(filepath.Join(q.w.dir, "src/.metrics")) {
		return errors.New("nested snapshot metrics exists")
	}
	return nil
}
func (q *snapshotState) killedAtRoot() error {
	if err := q.git(); err != nil {
		return err
	}
	if err := q.w.runWith(q.w.env(), "mutation run src/board.go"); err != nil {
		return err
	}
	v, err := q.object(".metrics/mutate/src/board.go.json")
	if err != nil {
		return err
	}
	units := objectList(v, "units")
	if q.w.exit != 0 || len(units) != 1 {
		return fmt.Errorf("root killed setup %v: %s", v, q.w.report())
	}
	u := units[0]
	if u["killed"] != float64(4) || u["survived"] != float64(0) || u["uncovered"] != float64(0) || len(objectList(u, "mutants")) != 4 {
		return fmt.Errorf("root killed inventory %v", u)
	}
	return nil
}
func (q *snapshotState) checkAtRoot() error {
	r, err := q.w.d.Run(Run{Dir: q.w.dir, Args: []string{"mutation", "check", "src/board.go"}})
	if err != nil {
		return err
	}
	if r.Exit != 0 {
		return fmt.Errorf("root mutation check exit %d: %s %s", r.Exit, r.Stdout, r.Stderr)
	}
	return nil
}
func (q *snapshotState) summaryName(file string) error {
	if q.w.exit != 0 || !strings.HasPrefix(q.w.stdout, file+": ") {
		return fmt.Errorf("snapshot summary name %q: %s", file, q.w.report())
	}
	return nil
}
func (q *snapshotState) jsonName(file string) error {
	r, err := q.w.d.Run(Run{Dir: filepath.Join(q.w.dir, "src"), Args: []string{"mutation", "run", "--json", "board.go"}})
	if err != nil {
		return err
	}
	var v map[string]any
	if err = json.Unmarshal([]byte(r.Stdout), &v); err != nil {
		return err
	}
	files := objectList(v, "files")
	if r.Exit != 0 || len(files) != 1 {
		return fmt.Errorf("snapshot JSON file inventory %v exit %d", v, r.Exit)
	}
	f := files[0]
	if f["file"] != file {
		return fmt.Errorf("snapshot JSON working directory name %v", f)
	}
	return nil
}
func (q *snapshotState) noGit(ctx context.Context) error {
	if _, err := q.w.d.Head(ctx, q.w.dir); err == nil {
		return errors.New("outside fixture unexpectedly has Git HEAD")
	}
	return nil
}

func (q *snapshotState) agedDirectory(ctx context.Context) error {
	for _, file := range []string{"run-aged/marker", "run-fresh/marker", "kept-report/coverage.out"} {
		if err := q.w.write(".metrics/coverage/"+file, "owned "+file); err != nil {
			return err
		}
	}
	observed, err := q.w.d.SetModTime(ctx, filepath.Join(q.w.dir, ".metrics/coverage/run-aged"), time.Now().Add(-48*time.Hour))
	if errors.Is(err, ErrPhysicalUnsupported) {
		return fmt.Errorf("%w: %v", godog.ErrSkip, err)
	}
	if err != nil {
		return err
	}
	if time.Since(observed) < 47*time.Hour {
		return fmt.Errorf("aged entry observed too young %v", observed)
	}
	return nil
}
func (q *snapshotState) agedRemoved() error {
	if q.w.d.Exists(filepath.Join(q.w.dir, ".metrics/coverage/run-aged")) {
		return errors.New("aged run directory remains")
	}
	for _, file := range []string{"run-fresh/marker", "kept-report/coverage.out"} {
		data, err := q.w.d.Read(filepath.Join(q.w.dir, ".metrics/coverage", file))
		if err != nil {
			return err
		}
		if string(data) != "owned "+file {
			return fmt.Errorf("cleanup control changed %s: %q", file, data)
		}
	}
	return nil
}

// Saved public result equality deliberately uses complete entries, not only
// their percentages, so file/function attribution remains paired.
func snapshotEntriesEqual(a, b map[string]any) bool {
	return reflect.DeepEqual(a["entries"], b["entries"])
}
