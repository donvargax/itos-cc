package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (l *languageMutation) pythonCacheSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Python project whose test kills a mutant that replaces "==" with "!="$`, func(ctx context.Context) error { return l.pythonCache(ctx, true) })
	sc.Step(`^the baseline and the mutant are written within the same second$`, l.pinnedPython)
	sc.Step(`^the mutant is killed(?:, not survived)?$`, l.pythonKilled)
	sc.Step(`^no __pycache__ directory is written beside the worker copy's sources$`, l.noBytecode)
	sc.Step(`^the project's own tree, its __pycache__ included, is left unchanged$`, l.pythonTreeKept)
	sc.Step(`^a Python project with a pytest test that kills its mutant and no \.pytest_cache directory$`, func(ctx context.Context) error { return l.pythonCache(ctx, false) })
	sc.Step(`^no \.pytest_cache directory is written in the project's tree or beside the worker copy's sources$`, l.noPytestCache)
	sc.Step(`^a run with --test-command runs that command exactly as given$`, l.customPytest)
}

const languagePythonSource = "def low(i):\n    return i == 3\n"

func (l *languageMutation) pythonCache(ctx context.Context, bytecode bool) error {
	w := l.m.w
	l.file = "board.py"
	l.language = "python"
	dir, err := w.d.Folder()
	if err != nil {
		return err
	}
	l.trace = filepath.Join(dir, "runs.jsonl")
	root, _ := json.Marshal(w.dir)
	trace, _ := json.Marshal(l.trace)
	project := "[project]\nname = \"board\"\nversion = \"0.0.0\"\n"
	if bytecode {
		project += "\n[tool.pytest.ini_options]\naddopts = \"-p no:cacheprovider\"\n"
	}
	conftest := "import os, pathlib, hashlib, json\nHERE = pathlib.Path(__file__).resolve().parent\nORIGINAL = pathlib.Path(" + string(root) + ").resolve()\nSOURCE = HERE / 'board.py'\n"
	if bytecode {
		conftest += "os.utime(SOURCE, (1700000000, 1700000000))\n"
	}
	conftest += "def pytest_unconfigure(config):\n    data = SOURCE.read_bytes()\n    record = {'cwd': str(HERE), 'original': str(ORIGINAL), 'mtime_ns': SOURCE.stat().st_mtime_ns, 'size': len(data), 'sha': hashlib.sha256(data).hexdigest(), 'bytecode': (HERE / '__pycache__').exists(), 'pytest': (HERE / '.pytest_cache').exists()}\n    with open(" + string(trace) + ", 'a') as f: f.write(json.dumps(record) + '\\n')\n"
	for name, text := range map[string]string{"pyproject.toml": project, "board.py": languagePythonSource, "test_board.py": "from board import low\n\ndef test_low():\n    assert low(3)\n    assert not low(4)\n", "conftest.py": conftest} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	if err := w.d.Coverage(w.dir, Coverage{Language: "python", Report: "SF:board.py\nDA:1,1\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider test_board.py"}); err != nil {
		return err
	}
	if bytecode {
		a, _, code, err := l.capture(`crap --json --coverage-command 'python3 -m coverage run -m pytest -q && python3 -m coverage lcov -o oracle.lcov && python3 -m compileall -q .' --coverage-report oracle.lcov board.py`)
		if err != nil {
			return err
		}
		if code != 0 || a["ok"] != true {
			return fmt.Errorf("original bytecode coverage setup: exit%d %v", code, a)
		}
		names, err := w.d.Files(ctx, w.dir)
		if err != nil {
			return err
		}
		found := false
		for _, name := range names {
			if strings.HasPrefix(name, "__pycache__/board.") && strings.HasSuffix(name, ".pyc") {
				b, err := w.d.Read(filepath.Join(w.dir, filepath.FromSlash(name)))
				if err != nil {
					return err
				}
				found = len(b) > 16
			}
		}
		if !found {
			return fmt.Errorf("public compileall produced no board bytecode: %v", names)
		}
	}
	if err := w.d.Write(l.trace, nil); err != nil {
		return err
	}
	l.before, err = l.files(ctx)
	return err
}
func (l *languageMutation) pinnedPython() error {
	b := l.before["conftest.py"]
	if !strings.Contains(b, "os.utime(SOURCE, (1700000000, 1700000000))") {
		return fmt.Errorf("fixture does not pin source mtime")
	}
	return nil
}
func (l *languageMutation) pythonKilled() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["baseline"] != "passed" || f["ran"] != float64(1) || f["killed"] != float64(1) || f["survived"] != float64(0) {
		return fmt.Errorf("the Python actual judgment: %v", f)
	}
	return l.m.w.exitsWith(0)
}
func (l *languageMutation) pythonTrace() ([]map[string]any, error) {
	b, err := l.m.w.d.Read(l.trace)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var a map[string]any
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("missing real Python baseline/trial observations: %s", b)
	}
	return out, nil
}
func (l *languageMutation) noBytecode() error {
	if err := l.pythonKilled(); err != nil {
		return err
	}
	records, err := l.pythonTrace()
	if err != nil {
		return err
	}
	base := sha256.Sum256([]byte(languagePythonSource))
	trial := sha256.Sum256([]byte(strings.Replace(languagePythonSource, "==", "!=", 1)))
	seen := map[string]int{}
	for _, r := range records {
		if r["cwd"] == r["original"] {
			continue
		}
		if r["bytecode"] != false || r["mtime_ns"] != float64(1700000000000000000) || r["size"] != float64(len(languagePythonSource)) {
			return fmt.Errorf("worker bytecode/mtime/size: %v", r)
		}
		seen[fmt.Sprint(r["sha"])]++
	}
	if seen[hex.EncodeToString(base[:])] < 1 || seen[hex.EncodeToString(trial[:])] < 1 {
		return fmt.Errorf("actual equal-size baseline/mutant hashes not observed: %v", records)
	}
	return nil
}
func (l *languageMutation) pythonTreeKept(ctx context.Context) error {
	after, err := l.files(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(l.before, after) {
		return fmt.Errorf("original tree/cache changed: before%v after%v", slicesOfKeys(l.before), slicesOfKeys(after))
	}
	return nil
}
func slicesOfKeys(a map[string]string) []string {
	var out []string
	for k := range a {
		out = append(out, k)
	}
	return out
}
func (l *languageMutation) noPytestCache() error {
	if err := l.pythonKilled(); err != nil {
		return err
	}
	records, err := l.pythonTrace()
	if err != nil {
		return err
	}
	original, workers := 0, 0
	for _, r := range records {
		if r["cwd"] == r["original"] {
			original++
		} else {
			workers++
		}
		if r["pytest"] != false {
			return fmt.Errorf("default pytest cache: %v", r)
		}
	}
	if original < 1 || workers < 2 || l.m.w.d.Exists(filepath.Join(l.m.w.dir, ".pytest_cache")) {
		return fmt.Errorf("missing original coverage/worker observations: %v", records)
	}
	return nil
}
func (l *languageMutation) customPytest() error {
	w := l.m.w
	if err := w.d.Write(l.trace, nil); err != nil {
		return err
	}
	command := "python3 -m pytest -q"
	if err := w.d.Tests(w.dir, Tests{Language: "python", Command: command}); err != nil {
		return err
	}
	a, log, code, err := l.capture("mutation run --json --no-coverage --mutate-all --test-command " + strconv.Quote(command) + " " + l.file)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if code != 0 || f["killed"] != float64(1) || !strings.Contains(log, "$ "+command+"\n") {
		return fmt.Errorf("literal custom pytest command: exit%d %v\n%s", code, f, log)
	}
	records, err := l.pythonTrace()
	if err != nil {
		return err
	}
	found := false
	for _, r := range records {
		if r["cwd"] != r["original"] && r["pytest"] == true {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("custom pytest command did not write worker cache: %v", records)
	}
	return nil
}
