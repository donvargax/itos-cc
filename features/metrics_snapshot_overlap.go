package features

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type snapshotRun struct {
	command, side, name, root string
	pool                      FixturePeers
	peer                      FixturePeer
	run                       Running
	result                    Result
}
type snapshotOverlap struct {
	out    string
	runs   []*snapshotRun
	latest map[string][]byte
	crap   map[string]any
}

func (q *snapshotState) concurrentProject(ctx context.Context) error {
	pool, err := q.w.d.FixturePeers(ctx)
	if errors.Is(err, ErrPhysicalUnsupported) {
		return godog.ErrSkip
	}
	if err != nil {
		return err
	}
	if err = pool.Close(ctx); err != nil {
		return err
	}
	w := q.w
	out, err := w.d.Folder()
	if err != nil {
		return err
	}
	q.overlap = &snapshotOverlap{out: out, latest: map[string][]byte{}}
	w.beforeRemove = q.cleanupOverlap
	for dir, module := range map[string]string{".": "example.com/root", "other": "example.com/other"} {
		tests := strings.ReplaceAll(snapshotConcurrentTests, "ORIGINAL_ROOT", strconv.Quote(w.dir))
		for name, text := range map[string]string{"go.mod": "module " + module + "\n\ngo 1.22\n", "board.go": snapshotSource, "board_test.go": tests} {
			if err = w.write(filepath.Join(dir, name), text); err != nil {
				return err
			}
		}
		path := filepath.Join(w.dir, dir)
		// The initial ordinary crap command runs only the positive arm. Mutating
		// runs below redeclare this same real command with full-reaching outcomes.
		if err = w.d.Tests(path, Tests{Language: "go", Command: "go test -count=1 -failfast .", Elapsed: 100 * time.Millisecond, Surviving: []Site{{File: "board.go", Line: 4, Original: ">", Replacement: ">="}, {File: "board.go", Line: 7, Original: "0", Replacement: "1"}}}); err != nil {
			return err
		}
		if err = w.d.Coverage(path, Coverage{Language: "go", Report: snapshotProfile(module, false)}); err != nil {
			return err
		}
	}
	return q.gitConcurrent()
}
func (q *snapshotState) gitConcurrent() error {
	for _, args := range [][]string{{"init", "-q"}, {"add", "go.mod", "board.go", "board_test.go", "other/go.mod", "other/board.go", "other/board_test.go"}, {"commit", "-qm", "concurrent snapshot fixture"}} {
		if err := q.w.d.Git(q.w.dir, args...); err != nil {
			return err
		}
	}
	return nil
}
func (q *snapshotState) cleanupOverlap(ctx context.Context) error {
	if q.overlap == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var failures []error
	for _, r := range q.overlap.runs {
		if r.run != nil {
			failures = append(failures, r.run.Interrupt(ctx))
		}
	}
	// Cooperative stop is cleanup only, after product admission has stopped.
	for _, r := range q.overlap.runs {
		if r.peer != nil {
			failures = append(failures, r.peer.Stop(ctx))
		}
	}
	// Close drains every authenticated connection, including a helper whose
	// Capture failed before it could be assigned to r.peer. Do this before
	// Wait so a held test cannot outlive its product pipe join.
	for _, r := range q.overlap.runs {
		if r.pool != nil {
			failures = append(failures, r.pool.Close(ctx))
		}
	}
	for _, r := range q.overlap.runs {
		if r.run != nil {
			_, err := r.run.Wait(ctx)
			failures = append(failures, err)
		}
		if r.peer != nil {
			failures = append(failures, r.peer.EOF(ctx))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return q.w.d.Remove(q.overlap.out)
}

func (q *snapshotState) launch(ctx context.Context, commands ...string) error {
	for i, command := range commands {
		r := &snapshotRun{command: command, side: "crap", name: strconv.Itoa(i)}
		if i == 0 {
			r.side = "mutation"
		}
		q.overlap.runs = append(q.overlap.runs, r)
		var err error
		r.pool, err = q.w.d.FixturePeers(ctx)
		if errors.Is(err, ErrPhysicalUnsupported) {
			return godog.ErrSkip
		}
		if err != nil {
			return err
		}
		env := append(r.pool.Environment(), "T82_SIDE="+r.side, "T82_HOLD=yes", "T82_TRACE="+filepath.Join(q.overlap.out, r.name+".jsonl"), "T82_READY="+filepath.Join(q.overlap.out, r.name+".ready"), "T82_RELEASE="+filepath.Join(q.overlap.out, r.name+".release"))
		for dir, module := range map[string]string{".": "example.com/root", "other": "example.com/other"} {
			t := Tests{Language: "go", Command: "go test -count=1 -failfast .", Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.overlap.out, r.name+".jsonl"), Source: "board.go"}}
			if r.side == "crap" {
				t.Surviving = []Site{{File: "board.go", Line: 4, Original: ">", Replacement: ">="}, {File: "board.go", Line: 7, Original: "0", Replacement: "1"}}
			}
			if err = q.w.d.Tests(filepath.Join(q.w.dir, dir), t); err != nil {
				return err
			}
			if err = q.w.d.Coverage(filepath.Join(q.w.dir, dir), Coverage{Language: "go", Report: snapshotProfile(module, r.side == "mutation")}); err != nil {
				return err
			}
		}
		args, err := commandWords(command)
		if err != nil {
			return err
		}
		r.run, err = q.w.d.Start(ctx, Run{Dir: q.w.dir, Args: args, Env: env})
		if errors.Is(err, ErrPhysicalUnsupported) {
			return godog.ErrSkip
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (q *snapshotState) startIsolation(ctx context.Context) error {
	return q.launch(ctx, "mutation run", "crap")
}
func (q *snapshotState) captureCollectors(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, r := range q.overlap.runs {
		var err error
		r.peer, err = r.pool.Capture(ctx, filepath.Join(q.overlap.out, r.name+".ready"))
		if err != nil {
			return err
		}
		var record map[string]any
		if err = json.Unmarshal(r.peer.Record(), &record); err != nil {
			return err
		}
		if record["side"] != r.side || record["source_sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(snapshotSource))) || !strings.HasSuffix(fmt.Sprint(record["cwd"]), string(filepath.Separator)+"other") {
			return fmt.Errorf("collector readiness identity %v", record)
		}
		rel, err := filepath.Rel(fmt.Sprint(record["original_root"]), fmt.Sprint(record["cover_dir"]))
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 5 || parts[0] != ".metrics" || parts[1] != "coverage" || !strings.HasPrefix(parts[2], "run-") || !strings.HasPrefix(parts[3], "go-") || parts[4] != "gocoverdir" {
			return fmt.Errorf("collector owned report root %v", record)
		}
		r.root = parts[2]
	}
	return nil
}
func (q *snapshotState) separateReports(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	files, err := q.w.d.Files(ctx, filepath.Join(q.w.dir, ".metrics/coverage"))
	if err != nil {
		return err
	}
	var roots []string
	for _, r := range q.overlap.runs {
		var matches []string
		want := []byte(snapshotProfile("example.com/root", r.side == "mutation"))
		for _, file := range files {
			parts := strings.Split(file, "/")
			if len(parts) != 3 || parts[0] != r.root || parts[2] != "coverage.out" {
				continue
			}
			data, err := q.w.d.Read(filepath.Join(q.w.dir, ".metrics/coverage", file))
			if err != nil {
				return err
			}
			if bytes.Equal(data, want) {
				matches = append(matches, parts[0])
			}
		}
		if len(matches) != 1 {
			return fmt.Errorf("separate actual report %s matches %v in %v", r.side, matches, files)
		}
		roots = append(roots, r.root)
		if err = r.peer.Ping(ctx); err != nil {
			return err
		}
	}
	if len(roots) != 2 || roots[0] == roots[1] {
		return fmt.Errorf("coverage run roots not distinct %v", roots)
	}
	return nil
}

func (q *snapshotState) releaseMarker(r *snapshotRun) error {
	return q.w.d.Write(filepath.Join(q.overlap.out, r.name+".release"), []byte("release"))
}
func (q *snapshotState) release(ctx context.Context, r *snapshotRun) error {
	if err := q.releaseMarker(r); err != nil {
		return err
	}
	return q.join(ctx, r)
}
func (q *snapshotState) join(ctx context.Context, r *snapshotRun) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var err error
	r.result, err = r.run.Wait(ctx)
	if err != nil {
		return err
	}
	if err = r.peer.EOF(ctx); err != nil {
		return err
	}
	if r.result.Exit != 0 {
		return fmt.Errorf("overlap command %s exit %d: %s %s", r.command, r.result.Exit, r.result.Stdout, r.result.Stderr)
	}
	return nil
}
func (q *snapshotState) consumedReports(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	a, b := q.overlap.runs[0], q.overlap.runs[1]
	if err := q.release(ctx, a); err != nil {
		return err
	}
	for _, file := range []string{"board.go", "other/board.go"} {
		if !strings.Contains(a.result.Stdout, "4 killed, 0 survived, 0 uncovered in "+file+" (ran 4, reused 0)") {
			return fmt.Errorf("mutation consumed profile %s: %s", file, a.result.Stdout)
		}
		v, err := q.object(".metrics/mutate/" + file + ".json")
		if err != nil {
			return err
		}
		units := objectList(v, "units")
		if len(units) != 1 {
			return fmt.Errorf("mutation snapshot unit population %v", v)
		}
		u := units[0]
		if u["name"] != "Value" || u["killed"] != float64(4) || u["uncovered"] != float64(0) || len(objectList(u, "mutants")) != 4 {
			return fmt.Errorf("mutation own outcomes %v", u)
		}
		data, err := q.w.d.Read(filepath.Join(q.w.dir, file))
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "4 killed") {
			return fmt.Errorf("normal mutation annotation missing %s: %s", file, data)
		}
	}
	if q.w.d.Exists(filepath.Join(q.w.dir, ".metrics/coverage", a.root)) || !q.w.d.Exists(filepath.Join(q.w.dir, ".metrics/coverage", b.root)) {
		return fmt.Errorf("staged coverage cleanup before second release")
	}
	if err := b.peer.Ping(ctx); err != nil {
		return err
	}
	if err := q.saveLatest(ctx, true); err != nil {
		return err
	}
	if err := q.release(ctx, b); err != nil {
		return err
	}
	v, err := q.object(".metrics/crap.json")
	if err != nil {
		return err
	}
	if err = q.crapPopulation(v, 66.7); err != nil {
		return err
	}
	q.overlap.crap = v
	if err = q.writerTable(b, 66.7); err != nil {
		return err
	}
	return nil
}

func (q *snapshotState) crapPopulation(v map[string]any, coverage float64) error {
	entries := objectList(v, "entries")
	if len(entries) != 2 {
		return fmt.Errorf("own report public population %v", v)
	}
	var files []string
	for _, entry := range entries {
		e := entry
		namespace := "example.com/root"
		if e["file"] == "other/board.go" {
			namespace = "example.com/other"
		}
		score := float64(2)
		if coverage == 66.7 {
			score = 2.1
		}
		if e["name"] != "Value" || e["coverage"] != coverage || e["complexity"] != float64(2) || e["namespace"] != namespace || e["language"] != "go" || e["start_line"] != float64(3) || e["end_line"] != float64(8) || e["crap"] != score {
			return fmt.Errorf("own measured public entry %v want coverage %v", e, coverage)
		}
		files = append(files, fmt.Sprint(e["file"]))
	}
	slices.Sort(files)
	if !slices.Equal(files, []string{"board.go", "other/board.go"}) {
		return fmt.Errorf("own measured file attribution %v", files)
	}
	return nil
}
func (q *snapshotState) saveLatest(ctx context.Context, full bool) error {
	files, err := q.w.d.Files(ctx, filepath.Join(q.w.dir, ".metrics/coverage"))
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, file := range files {
		parts := strings.Split(file, "/")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "go-") || parts[1] != "coverage.out" {
			continue
		}
		data, err := q.w.d.Read(filepath.Join(q.w.dir, ".metrics/coverage", file))
		if err != nil {
			return err
		}
		match := ""
		for _, module := range []string{"example.com/root", "example.com/other"} {
			if bytes.Equal(data, []byte(snapshotProfile(module, full))) {
				match = module
			}
		}
		if match == "" || found[match] {
			return fmt.Errorf("latest measured report %s: %q", file, data)
		}
		found[match] = true
		q.overlap.latest[file] = data
	}
	if len(found) != 2 {
		return fmt.Errorf("latest report population %v", found)
	}
	return nil
}
func (q *snapshotState) latestReports(ctx context.Context) error {
	for _, run := range q.overlap.runs {
		if q.w.d.Exists(filepath.Join(q.w.dir, ".metrics/coverage", run.root)) {
			return fmt.Errorf("completed owned run directory remains %s", run.root)
		}
	}
	if err := q.saveLatest(ctx, false); err != nil {
		return err
	}
	files, err := q.w.d.Files(ctx, filepath.Join(q.w.dir, ".metrics/coverage"))
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasPrefix(file, "run-") {
			return fmt.Errorf("completed run data remains %s", file)
		}
	}
	trace := map[string][]byte{}
	for _, r := range q.overlap.runs {
		data, err := q.w.d.Read(filepath.Join(q.overlap.out, r.name+".jsonl"))
		if err != nil {
			return err
		}
		trace[r.name] = data
	}
	r, err := q.w.d.Run(Run{Dir: q.w.dir, Args: []string{"crap", "--use-existing-coverage", "--json"}})
	if err != nil {
		return err
	}
	var v map[string]any
	if err = json.Unmarshal([]byte(r.Stdout), &v); err != nil {
		return err
	}
	if r.Exit != 0 || !snapshotEntriesEqual(v, q.overlap.crap) {
		return fmt.Errorf("latest public reuse differs %v exit %d", v, r.Exit)
	}
	for _, run := range q.overlap.runs {
		data, err := q.w.d.Read(filepath.Join(q.overlap.out, run.name+".jsonl"))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, trace[run.name]) {
			return fmt.Errorf("latest reuse executed collector %s", run.name)
		}
	}
	for file, want := range q.overlap.latest {
		data, err := q.w.d.Read(filepath.Join(q.w.dir, ".metrics/coverage", file))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, want) {
			return fmt.Errorf("latest reuse changed report %s", file)
		}
	}
	return nil
}
func (q *snapshotState) concurrentWriters(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := q.launch(ctx, "crap", "crap"); err != nil {
		return err
	}
	if err := q.captureCollectors(ctx); err != nil {
		return err
	}
	if err := q.separateReports(ctx); err != nil {
		return err
	}
	a, b := q.overlap.runs[0], q.overlap.runs[1]
	// Both real measurements become runnable before either product is joined:
	// this leaves publication genuinely concurrent, unlike staged SNAP06.
	if err := q.releaseMarker(a); err != nil {
		return err
	}
	if err := q.releaseMarker(b); err != nil {
		return err
	}
	if err := q.join(ctx, a); err != nil {
		return err
	}
	if err := q.join(ctx, b); err != nil {
		return err
	}
	if err := q.writerTable(a, 100); err != nil {
		return err
	}
	if err := q.writerTable(b, 66.7); err != nil {
		return err
	}
	last, err := q.object(".metrics/crap.json")
	if err != nil {
		return err
	}
	entries := objectList(last, "entries")
	if len(entries) != 2 {
		return fmt.Errorf("concurrent whole population %v", last)
	}
	coverage, ok := entries[0]["coverage"].(float64)
	if !ok || (coverage != 100 && coverage != 66.7) {
		return fmt.Errorf("concurrent whole writer result %v", last)
	}
	return q.crapPopulation(last, coverage)
}
func (q *snapshotState) writerTable(r *snapshotRun, coverage float64) error {
	var files []string
	for _, line := range strings.Split(r.result.Stdout, "\n") {
		fields := crapRow.FindStringSubmatch(line)
		if fields == nil {
			continue
		}
		want := fmt.Sprintf("%.1f", coverage)
		score := "2.0"
		if coverage == 66.7 {
			score = "2.1"
		}
		if fields[1] != score || fields[2] != "2" || fields[3] != want {
			return fmt.Errorf("independent writer table %s: %s", r.name, line)
		}
		f := rowFunction.FindStringSubmatch(fields[4])
		if f == nil || f[2] != "Value" || f[4] != "3" {
			return fmt.Errorf("independent writer subject %s: %s", r.name, line)
		}
		namespace := "example.com/root"
		if f[3] == "other/board.go" {
			namespace = "example.com/other"
		}
		if f[1] != namespace {
			return fmt.Errorf("independent writer namespace %s: %s", r.name, line)
		}
		files = append(files, f[3])
	}
	slices.Sort(files)
	if !slices.Equal(files, []string{"board.go", "other/board.go"}) {
		return fmt.Errorf("independent writer population %s: %v", r.name, files)
	}
	return nil
}
