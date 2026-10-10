package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario outline of "Rule: Language parity with Go for TypeScript
// coverage scope and counted runs" about counted runs, in
// features/mutate.feature. Each example commits a tiny project with one
// mutation site, `== 3` in low, which its one test kills, and installs its
// dependencies where the project keeps them, untracked and ignored: a
// count-one run then draws that site and judges it in one trial. They use
// the real tools languageTools finds, and skip without them. Counted runs
// are Linux and macOS only.

// countedLanguage is one example of the outline.
type countedLanguage struct {
	name, source string
	files        map[string]string
	// install puts the project's dependencies in place in dir, the live
	// project root, after its commit; it returns the path, relative to the
	// project root, of the executable beneath them that every preparation
	// command must run, where its path tells. The frozen copy is gone once
	// the run ends, so a command may name it in the copy, which lies
	// within the live project, as long as the path ends there.
	install func(t *testing.T, dir string) string
	// tests, when set, is the test file each test runner command of
	// preparation names, its coverage and baseline: the one test that
	// reaches the source, as Python's own tests are.
	tests string
}

var countedLanguages = []countedLanguage{
	{
		name:   "TypeScript",
		source: "src/board.ts",
		files: map[string]string{
			".gitignore": "node_modules\n",
			"package.json": `{"name": "board", "private": true, "type": "module", ` +
				`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
			"src/board.ts": "export function low(i: number): boolean {\n  return i === 3;\n}\n",
			"src/board.test.ts": "import { expect, test } from \"vitest\";\nimport { low } from \"./board\";\n\n" +
				"test(\"low\", () => {\n  expect(low(3)).toBe(true);\n  expect(low(4)).toBe(false);\n});\n",
		},
		// node_modules at the project root
		install: func(t *testing.T, dir string) string {
			modules := languageTools(t, "typescript")
			if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
				t.Skip("symlinks unavailable:", err)
			}
			return filepath.Join("node_modules", ".bin", "vitest")
		},
	},
	{
		name:   "Python",
		source: "board.py",
		files: map[string]string{
			".gitignore":     ".venv\n",
			"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
			"board.py":       "def low(i):\n    return i == 3\n",
			// boardhelp is installed in the project's .venv alone, so a run
			// through any other interpreter fails.
			"test_board.py": "from boardhelp import THREE\n\nfrom board import low\n\n\n" +
				"def test_low():\n    assert low(THREE)\n    assert not low(4)\n",
		},
		// .venv at the project root, over the system's pytest and coverage
		install: func(t *testing.T, dir string) string {
			languageTools(t, "python")
			venv := filepath.Join(dir, ".venv")
			if out, err := exec.Command("python3", "-m", "venv", "--system-site-packages", venv).CombinedOutput(); err != nil {
				t.Fatalf("python3 -m venv: %v\n%s", err, out)
			}
			python := filepath.Join(venv, "bin", "python")
			out, err := exec.Command(python, "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
			if err != nil {
				t.Fatalf("the .venv's site-packages: %v", err)
			}
			writeFile(t, filepath.Join(strings.TrimSpace(string(out)), "boardhelp.py"), "THREE = 3\n")
			if exec.Command("python3", "-c", "import boardhelp").Run() == nil {
				t.Fatal("the system's python3 imports boardhelp too, so it would not tell the .venv's from it")
			}
			// The .venv's python links to the system's, so its path proves
			// nothing; its tests passing through boardhelp do.
			return ""
		},
		tests: "test_board.py",
	},
	{
		name:   "Kotlin",
		source: "src/main/kotlin/board/Board.kt",
		files: map[string]string{
			".gitignore":          ".gradle\nbuild\n",
			"settings.gradle.kts": "rootProject.name = \"board\"\n",
			"build.gradle.kts": "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n    jacoco\n}\n\n" +
				"repositories {\n    mavenCentral()\n}\n\n" +
				"dependencies {\n    testImplementation(kotlin(\"test\"))\n" +
				"    testRuntimeOnly(\"org.junit.platform:junit-platform-launcher\")\n}\n\n" +
				"tasks.test {\n    useJUnitPlatform()\n}\n\n" +
				"tasks.jacocoTestReport {\n    reports {\n        xml.required = true\n    }\n}\n",
			"src/main/kotlin/board/Board.kt": "package board\n\nfun low(i: Int): Boolean {\n    return i == 3\n}\n",
			"src/test/kotlin/board/BoardTest.kt": "package board\n\nimport kotlin.test.Test\nimport kotlin.test.assertFalse\n" +
				"import kotlin.test.assertTrue\n\nclass BoardTest {\n    @Test\n    fun lowIsThree() {\n" +
				"        assertTrue(low(3))\n        assertFalse(low(4))\n    }\n}\n",
		},
		// the Gradle cache: a build of a copy of the project, online, fills
		// it with what the offline run needs
		install: func(t *testing.T, dir string) string {
			languageTools(t, "kotlin")
			warm := t.TempDir()
			if err := os.CopyFS(warm, os.DirFS(dir)); err != nil {
				t.Fatal(err)
			}
			os.RemoveAll(filepath.Join(warm, ".git"))
			if out, err := exec.Command("gradle", "-q", "-p", warm, "test", "jacocoTestReport").CombinedOutput(); err != nil {
				t.Fatalf("warming the Gradle cache: %v\n%s", err, out)
			}
			return ""
		},
	},
}

// commandLines are the lines of stderr that name a command preparation
// ran: its coverage and its baselines.
func commandLines(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		if (strings.HasPrefix(line, "itos-cc: coverage ") || strings.HasPrefix(line, "itos-cc: baseline ")) && strings.Contains(line, "$ ") {
			out = append(out, line)
		}
	}
	return out
}

// liveStatus is git's account of dir's tree, ignored files included.
func liveStatus(t *testing.T, dir string) string {
	t.Helper()
	return gitOut(t, dir, "status", "--porcelain=v1", "--untracked-files=all", "--ignored")
}

// @ID-MUT-209
func TestACountedRunJudgesAProjectOfEachLanguageThroughItsInstalledToolsOffline(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	for _, l := range countedLanguages {
		t.Run(l.name, func(t *testing.T) {
			// Given a committed <language> project with eligible mutation
			// sites and its dependencies installed in <installed>
			dir := t.TempDir()
			for name, text := range l.files {
				writeFile(t, filepath.Join(dir, name), text)
			}
			gitIn(t, dir, "init", "-q")
			gitIn(t, dir, "add", ".")
			gitIn(t, dir, "commit", "-qm", "board")
			tool := l.install(t, dir)
			live, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			useDir(t, dir)
			source := filepath.FromSlash(l.source)
			if sites := scanned(t, source); len(sites) != 1 {
				t.Fatalf("sites of %s: %+v, want the one == 3", source, sites)
			}
			before := liveStatus(t, dir)

			// When a count-one run judges it
			o := countedRun(t, "--count", "1", source)
			c := o.counted(t)
			defer func() {
				if t.Failed() {
					t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
				}
			}()

			// Then preparation measures coverage and runs the clean
			// baseline with the project's installed tools
			for _, name := range []string{"coverage", "baseline"} {
				if s := c.stage(name); s == nil || s["state"] != "complete" {
					t.Errorf("stage %s: %v, want complete; stages %v, problems %v", name, s, c.Stages, c.Problems)
				}
			}
			ran := commandLines(o.stderr)
			if tool != "" {
				for _, line := range ran {
					_, command, _ := strings.Cut(line, "$ ")
					executable, _, _ := strings.Cut(command, " ")
					if !strings.HasPrefix(executable, live+string(filepath.Separator)) ||
						!strings.HasSuffix(executable, string(filepath.Separator)+tool) {
						t.Errorf("%q runs %s, want the project's %s", line, executable, tool)
					}
				}
			}

			if l.tests != "" {
				runners := map[string]int{}
				for _, line := range ran {
					if !strings.Contains(line, " -m pytest") {
						continue
					}
					stage, _, _ := strings.Cut(strings.TrimPrefix(line, "itos-cc: "), " ")
					runners[stage]++
					if !strings.HasSuffix(line, " "+l.tests) {
						t.Errorf("%q runs other tests than %s, the one that reaches %s", line, l.tests, source)
					}
				}
				if runners["coverage"] == 0 || runners["baseline"] == 0 {
					t.Errorf("pytest ran %v times by stage, want for coverage and for the baseline", runners)
				}
			}

			// And exactly one selected mutant is judged with a real outcome
			if len(c.Selected) != 1 || c.Selected[0]["state"] != "judged" || c.Selected[0]["outcome"] != "killed" {
				t.Errorf("selected %v, want its one site judged and killed", c.Selected)
			}
			if n := c.number("executed"); n != 1 {
				t.Errorf("executed %d, want 1", n)
			}

			// And no command downloads or installs anything
			if len(ran) == 0 {
				t.Errorf("stderr names no command preparation ran")
			}
			for _, line := range ran {
				for _, fetch := range []string{"pip install", "npm install", "npm ci", "npx ", "pnpm install", "yarn install"} {
					if strings.Contains(line, fetch) {
						t.Errorf("%q installs", line)
					}
				}
				_, command, _ := strings.Cut(line, "$ ")
				executable, _, _ := strings.Cut(command, " ")
				switch filepath.Base(executable) {
				case "gradle", "gradlew":
					if !strings.Contains(command, " --offline") {
						t.Errorf("%q may download: it runs Gradle without --offline", line)
					}
				case "mvn":
					if !strings.Contains(command, " -o") {
						t.Errorf("%q may download: it runs Maven without -o", line)
					}
				}
			}

			// And the live working tree is left unchanged
			if after := liveStatus(t, dir); after != before {
				t.Errorf("the live tree changed:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}
