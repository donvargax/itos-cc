package features

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (r *resultMutation) lateSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Python project with a file no test reaches$`, r.unreachedPython)
	sc.Step(`^I run "itos-cc ([^"]*)" for (that file|its file|a file|its production file|the production file|one of its files)$`, func(args, _ string) error {
		if r.s.m.forFile != nil {
			return r.s.m.forFile(args)
		}
		return r.run(args + " " + r.file)
	})
	sc.Step(`^its "baseline" is "not-run" and its mutants are uncovered$`, r.unreachedResult)
	sc.Step(`^a file whose tests ran reports its baseline as "passed", as before$`, r.reachedResult)
	sc.Step(`^a committed Python project whose file no own test reaches, but a listed test reaches one of its lines and kills its mutant$`, r.listedPython)
	sc.Step(`^a count-one run judges that file$`, func() error {
		if err := r.s.m.w.d.Counted(); err != nil {
			return err
		}
		return r.run("mutation run --count 1 --seed 1 --workers 1 --json " + r.file)
	})
	sc.Step(`^its mutant is judged by the listed test and killed, as a complete run judges it$`, r.countedListed)

	sc.Step(`^a Python project whose coverage\.py configuration sets parallel = true, with a test that kills its mutant$`, func() error {
		return r.python("calc.py", "def mid(i):\n    return i == 5\n", "test_calc.py", "from calc import mid\n\ndef test_mid():\n    assert mid(5)\n    assert not mid(4)\n", true)
	})
	sc.Step(`^its coverage is measured, not "coverage\.measured-nothing", and its mutant is killed$`, r.parallelMeasured)
	sc.Step(`^nothing of the run's data files is left in the project's tree$`, r.noParallelData)
	sc.Step(`^worker copies a mutation run left in the temporary directory when it was killed$`, r.deadWorkers)
	sc.Step(`^worker copies of a mutation run still going$`, r.liveWorkers)
	sc.Step(`^the killed run's worker copies are removed$`, func() error {
		if r.s.m.w.d.Exists(r.killed) {
			return fmt.Errorf("killed worker directory remains: %s", r.killed)
		}
		return nil
	})
	sc.Step(`^the running one's are left alone$`, r.liveKept)
	sc.Step(`^fresh results for "Board#place" that record a surviving mutant at a site the function no longer holds, as when a newer itos-cc removes an operator, and every other mutant killed$`, func() error { return r.removedSite("survived") })
	sc.Step(`^no "mutation\.survived" is reported for that mutant, and the exit code is 0$`, r.goneNotReported)
	sc.Step(`^"itos-cc mutation run src/board\.ts" runs no mutant, and rewrites the entry without it$`, r.dropGone)
	sc.Step(`^a snapshot under \.metrics/mutate for src/old\.ts, a file that no longer exists$`, r.orphan)
	sc.Step(`^it names the orphaned snapshot, without failing any function for it$`, r.orphanNamed)
	sc.Step(`^the orphaned snapshot is removed, and the snapshots of files that exist are kept$`, r.orphanRemoved)
	sc.Step(`^a snapshot that "itos-cc mutation run" wrote for a file outside the project root, which still exists$`, r.outsideSnapshot)
	sc.Step(`^I run "itos-cc ([^"]*)" at the project root$`, r.run)
	sc.Step(`^it names no orphan$`, r.noOrphan)
	sc.Step(`^that snapshot is kept$`, r.outsideKept)
	sc.Step(`^src/old\.ts, with a snapshot, renamed to lib/new\.ts in a commit since HEAD~1$`, func() error { return r.renameOld(true) })
	sc.Step(`^src/old\.ts, with a snapshot, renamed to lib/new\.ts with git mv and not committed$`, func() error { return r.renameOld(false) })
	sc.Step(`^the snapshot of src/old\.ts is kept, and no orphan is named$`, r.oldKept)
	sc.Step(`^the snapshot moves to lib/new\.ts, and src/old\.ts's is gone$`, r.oldMoved)
	sc.Step(`^"itos-cc mutation check src" names no orphan$`, func() error {
		if err := r.run("mutation check src"); err != nil {
			return err
		}
		return r.noOrphan()
	})
	sc.Step(`^once the rename is committed, "itos-cc mutation run --since HEAD~1 lib" moves the snapshot to lib/new\.ts$`, func() error {
		if err := r.s.commit("commit rename"); err != nil {
			return err
		}
		if err := r.run("mutation run --since HEAD~1 lib"); err != nil {
			return err
		}
		return r.oldMoved()
	})
	sc.Step(`^a file whose snapshot is fresh, so that a run reuses every one of its mutants$`, r.freshFile)
	sc.Step(`^its "baseline" is "not-run", and it ran no mutant$`, r.reusedBaseline)
	sc.Step(`^after an edit to one of its functions, the next run reports its "baseline" as "passed"$`, r.changedBaseline)
	sc.Step(`^fresh results for "Board#place" that record a killed mutant at a site the function no longer holds, and every other mutant killed$`, func() error { return r.removedSite("killed") })
	sc.Step(`^the "functions" entry of "Board#place" has "left_out" 1, and no other function has "left_out"$`, r.leftOut)
	sc.Step(`^"itos-cc mutation check src/board\.ts" prints a "left out" line naming "Board#place" and 1 mutant$`, r.leftOutLine)
}
func (r *resultMutation) unreachedPython() error {
	if err := r.python("a.py", "def place(i):\n    return i > 5\n", "test_a.py", "from a import place\n\ndef test_place():\n    assert place(6)\n    assert not place(5)\n", false); err != nil {
		return err
	}
	if err := r.s.m.w.write("u.py", "def unused(i):\n    return i > 5\n"); err != nil {
		return err
	}
	r.file = "u.py"
	return nil
}
func (r *resultMutation) unreachedResult() error {
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if f["baseline"] != "not-run" || f["uncovered"] != float64(1) || f["ran"] != float64(0) || f["killed"] != float64(0) || f["survived"] != float64(0) || len(entries) != 1 || entries[0]["outcome"] != "uncovered" {
		return fmt.Errorf("unreached file %v", f)
	}
	if len(baselines(r.s.m.w.stderr)) != 0 {
		return fmt.Errorf("unreached baseline ran: %s", r.s.m.w.stderr)
	}
	return nil
}
func (r *resultMutation) reachedResult() error {
	w := r.s.m.w
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "run", "--json", "--no-annotate", "a.py")
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	if code != 0 || len(files) != 1 || files[0].(map[string]any)["baseline"] != "passed" || len(baselines(stderr)) != 1 {
		return fmt.Errorf("reached file exit%d: %s%s", code, out, stderr)
	}
	return nil
}
func (r *resultMutation) listedPython() error {
	if err := r.s.m.w.d.Counted(); err != nil {
		return err
	}
	test := "import importlib\n\ndef test_mid():\n    calc = importlib.import_module(\"c\" + \"alc\")\n    assert calc.mid(5)\n    assert not calc.mid(4)\n"
	if err := r.python("calc.py", "def mid(i):\n    return i == 5\n", "e2e_mid.py", test, false); err != nil {
		return err
	}
	w := r.s.m.w
	config := "mutation:\n  tests:\n    list: cat tests.txt\n    run: python3 -m pytest -q -p no:cacheprovider e2e_mid.py -k \"{pattern}\"\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \" or \"\n"
	if err := w.write("tests.txt", "mid\te2e_mid.py\n"); err != nil {
		return err
	}
	if err := w.write("itos-cc.yaml", config); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "python"}); err != nil {
		return err
	}
	t := Tests{Language: "python", Command: "", Listed: &ListedTests{Files: map[string]string{"mid": "e2e_mid.py"}, Lines: map[string]map[int][]string{"calc.py": {2: {"mid"}}}, Elapsed: map[string]time.Duration{"mid": time.Second}, Surviving: map[string][]Site{}, TimingOut: map[string][]Site{}}}
	for _, command := range []string{"", "python3 -m pytest -q -p no:cacheprovider e2e_mid.py -k \"mid\""} {
		t.Command = command
		if err := w.d.Tests(w.dir, t); err != nil {
			return err
		}
	}
	return r.s.commit("declare listed tests")
}
func (r *resultMutation) countedListed() error {
	obj, err := r.s.m.w.json()
	if err != nil {
		return err
	}
	selected, _ := obj["selected"].([]any)
	sampling, _ := obj["sampling"].(map[string]any)
	if len(selected) != 1 {
		return fmt.Errorf("counted selected %v", selected)
	}
	mu := selected[0].(map[string]any)
	ids, _ := mu["tests"].([]any)
	if mu["state"] != "judged" || mu["outcome"] != "killed" || mu["scope"] != "listed" || len(ids) != 1 || ids[0] != "mid" || sampling["executed"] != float64(1) || sampling["completion"] != "completed" || r.s.m.w.exit != 0 {
		return fmt.Errorf("counted listed judgment %v", obj)
	}
	w := r.s.m.w
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "run", "--json", "--no-annotate", r.file)
	if err != nil {
		return err
	}
	var full map[string]any
	if err := json.Unmarshal([]byte(out), &full); err != nil {
		return err
	}
	entries := sampleEntries(full)
	if code != 0 || len(entries) != 1 || entries[0]["outcome"] != "killed" || entries[0]["scope"] != "listed" || entries[0]["original"] != "==" || entries[0]["replacement"] != "!=" {
		return fmt.Errorf("complete listed judgment exit%d: %s%s", code, out, stderr)
	}
	return nil
}
func (r *resultMutation) parallelMeasured() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	if len(checkProblems(obj)) != 0 || strings.Contains(r.s.m.w.stderr, "coverage.measured-nothing") {
		return fmt.Errorf("collector problems %v", obj)
	}
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0]["outcome"] != "killed" || f["killed"] != float64(1) || f["uncovered"] != float64(0) {
		return fmt.Errorf("parallel collector verdict %v", f)
	}
	sources, _ := entries[0]["coverage"].([]any)
	if len(sources) != 1 || sources[0] != "in-process" {
		return fmt.Errorf("coverage provenance %v", entries[0])
	}
	return nil
}

// noParallelData rejects raw parallel files and data outside the retained report.
// The report keeps .metrics/coverage/<plan>/.coverage beside its LCOV artifact;
// that one durable report-data file is not an abandoned run data file.
func (r *resultMutation) noParallelData(ctx context.Context) error {
	files, err := r.s.m.w.d.Files(ctx, r.s.m.w.dir)
	if err != nil {
		return err
	}
	var left []string
	for _, file := range files {
		name := path.Base(file)
		kept := name == ".coverage" && path.Dir(path.Dir(file)) == ".metrics/coverage"
		if strings.HasPrefix(name, ".coverage.") || name == ".coverage" && !kept {
			left = append(left, file)
		}
	}
	if len(left) != 0 {
		return fmt.Errorf("parallel data left in project: %v", left)
	}
	return nil
}
func (r *resultMutation) workerCopies(name string) (string, error) {
	dir := filepath.Join(r.temp, name)
	for _, worker := range []string{"w0", "w1"} {
		if err := r.s.m.w.d.Write(filepath.Join(dir, worker, "src/board.go"), []byte("package board\n")); err != nil {
			return "", err
		}
	}
	if err := r.s.m.w.d.Write(filepath.Join(dir, "owner.lock"), []byte("itos-cc pid 999999\n")); err != nil {
		return "", err
	}
	return dir, nil
}
func (r *resultMutation) deadWorkers() error {
	if err := r.s.project(false); err != nil {
		return err
	}
	r.file = r.s.file
	var err error
	r.temp, err = r.s.m.w.d.Folder()
	if err != nil {
		return err
	}
	r.s.m.w.variables = []string{"TMPDIR=" + r.temp, "TMP=" + r.temp, "TEMP=" + r.temp}
	previous := r.s.m.w.prepare
	r.s.m.w.prepare = func(args []string) []string { args = previous(args); return append(args, "--no-coverage") }
	r.killed, err = r.workerCopies("itos-cc-mutate-1000001")
	return err
}
func (r *resultMutation) liveWorkers(ctx context.Context) error {
	var err error
	r.live, err = r.workerCopies("itos-cc-mutate-1000002")
	if err != nil {
		return err
	}
	r.release, err = r.s.m.w.d.Lock(ctx, filepath.Join(r.live, "owner.lock"))
	return err
}
func (r *resultMutation) liveKept() error {
	d := r.s.m.w.d
	if !d.Exists(filepath.Join(r.live, "owner.lock")) {
		return fmt.Errorf("live marker removed")
	}
	for _, worker := range []string{"w0", "w1"} {
		file := filepath.Join(r.live, worker, "src/board.go")
		data, err := d.Read(file)
		if err != nil || string(data) != "package board\n" {
			return fmt.Errorf("live copy %s changed: %q, %v", file, data, err)
		}
	}
	return nil
}
func (r *resultMutation) removedSite(outcome string) error {
	if err := r.strong(); err != nil {
		return err
	}
	if err := r.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	snap, err := r.s.snapshot(r.file)
	if err != nil {
		return err
	}
	units := r.s.units(snap, r.s.place)
	if len(units) != 1 {
		return fmt.Errorf("place snapshot %v", units)
	}
	u := units[0]
	mutants := u["mutants"].([]any)
	if len(mutants) != 1 {
		return fmt.Errorf("place mutants %v", mutants)
	}
	gone := maps.Clone(mutants[0].(map[string]any))
	gone["replacement"] = "<"
	gone["outcome"] = outcome
	u["mutants"] = append(mutants, gone)
	u["sites"] = u["sites"].(float64) + 1
	u[outcome] = u[outcome].(float64) + 1
	return (&checkMutation{s: r.s}).writeSnapshot(snap)
}
func (r *resultMutation) goneNotReported() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	if len(checkProblems(obj)) != 0 || r.s.m.w.exit != 0 {
		return fmt.Errorf("gone survivor still fails: %v", obj)
	}
	functions := resultCheckFunctions(obj, r.file)
	n := 0
	for _, fn := range functions {
		if sinceFunction(listedSite{function: fmt.Sprint(fn["function"])}, r.s.place) {
			n++
			if fn["state"] != "fresh" || fn["survived"] != float64(0) || fn["killed"] != float64(1) {
				return fmt.Errorf("gone-site counts %v", fn)
			}
		}
	}
	if n != 1 {
		return fmt.Errorf("place functions %v", functions)
	}
	return nil
}
func (r *resultMutation) dropGone() error {
	if err := r.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	if f["ran"] != float64(0) || r.s.m.w.exit != 0 || len(trialsOf(r.s.m.w.stderr, r.file)) != 0 {
		return fmt.Errorf("removed-site run %v", f)
	}
	snap, err := r.s.snapshot(r.file)
	if err != nil {
		return err
	}
	units := r.s.units(snap, r.s.place)
	if len(units) != 1 {
		return fmt.Errorf("place units %v", units)
	}
	u := units[0]
	mutants := u["mutants"].([]any)
	if len(mutants) != 1 || mutants[0].(map[string]any)["replacement"] != ">=" || u["sites"] != float64(1) || u["killed"] != float64(1) || u["survived"] != float64(0) {
		return fmt.Errorf("removed mutant remains: %v", u)
	}
	return nil
}
func resultOrphan(file string) string {
	return `{"version":1,"file":` + fmt.Sprintf("%q", file) + `,"language":"typescript","tests":{},"units":[{"namespace":"old","name":"old","hash":"0000","start_line":1,"end_line":3,"killed":0,"survived":1,"uncovered":0,"sites":1,"mutants":[{"line":2,"column":12,"offset":30,"original":">","replacement":">=","outcome":"survived"}]}]}` + "\n"
}
func (r *resultMutation) orphan() error {
	if err := r.strong(); err != nil {
		return err
	}
	if err := r.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	r.held = map[string]string{".metrics/mutate/src/board.ts.json": r.s.m.read(".metrics/mutate/src/board.ts.json")}
	return r.s.m.w.write(".metrics/mutate/src/old.ts.json", resultOrphan("src/old.ts"))
}
func (r *resultMutation) orphanNamed() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	orphans, _ := obj["orphans"].([]any)
	if len(orphans) != 1 || len(checkProblems(obj)) != 0 || r.s.m.w.exit != 0 {
		return fmt.Errorf("orphan output %v", obj)
	}
	orphan := orphans[0].(map[string]any)
	if orphan["file"] != "src/old.ts" || orphan["snapshot"] != ".metrics/mutate/src/old.ts.json" || orphan["why"] != "gone" || !strings.Contains(r.s.m.w.stdout, ".metrics/mutate/src/old.ts.json") {
		return fmt.Errorf("orphan metadata %v", orphan)
	}
	return nil
}
func (r *resultMutation) orphanRemoved() error {
	if r.s.m.w.d.Exists(filepath.Join(r.s.m.w.dir, ".metrics/mutate/src/old.ts.json")) {
		return fmt.Errorf("orphan snapshot remains")
	}
	for file, want := range r.held {
		if r.s.m.read(file) != want {
			return fmt.Errorf("existing snapshot %s changed", file)
		}
	}
	return nil
}
func (r *resultMutation) outsideSnapshot() error {
	if err := r.strong(); err != nil {
		return err
	}
	w := r.s.m.w
	var err error
	r.outside, err = w.d.Folder()
	if err != nil {
		return err
	}
	tests := strings.Replace(sinceBoardTestTS, `expect(new Board().clear(2)).toBe(true);`, `expect(new Board().clear(2)).toBe(true); expect(new Board().clear(3)).toBe(false);`, 1)
	for file, text := range map[string]string{"package.json": vitestPackage, "src/board.ts": sinceBoardTS, "src/board.test.ts": tests} {
		if err := w.d.Write(filepath.Join(r.outside, file), []byte(text)); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(r.outside, Coverage{Language: "typescript", Report: "SF:src/board.ts\nDA:4,1\nDA:7,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := w.d.Tests(r.outside, Tests{Language: "typescript", Command: vitestCommand + "src/board.ts"}); err != nil {
		return err
	}
	file := filepath.Join(r.outside, "src/board.ts")
	// Some real filesystems cannot name a drive colon inside a snapshot key.
	// An ordinary owned sibling must work before this one capability can skip.
	r.externalSnapshot = filepath.Join(w.dir, ".metrics/mutate", filepath.FromSlash(filepath.ToSlash(file)+".json"))
	sibling := filepath.Join(w.dir, ".metrics/mutate/fixture-name-probe.json")
	if err := w.d.Write(sibling, []byte("{}\n")); err != nil {
		return err
	}
	if err := w.d.Remove(sibling); err != nil {
		return err
	}
	if err := w.d.Write(r.externalSnapshot, []byte("{}\n")); err != nil {
		if strings.Contains(filepath.VolumeName(file), ":") {
			return fmt.Errorf("%w: cannot name snapshot %s for source %s: %v", godog.ErrSkip, r.externalSnapshot, file, err)
		}
		return err
	}
	if err := w.d.Remove(r.externalSnapshot); err != nil {
		return err
	}

	if err := r.run("mutation run --json " + file); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("outside snapshot run: %s", w.report())
	}
	data, err := w.d.Read(r.externalSnapshot)
	if err != nil {
		return err
	}
	r.held = map[string]string{r.externalSnapshot: string(data)}
	return nil
}
func (r *resultMutation) noOrphan() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	if _, ok := obj["orphans"]; ok {
		return fmt.Errorf("unexpected orphans %v", obj["orphans"])
	}
	if strings.Contains(r.s.m.w.stdout, "orphan ") {
		return fmt.Errorf("plain output names orphan: %s", r.s.m.w.stdout)
	}
	return nil
}
func (r *resultMutation) outsideKept() error {
	if err := r.noOrphan(); err != nil {
		return err
	}
	data, err := r.s.m.w.d.Read(r.externalSnapshot)
	if err != nil {
		return err
	}
	if string(data) != r.held[r.externalSnapshot] {
		return fmt.Errorf("outside snapshot changed")
	}
	return nil
}
func (r *resultMutation) renameOld(commit bool) error {
	if err := r.strong(); err != nil {
		return err
	}
	w := r.s.m.w
	if err := w.write("src/old.ts", "export function old(i: number): boolean {\n  return i > 5;\n}\n"); err != nil {
		return err
	}
	if err := w.write(".metrics/mutate/src/old.ts.json", resultOrphan("src/old.ts")); err != nil {
		return err
	}
	if err := r.s.commit("old source"); err != nil {
		return err
	}
	if err := w.d.Mkdir(filepath.Join(w.dir, "lib")); err != nil {
		return err
	}
	if err := w.git("mv", "src/old.ts", "lib/new.ts"); err != nil {
		return err
	}
	if commit {
		return r.s.commit("rename old")
	}
	return nil
}
func (r *resultMutation) oldKept() error {
	if !r.s.m.w.d.Exists(filepath.Join(r.s.m.w.dir, ".metrics/mutate/src/old.ts.json")) {
		return fmt.Errorf("old snapshot removed before new path was selected")
	}
	return r.noOrphan()
}
func (r *resultMutation) oldMoved() error {
	d, w := r.s.m.w.d, r.s.m.w
	if d.Exists(filepath.Join(w.dir, ".metrics/mutate/src/old.ts.json")) {
		return fmt.Errorf("old snapshot remains after rename")
	}
	data, err := d.Read(filepath.Join(w.dir, ".metrics/mutate/lib/new.ts.json"))
	if err != nil {
		return err
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap["file"] != "lib/new.ts" || len(snapshotEntries(snap)) != 1 {
		return fmt.Errorf("moved snapshot %v", snap)
	}
	return nil
}
func (r *resultMutation) freshFile() error {
	if err := r.s.project(false); err != nil {
		return err
	}
	r.file = r.s.file
	return r.run("mutation run --json " + r.file)
}
func (r *resultMutation) reusedBaseline() error {
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if f["baseline"] != "not-run" || f["ran"] != float64(0) || f["reused"] != float64(2) || len(entries) != 2 || len(baselines(r.s.m.w.stderr)) != 0 || len(trialsOf(r.s.m.w.stderr, r.file)) != 0 {
		return fmt.Errorf("reused baseline %v", f)
	}
	for _, mu := range entries {
		if mu["reused"] != true || mu["outcome"] != "killed" {
			return fmt.Errorf("reused mutant %v", mu)
		}
	}
	return nil
}
func (r *resultMutation) changedBaseline() error {
	if err := r.s.replace(r.file, "// past five", "// past five, edited"); err != nil {
		return err
	}
	if err := r.run("mutation run --json " + r.file); err != nil {
		return err
	}
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	if f["baseline"] != "passed" || f["ran"] != float64(1) || f["reused"] != float64(1) || len(baselines(r.s.m.w.stderr)) != 1 {
		return fmt.Errorf("edited file baseline %v", f)
	}
	return nil
}
func (r *resultMutation) leftOut() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	functions := resultCheckFunctions(obj, r.file)
	n := 0
	for _, fn := range functions {
		left, present := fn["left_out"]
		if sinceFunction(listedSite{function: fmt.Sprint(fn["function"])}, r.s.place) {
			n++
			if left != float64(1) {
				return fmt.Errorf("left out %v", fn)
			}
		} else if present {
			return fmt.Errorf("unaffected function has left_out: %v", fn)
		}
	}
	if n != 1 {
		return fmt.Errorf("place functions %v", functions)
	}
	return nil
}
func (r *resultMutation) leftOutLine() error {
	sites, err := r.s.functionSites(r.s.place)
	if err != nil {
		return err
	}
	if len(sites) != 1 {
		return fmt.Errorf("place sites %v", sites)
	}
	w := r.s.m.w
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "check", r.file)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("  left out %s:2 in %s: 1 recorded mutant at sites gone, which the next mutation run drops\n", r.file, sites[0].function)
	if code != 0 || !strings.Contains(out, want) || strings.Count(out, "left out ") != 1 {
		return fmt.Errorf("left-out note exit%d: %s%s; want %s", code, out, stderr, want)
	}
	return nil
}

func resultCheckFunctions(obj map[string]any, file string) []map[string]any {
	var out []map[string]any
	files, _ := obj["files"].([]any)
	for _, raw := range files {
		f := raw.(map[string]any)
		if filepath.ToSlash(fmt.Sprint(f["file"])) != file {
			continue
		}
		functions, _ := f["functions"].([]any)
		for _, fn := range functions {
			out = append(out, fn.(map[string]any))
		}
	}
	return out
}
