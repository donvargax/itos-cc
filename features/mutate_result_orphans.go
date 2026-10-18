package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// The late-result steps about snapshots whose source is gone or was renamed:
// what a run reports, keeps and drops of them.

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
	if filepath.ToSlash(fmt.Sprint(orphan["file"])) != "src/old.ts" || orphan["snapshot"] != ".metrics/mutate/src/old.ts.json" || orphan["why"] != "gone" || !strings.Contains(r.s.m.w.stdout, ".metrics/mutate/src/old.ts.json") {
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
