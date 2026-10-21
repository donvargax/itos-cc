package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) refusalSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a (typescript|python|kotlin|dart) project with (.+), and fresh line coverage evidence for its judged functions$`, q.refusalProject)
	sc.Step(`^I run "itos-cc mutation check --fail-uncovered --json" and "itos-cc mutation run --fail-uncovered --json"$`, func() error {
		q.scopeRuns = nil
		for _, cmd := range []string{"check", "run"} {
			a, log, code, err := q.capture("mutation " + cmd + " --fail-uncovered --json " + q.file)
			if err != nil {
				return err
			}
			q.scopeRuns = append(q.scopeRuns, strictFlagObservation{flag: cmd, object: a, log: log, code: code})
			if cmd == "check" {
				q.scopeCheckWrites, err = q.observe()
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	sc.Step(`^both report "mutation.coverage-unsupported" for its judged functions, naming the scope, and fail$`, func() error {
		if len(q.scopeCheckWrites) != 0 {
			return fmt.Errorf("strict scoped check wrote %v", q.scopeCheckWrites)
		}
		if len(q.scopeRuns) != 2 {
			return fmt.Errorf("missing both strict observations")
		}
		for _, r := range q.scopeRuns {
			ps := q.findings(r.object, "mutation.coverage-unsupported", "label")
			if r.code != 1 || len(ps) != 1 || len(checkProblems(r.object)) != 1 || filepath.ToSlash(fmt.Sprint(ps[0]["file"])) != q.file || ps[0]["function"] != q.namespace+"#label" {
				return fmt.Errorf("scope refusal %s exit%d: %v", r.flag, r.code, r.object)
			}
			for _, token := range q.scopeNames {
				if !strings.Contains(strings.ToLower(fmt.Sprint(ps[0]["message"])), strings.ToLower(token)) {
					return fmt.Errorf("scope refusal does not name %s: %v", token, ps[0])
				}
			}
			if err := strictNoCommands(r.log); err != nil {
				return err
			}
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(snap, q.saved) {
			return fmt.Errorf("refusal refreshed seeded evidence: %v vs%v", snap, q.saved)
		}
		return nil
	})
	sc.Step(`^itos-cc reads nothing outside the project root$`, func() error {
		if q.reads == nil {
			return fmt.Errorf("missing outside read observer")
		}
		read, err := q.reads()
		if err != nil {
			return err
		}
		if len(read) > 0 {
			return fmt.Errorf("outside reads: %v", read)
		}
		return nil
	})
	sc.Step(`^without --fail-uncovered both report as before$`, func() error {
		for _, cmd := range []string{"check", "run"} {
			a, _, code, err := q.capture("mutation " + cmd + " --json " + q.file)
			if err != nil {
				return err
			}
			before := q.plain
			if cmd == "check" {
				before = q.plainCheck
			}
			if code != 0 || !slices.Equal(strictProblemRecords(a), strictProblemRecords(before)) || !reflect.DeepEqual(strictCounts(a), strictCounts(before)) {
				return fmt.Errorf("non-strict %s changed exit%d: %v vs%v", cmd, code, a, before)
			}
		}
		return nil
	})
}
func (q *strictMutation) refusalProject(language, scope string) error {
	w := q.m.w
	// Keep all outside fixtures beneath this scenario's owned parent. Both the
	// ordinary and adversarial layouts use exactly the same project path.
	parent := w.dir
	q.outside = append(q.outside, parent)
	w.dir = filepath.Join(parent, "project")
	if err := w.d.Mkdir(w.dir); err != nil {
		return err
	}
	q.language = language
	files := map[string]string{}
	plain, scoped := map[string]string{}, map[string]string{}
	c := Coverage{Language: language}
	t := Tests{Language: language}
	measureDir := w.dir
	inputSource := ""
	marker := "package.json"
	switch scope {
	case "an npm workspace", "a file: dependency outside the root":
		pkg := `{"name":"app","private":true,"type":"module","devDependencies":{"vitest":"5.0.2","@vitest/coverage-v8":"5.0.2"}}` + "\n"
		q.file = "src/label.ts"
		q.testFile = "src/label.test.ts"
		prefix := ""
		if scope == "an npm workspace" {
			prefix = "packages/a/"
			measureDir = filepath.Join(w.dir, "packages", "a")
			q.file = prefix + q.file
			q.testFile = prefix + q.testFile
			files[prefix+"package.json"] = pkg
			plain["package.json"] = `{"name":"ws","private":true}` + "\n"
			scoped["package.json"] = `{"name":"ws","private":true,"workspaces":["packages/a","../outside/lib"]}` + "\n"
			q.scopeNames = []string{"workspace", "package.json"}
		} else {
			plain["package.json"] = pkg
			var obj map[string]any
			if err := json.Unmarshal([]byte(pkg), &obj); err != nil {
				return err
			}
			obj["dependencies"] = map[string]string{"lib": "file:../outside/lib"}
			b, err := json.Marshal(obj)
			if err != nil {
				return err
			}
			scoped["package.json"] = string(b) + "\n"
			q.scopeNames = []string{"file:../outside/lib"}
		}
		files[q.file] = "export function label(name: string): string {\n  return name;\n}\n"
		files[q.testFile] = "import {expect,test} from 'vitest';\nimport {label} from './label';\ntest('label',()=>expect(label('value')).toBe('value'));\n"
		c.Report = "SF:src/label.ts\nDA:2,1\nend_of_record\n"
		t.Command = vitestCommand + "src/label.ts"
	case "an editable dependency outside the root":
		marker = "pyproject.toml"
		q.file = "label.py"
		q.testFile = "test_label.py"
		plain[marker] = "[project]\nname = \"app\"\nversion = \"0\"\n"
		scoped[marker] = plain[marker] + "\n[tool.uv.sources]\nlib = { path = \"../outside/lib\", editable = true }\n"
		files[q.file] = "def label(name):\n    return name\n"
		files[q.testFile] = "from label import label\ndef test_label():\n    assert label('value') == 'value'\n"
		q.scopeNames = []string{"../outside/lib", "pyproject.toml"}
		w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
		c.Report = "SF:label.py\nDA:1,1\nDA:2,1\nend_of_record\n"
		t.Command = "python3 -m pytest -q -x -p no:cacheprovider test_label.py"
	case "a Gradle includeBuild outside the root":
		marker = "settings.gradle.kts"
		q.file = "src/main/kotlin/Label.kt"
		q.testFile = "src/test/kotlin/LabelTest.kt"
		for file, text := range languageKotlinBuilds["gradle"] {
			files[file] = text
		}
		plain[marker] = "rootProject.name = \"app\"\n"
		scoped[marker] = plain[marker] + "includeBuild(\"../outside/lib\")\n"
		files[q.file] = "fun label(name: String): String {\n    return name\n}\n"
		files[q.testFile] = "import kotlin.test.*\nclass LabelTest { @Test fun reaches(){assertEquals(\"value\",label(\"value\"))} }\n"
		q.scopeNames = []string{"includeBuild", "../outside/lib"}
		c.Program = "gradle"
		c.Report = `<report><package name=""><sourcefile name="Label.kt"><line nr="2" mi="0" ci="1"/></sourcefile></package></report>`
		t.Command = "gradle -p . test --fail-fast --tests LabelTest"
	case "a Maven module outside the root":
		marker = "pom.xml"
		q.file = "app/src/main/kotlin/Label.kt"
		q.testFile = "app/src/test/kotlin/LabelTest.kt"
		build := languageKotlinBuilds["maven"]["pom.xml"]
		plain["pom.xml"] = strings.Replace(build, "<artifactId>own</artifactId>", "<artifactId>top</artifactId>\n<packaging>pom</packaging>\n<modules><module>app</module></modules>", 1)
		scoped["pom.xml"] = strings.Replace(plain["pom.xml"], "</modules>", "<module>../outside/lib</module></modules>", 1)
		files["app/pom.xml"] = strings.Replace(build, "<artifactId>own</artifactId>", "<artifactId>app</artifactId>", 1)
		files[q.file] = "fun label(name: String): String {\n    return name\n}\n"
		files[q.testFile] = "import kotlin.test.*\nclass LabelTest { @Test fun reaches(){assertEquals(\"value\",label(\"value\"))} }\n"
		q.scopeNames = []string{"module", "../outside/lib"}
		c.Program = "mvn"
		c.Report = `<report><package name=""><sourcefile name="Label.kt"><line nr="2" mi="0" ci="1"/></sourcefile></package></report>`
		t.Command = "mvn -q test -Dtest=LabelTest -Dsurefire.failIfNoSpecifiedTests=false"
		// Maven measures and tests the module where its own pom.xml is.
		measureDir = filepath.Join(w.dir, "app")
	case "a path dependency outside the root", "a pubspec_overrides.yaml path override outside the root":
		// The plain package has an overrides file that overrides nothing,
		// so the adversarial cache holds its hash, as it holds pubspec.yaml's.
		marker = "pubspec.yaml"
		q.file = "lib/label.dart"
		q.testFile = "test/label_test.dart"
		plain[marker] = coverageDartFlutterPubspec("app", false)
		plain["pubspec_overrides.yaml"] = "dependency_overrides: {}\n"
		if scope == "a path dependency outside the root" {
			scoped[marker] = strings.Replace(plain[marker], "dev_dependencies:", "  lib:\n    path: ../outside/lib\ndev_dependencies:", 1)
			q.scopeNames = []string{"../outside/lib", "pubspec.yaml"}
		} else {
			scoped["pubspec_overrides.yaml"] = "dependency_overrides:\n  lib:\n    path: ../outside/lib\n"
			q.scopeNames = []string{"../outside/lib", "pubspec_overrides.yaml"}
		}
		files[q.file] = "String label(String name) {\n  return name;\n}\n"
		files[q.testFile] = "import 'package:flutter_test/flutter_test.dart';\nimport 'package:app/label.dart';\n\n" +
			"void main() {\n  test('label', () {\n    expect(label('value'), 'value');\n  });\n}\n"
		c.Report = "SF:lib/label.dart\nDA:1,1\nDA:2,1\nend_of_record\n"
		t.Command = strictDartFlutterTest + q.testFile
	default:
		return fmt.Errorf("unknown unsupported scope %s", scope)
	}
	inputSource = q.file
	// The test imports, or in Kotlin references, the judged file.
	q.m.w.imports(map[string][]string{q.testFile: {q.file}})
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	for file, text := range plain {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(measureDir, c); err != nil {
		return err
	}
	if err := w.d.Tests(measureDir, t); err != nil {
		return err
	}
	if language == "python" {
		if err := q.m.pythonProbe(true); err != nil {
			return err
		}
	}
	a, log, code, err := q.capture("mutation run --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("supported plain strict setup exit%d: %v\n%s", code, a, log)
	}
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	units := objectList(snap, "units")
	if len(units) != 1 || units[0]["name"] != "label" {
		return fmt.Errorf("plain judged population: %v", snap)
	}
	q.namespace, _ = units[0]["namespace"].(string)
	hash, hashOK := units[0]["hash"].(string)
	e, ok := units[0]["line_coverage"].(map[string]any)
	producer, producerOK := e["producer"].(string)
	if !hashOK || hash == "" || !producerOK || producer == "" {
		return fmt.Errorf("missing actual plain function hash/producer: %v", units[0])
	}
	if !ok || e["complete"] != true || e["function_hash"] != units[0]["hash"] || len(objectList(e, "blocks")) == 0 {
		return fmt.Errorf("plain nonempty provenance: %v", units[0])
	}
	inputs, ok := e["inputs"].(map[string]any)
	if !ok || len(inputs) == 0 || inputs["@producer"] == nil {
		return fmt.Errorf("plain full input provenance missing: %v", e)
	}
	source, err := w.d.Read(filepath.Join(w.dir, filepath.FromSlash(q.file)))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(source)
	if inputs[inputSource] != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("plain source input hash %s != %x: %v", inputSource, sum, inputs)
	}
	// Plain positive controls prove the complete observed record is reusable.
	q.plain = a
	if err := q.observed(); err != nil {
		return err
	}
	q.plainCheck, log, code, err = q.capture("mutation check --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("plain strict check exit%d: %v", code, q.plainCheck)
	}
	if err := q.quiet(log); err != nil {
		return err
	}
	_, log, code, err = q.capture("mutation run --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("plain strict cached run exit%d", code)
	}
	if err := strictNoCommands(log); err != nil {
		return err
	}
	cached, err := q.snapshot()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(cached, snap) {
		return fmt.Errorf("cached supported run changed observed evidence: %v vs%v", cached, snap)
	}
	for file, text := range scoped {
		if err := w.write(file, text); err != nil {
			return err
		}
		// Update only an already-observed manifest key. All opaque producer,
		// environment and unchanged input values remain the observed values.
		if _, exists := inputs[file]; exists {
			sum := sha256.Sum256([]byte(text))
			inputs[file] = hex.EncodeToString(sum[:])
		}
	}
	if err := q.writeSnapshot(snap); err != nil {
		return err
	}
	q.saved = snap
	outside := filepath.Join(parent, "outside", "lib")
	if err := w.d.Write(filepath.Join(outside, marker), []byte("owned outside dependency marker\n")); err != nil {
		return err
	}
	q.reads, err = w.d.ObserveReads(outside, filepath.Join(outside, marker))
	if errors.Is(err, ErrReadObservationUnsupported) {
		fmt.Printf("outside read observation omission: %v\n", err)
		return godog.ErrSkip
	}
	if err != nil {
		return err
	}
	return q.observed()
}
