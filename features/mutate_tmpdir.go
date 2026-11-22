package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// The temporary folder each worker's tests see (slice-62, ID-MUT-353 and
// ID-MUT-354): a Go package whose test records, in a file of its own for
// each run (T-213), the folder it ran in and the temporary folder it saw,
// run with a temporary folder of the scenario's own as the system's.

// tempLeft is the name of the file ID-MUT-354's test leaves in its
// temporary folder.
const tempLeft = "left-by-board-test"

// tempRun is what one run of the package's test recorded: the folder it
// ran in, whether that is the real tree's, TMPDIR, TMP and TEMP, Go's
// temporary folder, each with its links resolved, "" when unset, and the
// folder it left tempLeft in, "" for none.
type tempRun struct {
	Cwd    string `json:"cwd"`
	Real   bool   `json:"real"`
	TMPDIR string `json:"TMPDIR"`
	TMP    string `json:"TMP"`
	TEMP   string `json:"TEMP"`
	Go     string `json:"go"`
	Left   string `json:"left"`
}

func (l *languageMutation) tempSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go package whose test records the temporary folder it sees, and TMPDIR, TMP and TEMP, with several mutants$`, func() error { return l.tempProject("") })
	sc.Step(`^a Go package whose test writes a file with a name of its own to its temporary folder and leaves it, with several mutants$`, func() error { return l.tempProject(tempLeft) })
	sc.Step(`^every trial saw TMPDIR, TMP and TEMP name one folder, and Go's temporary folder was it$`, l.tempOneFolder)
	sc.Step(`^the trials of a worker saw the same folder, two workers never the same, and none the system's or inside a worker's copy$`, l.tempPerWorker)
	sc.Step(`^no file of that name is in the system's temporary folder$`, l.tempNothingLeft)
	sc.Step(`^no worker's temporary folder is left$`, l.tempFoldersGone)
}

// tempRecorder is the test of the package at real: before its checks, it
// writes what it saw to a file of its own in records, and, when left is
// not "", leaves a file of that name in its temporary folder, but in the
// real tree, where the coverage run keeps the system's folder.
func tempRecorder(records, real, left, checks string) string {
	return `package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const records, real, left = ` + strconv.Quote(records) + `, ` + strconv.Quote(real) + `, ` + strconv.Quote(left) + `

// resolved is path with its links resolved, "" when it is "".
func resolved(path string) string {
	if path == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return "unresolved " + path
}

func TestAll(t *testing.T) {
	cwd, _ := os.Getwd()
	here := resolved(cwd)
	entry := map[string]any{"cwd": here, "real": here == resolved(real), "go": resolved(os.TempDir())}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		entry[name] = resolved(os.Getenv(name))
	}
	if left != "" && here != resolved(real) {
		if err := os.WriteFile(filepath.Join(os.TempDir(), left), []byte("left\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		entry["left"] = resolved(os.TempDir())
	}
	data, _ := json.Marshal(entry)
	f, err := os.CreateTemp(records, "run-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
` + checks + "}\n"
}

// tempProject writes a module whose src/board.go holds several functions,
// each with mutants its test kills, the test recording each run, and
// leaving a file named left when left is not "". The runs have a folder
// of the scenario's own as the system's temporary folder, named as the
// system names it: TMP and TEMP on Windows, TMPDIR elsewhere, the others
// unset.
func (l *languageMutation) tempProject(left string) error {
	w := l.m.w
	records, err := l.m.testRecords()
	if err != nil {
		return err
	}
	if l.system, err = w.d.Folder(); err != nil {
		return err
	}
	w.folders = append(w.folders, l.system)
	l.language, l.file, l.m.file = "go", "src/board.go", "src/board.go"
	if w.d.Platform() == "windows" {
		w.variables = append(w.variables, "TMPDIR", "TMP="+l.system, "TEMP="+l.system)
	} else {
		w.variables = append(w.variables, "TMPDIR="+l.system, "TMP", "TEMP")
	}
	l.m.forFile = func(args string) error { return l.run(args + " " + filepath.FromSlash(l.file)) }
	var source, checks strings.Builder
	source.WriteString("package board\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&source, "\n// F%d is whether i is past %d.\nfunc F%d(i int) bool {\n\treturn i > %d\n}\n", i, i+2, i, i+2)
		fmt.Fprintf(&checks, "\tif F%d(%d) || !F%d(%d) {\n\t\tt.Fatal(\"F%d\")\n\t}\n", i, i+2, i, i+3, i)
	}
	test := tempRecorder(records, filepath.Join(w.dir, "src"), left, checks.String())
	if err := l.m.writeAll(map[string]string{"go.mod": mutateModule, l.file: source.String(), "src/board_test.go": test}); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "go", Command: goTests.Command})
}

// tempTrials are the runs of the test outside the real tree, in workers'
// copies: a baseline and the mutants', at least two.
func (l *languageMutation) tempTrials(ctx context.Context) ([]tempRun, error) {
	w := l.m.w
	names, err := w.d.Files(ctx, l.m.records)
	if err != nil {
		return nil, err
	}
	var trials []tempRun
	for _, name := range names {
		if !strings.HasPrefix(path.Base(name), "run-") {
			continue
		}
		data, err := w.d.Read(filepath.Join(l.m.records, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		var r tempRun
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("the record %s is no run's: %v", name, err)
		}
		if !r.Real {
			trials = append(trials, r)
		}
	}
	if len(trials) < 2 {
		return nil, fmt.Errorf("the test ran %d times in workers' copies, not a baseline and a mutant's\n%s", len(trials), w.report())
	}
	return trials, nil
}

// tempOneFolder is whether every trial saw TMPDIR, TMP and TEMP set, all
// naming one folder, and Go's temporary folder that one.
func (l *languageMutation) tempOneFolder(ctx context.Context) error {
	trials, err := l.tempTrials(ctx)
	if err != nil {
		return err
	}
	for _, r := range trials {
		if r.TMPDIR == "" || r.TMP != r.TMPDIR || r.TEMP != r.TMPDIR || r.Go != r.TMPDIR {
			return fmt.Errorf("a trial in %s saw TMPDIR %q, TMP %q, TEMP %q and Go's temporary folder %q\n%s", r.Cwd, r.TMPDIR, r.TMP, r.TEMP, r.Go, l.m.w.report())
		}
	}
	return nil
}

// tempPerWorker is whether the trials in one worker's copy all saw one
// temporary folder, the trials of at least two workers ran, no two
// workers saw the same, and none saw the system's or one inside its copy,
// the module's folder above the package's.
func (l *languageMutation) tempPerWorker(ctx context.Context) error {
	trials, err := l.tempTrials(ctx)
	if err != nil {
		return err
	}
	system, err := filepath.EvalSymlinks(l.system)
	if err != nil {
		return err
	}
	byCopy, owner := map[string]string{}, map[string]string{}
	for _, r := range trials {
		copied := filepath.Dir(r.Cwd)
		switch seen, ok := byCopy[r.Cwd]; {
		case ok && seen != r.Go:
			return fmt.Errorf("the trials in %s saw %s and %s\n%s", r.Cwd, seen, r.Go, l.m.w.report())
		case r.Go == system:
			return fmt.Errorf("a trial in %s saw the system's temporary folder %s\n%s", r.Cwd, system, l.m.w.report())
		case r.Go == copied || strings.HasPrefix(r.Go, copied+string(filepath.Separator)):
			return fmt.Errorf("a trial in %s saw %s, inside its copy\n%s", r.Cwd, r.Go, l.m.w.report())
		case owner[r.Go] != "" && owner[r.Go] != r.Cwd:
			return fmt.Errorf("the trials in %s and in %s both saw %s\n%s", owner[r.Go], r.Cwd, r.Go, l.m.w.report())
		}
		byCopy[r.Cwd], owner[r.Go] = r.Go, r.Cwd
	}
	if len(byCopy) < 2 {
		return fmt.Errorf("the test ran in %d workers' copies, not 2: %v\n%s", len(byCopy), trials, l.m.w.report())
	}
	return nil
}

// tempNothingLeft is whether trials left their file and none is in the
// system's temporary folder.
func (l *languageMutation) tempNothingLeft(ctx context.Context) error {
	trials, err := l.tempTrials(ctx)
	if err != nil {
		return err
	}
	for _, r := range trials {
		if r.Left == "" {
			return fmt.Errorf("a trial in %s left no %s\n%s", r.Cwd, tempLeft, l.m.w.report())
		}
	}
	if l.m.w.d.Exists(filepath.Join(l.system, tempLeft)) {
		return fmt.Errorf("%s is in the system's temporary folder %s\n%s", tempLeft, l.system, l.m.w.report())
	}
	return nil
}

// tempFoldersGone is whether no temporary folder a trial left its file in
// is there after the run.
func (l *languageMutation) tempFoldersGone(ctx context.Context) error {
	trials, err := l.tempTrials(ctx)
	if err != nil {
		return err
	}
	for _, r := range trials {
		if r.Left == "" || l.m.w.d.Exists(r.Left) {
			return fmt.Errorf("the temporary folder %q a trial in %s left %s in is still there\n%s", r.Left, r.Cwd, tempLeft, l.m.w.report())
		}
	}
	return nil
}
