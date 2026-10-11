package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
)

// The scenario outline of "Rule: Strict runs prove executable line coverage
// in every language" in features/mutate.feature that the
// strict-scope-refusals slice holds. No language tool runs: each project's
// judged function has no mutation site, and its strict line evidence is
// seeded fresh, bound to the fingerprints its language computes now, so a
// strict run reuses it and runs nothing, and a strict check passes on it
// (each example's plain project, without the scope, shows that). The
// projects live in root/, beside root/../outside/lib, which their scopes
// name and which itos-cc must never read: its files' and its directory's
// access times, set two days back, must stay as they are, where the file
// system keeps access times.

// scopeExample is one example of the outline: its project's files, the
// judged source, the files that give it the scope, those that replace them
// in its plain twin, the marker the outside directory holds, and the words
// a refusal names the scope by.
type scopeExample struct {
	language, scope string
	files           map[string]string
	source          string
	scoped, plain   map[string]string
	marker          string
	names           []string
}

var scopeExamples = []scopeExample{
	{
		language: "typescript", scope: "an npm workspace",
		files: map[string]string{
			"packages/a/package.json":  `{"name": "a"}` + "\n",
			"packages/a/src/label.ts":  "export function label(name: string): string {\n  return name;\n}\n",
			"packages/b/package.json":  `{"name": "b"}` + "\n",
			"packages/b/src/unused.ts": "export const unused = 1;\n",
		},
		source: "packages/a/src/label.ts",
		scoped: map[string]string{"package.json": `{"name": "ws", "private": true, "workspaces": ["packages/*"]}` + "\n"},
		plain:  map[string]string{"package.json": `{"name": "ws", "private": true}` + "\n"},
		marker: "package.json", names: []string{"workspace", "package.json"},
	},
	{
		language: "typescript", scope: "a file: dependency outside the root",
		files:  map[string]string{"src/label.ts": "export function label(name: string): string {\n  return name;\n}\n"},
		source: "src/label.ts",
		scoped: map[string]string{"package.json": `{"name": "app", "dependencies": {"lib": "file:../outside/lib"}}` + "\n"},
		plain:  map[string]string{"package.json": `{"name": "app"}` + "\n"},
		marker: "package.json", names: []string{"file:../outside/lib"},
	},
	{
		language: "python", scope: "an editable dependency outside the root",
		files:  map[string]string{"label.py": "def label(name):\n    return name\n"},
		source: "label.py",
		scoped: map[string]string{"pyproject.toml": "[project]\nname = \"app\"\nversion = \"0\"\n\n[tool.uv.sources]\nlib = { path = \"../outside/lib\", editable = true }\n"},
		plain:  map[string]string{"pyproject.toml": "[project]\nname = \"app\"\nversion = \"0\"\n"},
		marker: "pyproject.toml", names: []string{"../outside/lib", "pyproject.toml"},
	},
	{
		language: "kotlin", scope: "a Gradle includeBuild outside the root",
		files: map[string]string{
			"build.gradle.kts":         "",
			"src/main/kotlin/Label.kt": "fun label(name: String): String {\n    return name\n}\n",
		},
		source: "src/main/kotlin/Label.kt",
		scoped: map[string]string{"settings.gradle.kts": "rootProject.name = \"app\"\nincludeBuild(\"../outside/lib\")\n"},
		plain:  map[string]string{"settings.gradle.kts": "rootProject.name = \"app\"\n"},
		marker: "settings.gradle.kts", names: []string{"includeBuild", "../outside/lib"},
	},
	{
		language: "kotlin", scope: "a Maven module outside the root",
		files: map[string]string{
			"app/pom.xml":                  "<project><artifactId>app</artifactId></project>\n",
			"app/src/main/kotlin/Label.kt": "fun label(name: String): String {\n    return name\n}\n",
		},
		source: "app/src/main/kotlin/Label.kt",
		scoped: map[string]string{"pom.xml": "<project><artifactId>top</artifactId><packaging>pom</packaging>\n" +
			"  <modules><module>app</module><module>../outside/lib</module></modules>\n</project>\n"},
		plain: map[string]string{"pom.xml": "<project><artifactId>top</artifactId><packaging>pom</packaging>\n" +
			"  <modules><module>app</module></modules>\n</project>\n"},
		marker: "pom.xml", names: []string{"module", "../outside/lib"},
	},
}

// seedFreshLineEvidence writes, for each function of the file at rel under
// root, complete strict line evidence with no executable line, bound to its
// hash and to the producer and fingerprints its language computes now, as
// a strict run that measured it would have recorded it.
func seedFreshLineEvidence(t *testing.T, root, rel string) {
	t.Helper()
	source := filepath.Join(root, filepath.FromSlash(rel))
	f, err := lang.ParseFile(source)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var producer string
	var inputs map[string]string
	switch f.Spec.Name {
	case "typescript":
		producer = mutate.TypeScriptCoverageProducer(coverage.TypeScriptRunner(lang.FindUp(source, "package.json"), true), false)
		inputs, err = mutate.TypeScriptCoverageInputs(source, root, producer, nil)
	case "python":
		producer = mutate.CoverageProducer("python", false)
		inputs, err = mutate.PythonCoverageInputs(source, root, producer, nil)
	case "kotlin":
		module := lang.FindUp(source, "build.gradle.kts", "build.gradle", "pom.xml")
		producer = mutate.KotlinCoverageProducer(coverage.KotlinRunner(module), false)
		inputs, err = mutate.KotlinCoverageInputs(source, root, producer, nil)
	default:
		t.Fatalf("no strict line evidence for %s", f.Spec.Name)
	}
	if err != nil {
		t.Fatal(err)
	}
	snap := mutate.Snapshot{Version: 1, File: rel, Language: f.Spec.Name, Units: []mutate.UnitResult{}}
	for _, unit := range f.Units {
		hash := mutate.UnitHash(f, unit)
		snap.Units = append(snap.Units, mutate.UnitResult{
			Namespace: unit.Namespace, Name: unit.Name, Hash: hash, StartLine: unit.StartLine, EndLine: unit.EndLine,
			Mutants: []mutate.Mutant{},
			Coverage: &mutate.CoverageEvidence{Version: 1, Language: f.Spec.Name, File: rel,
				Function: unit.Namespace + "#" + unit.Name, Hash: hash, Producer: producer, Inputs: maps.Clone(inputs),
				Complete: true, Blocks: []mutate.CoverageBlock{}},
		})
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".metrics", filepath.FromSlash(mutate.SnapshotName(rel))), string(data)+"\n")
}

// accessTime is the time the file at path was last read, as the file
// system keeps it, and false where its status names none this test knows.
func accessTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	v := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if v.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	for _, name := range []string{"Atim", "Atimespec"} {
		if f := v.FieldByName(name); f.IsValid() && f.Kind() == reflect.Struct {
			sec, nsec := f.FieldByName("Sec"), f.FieldByName("Nsec")
			if sec.IsValid() && nsec.IsValid() {
				return time.Unix(sec.Int(), nsec.Int()), true
			}
		}
	}
	return time.Time{}, false
}

// strictCommands are the two strict commands of the outline, for source.
func strictCommands(source string) map[string][]string {
	return map[string][]string{
		"check": {"mutation", "check", "--fail-uncovered", "--json", source},
		"run":   {"mutation", "run", "--fail-uncovered", "--json", "--no-annotate", "--workers", "1", source},
	}
}

// @ID-MUT-224
func TestStrictCoverageRefusesScopesItsFingerprintsCannotProve(t *testing.T) {
	t.Parallel()
	for _, example := range scopeExamples {
		t.Run(example.language+"/"+example.scope, func(t *testing.T) {
			t.Parallel()
			source := filepath.FromSlash(example.source)
			// Its plain twin, without the scope, seeded the same way: the
			// seeded evidence is fresh, and the strict commands pass on it.
			plain := t.TempDir()
			for name, text := range example.files {
				writeFile(t, filepath.Join(plain, filepath.FromSlash(name)), text)
			}
			for name, text := range example.plain {
				writeFile(t, filepath.Join(plain, filepath.FromSlash(name)), text)
			}
			seedFreshLineEvidence(t, plain, example.source)
			for _, name := range []string{"check", "run"} {
				useDir(t, plain)
				o := cli(t, strictCommands(source)[name]...)
				if got := strictLineFindings(t, o); len(got) != 0 || o.code != 0 {
					t.Fatalf("the plain project's strict %s: exit %d %q, want a pass on fresh evidence\n%s%s", name, o.code, got, o.stdout, o.stderr)
				}
			}

			// Given a <language> project with <scope>, and fresh line
			// coverage evidence for its judged functions
			parent := t.TempDir()
			root := filepath.Join(parent, "root")
			for name, text := range example.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(name)), text)
			}
			for name, text := range example.scoped {
				writeFile(t, filepath.Join(root, filepath.FromSlash(name)), text)
			}
			seedFreshLineEvidence(t, root, example.source)
			outside := filepath.Join(parent, "outside", "lib")
			marker := filepath.Join(outside, example.marker)
			writeFile(t, marker, "")
			past := time.Now().Add(-48 * time.Hour)
			for _, path := range []string{marker, outside} {
				if err := os.Chtimes(path, past, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			markerRead, markerKept := accessTime(marker)
			dirRead, _ := accessTime(outside)

			// When I run "itos-cc mutation check --fail-uncovered --json"
			// and "itos-cc mutation run --fail-uncovered --json"
			useDir(t, root)
			for _, name := range []string{"check", "run"} {
				o := cli(t, strictCommands(source)[name]...)
				// Then both report "mutation.coverage-unsupported" for its
				// judged functions, naming the scope, and fail
				want := fmt.Sprintf("mutation.coverage-unsupported %s %s:%d", example.source, scopeFunction(example), 1)
				if got := strictLineFindings(t, o); len(got) != 1 || got[0] != want {
					t.Errorf("strict %s reports %q, want [%s]\n%s%s", name, got, want, o.stdout, o.stderr)
				}
				for _, p := range o.json(t).Problems {
					msg, _ := p["message"].(string)
					if p["rule"] != "mutation.coverage-unsupported" {
						continue
					}
					for _, word := range example.names {
						if !strings.Contains(msg, word) {
							t.Errorf("strict %s's refusal %q does not name the scope by %q", name, msg, word)
						}
					}
				}
				if o.code != 1 {
					t.Errorf("strict %s exit %d, want 1\n%s%s", name, o.code, o.stdout, o.stderr)
				}
			}

			// And itos-cc reads nothing outside the project root
			if markerKept {
				if now, _ := accessTime(marker); !now.Equal(markerRead) {
					t.Errorf("the strict commands read %s, outside the project root", marker)
				}
				if now, _ := accessTime(outside); !now.Equal(dirRead) {
					t.Errorf("the strict commands listed %s, outside the project root", outside)
				}
			} else {
				t.Log("this file system names no access time, so reads outside the root go unseen")
			}

			// But without --fail-uncovered both report as before: the
			// function has no mutation site, so both pass.
			for _, args := range [][]string{
				{"mutation", "check", "--json", source},
				{"mutation", "run", "--json", "--no-annotate", "--workers", "1", source},
			} {
				o := cli(t, args...)
				if rules := problemRules(t, o); len(rules) != 0 || o.code != 0 {
					t.Errorf("%s without --fail-uncovered: exit %d %q, want a pass as before\n%s%s", args[1], o.code, rules, o.stdout, o.stderr)
				}
			}
		})
	}
}

// scopeFunction is the namespace#name of example's judged function.
func scopeFunction(example scopeExample) string {
	base := strings.TrimSuffix(filepath.Base(example.source), filepath.Ext(example.source))
	if example.language == "kotlin" {
		return "#label"
	}
	return base + "#label"
}
