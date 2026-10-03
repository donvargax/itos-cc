package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/lang"
)

// Plan is how to measure one project: the directory to run in, the commands
// that produce coverage, and where the reports land. A project is one
// language under one build root (go.mod, package.json, pyproject.toml, a
// Gradle or Maven build).
type Plan struct {
	Language string
	Dir      string
	Commands [][]string
	Reports  []string // absolute
	// Existing are report locations to read with --use-existing-coverage,
	// in order of preference. Reports come first.
	Existing []string
}

var markers = map[string][]string{
	"go":         {"go.mod"},
	"typescript": {"package.json"},
	"python":     {"pyproject.toml", "setup.py", "setup.cfg"},
	"kotlin":     {"build.gradle.kts", "build.gradle", "pom.xml"},
}

// Plans groups sources by language and build root and returns one plan per
// group, with reports written under outDir.
func Plans(sources []string, outDir string) []Plan {
	type key struct{ lang, dir string }
	groups := map[key]bool{}
	for _, s := range sources {
		spec := lang.Detect(s)
		if spec == nil {
			continue
		}
		dir := lang.FindUp(s, markers[spec.Name]...)
		if dir == "" {
			dir = filepath.Dir(s)
		}
		groups[key{spec.Name, dir}] = true
	}
	var plans []Plan
	for k := range groups {
		out := filepath.Join(outDir, k.lang+"-"+shortHash(k.dir))
		switch k.lang {
		case "go":
			plans = append(plans, goPlan(k.dir, out))
		case "typescript":
			plans = append(plans, typescriptPlan(k.dir, out))
		case "python":
			plans = append(plans, pythonPlan(k.dir, out))
		case "kotlin":
			plans = append(plans, kotlinPlan(k.dir))
		}
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Language+plans[i].Dir < plans[j].Language+plans[j].Dir
	})
	return plans
}

func goPlan(dir, out string) Plan {
	report := filepath.Join(out, "coverage.out")
	return Plan{
		Language: "go",
		Dir:      dir,
		Commands: [][]string{{"go", "test", "./...", "-count=1", "-covermode=set", "-coverpkg=./...", "-coverprofile=" + report}},
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage.out"), filepath.Join(dir, "cover.out")},
	}
}

func typescriptPlan(dir, out string) Plan {
	report := filepath.Join(out, "lcov.info")
	plan := Plan{
		Language: "typescript",
		Dir:      dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage", "lcov.info")},
	}
	pkg := readPackageJSON(dir)
	switch {
	case pkg.Scripts["coverage"] != "":
		// The project's own script decides where it writes; the usual place
		// is coverage/lcov.info.
		plan.Commands = [][]string{{"npm", "run", "coverage"}}
		plan.Reports = []string{filepath.Join(dir, "coverage", "lcov.info")}
	case pkg.has("vitest"):
		if !exists(filepath.Join(dir, "node_modules", "@vitest", "coverage-v8")) {
			// Without the provider Vitest stops to ask; install the version
			// that matches Vitest, without touching package.json.
			provider := "@vitest/coverage-v8"
			if v := installedVersion(dir, "vitest"); v != "" {
				provider += "@" + v
			}
			plan.Commands = append(plan.Commands, []string{"npm", "install", "--no-save", provider})
		}
		plan.Commands = append(plan.Commands, []string{"npx", "vitest", "run", "--coverage.enabled",
			"--coverage.reporter=lcov", "--coverage.reportsDirectory=" + out})
	case pkg.has("jest"):
		plan.Commands = [][]string{{"npx", "jest", "--coverage", "--coverageReporters=lcov", "--coverageDirectory=" + out}}
	default:
		plan.Commands = [][]string{{"npx", "--yes", "c8", "--reporter=lcov", "--reports-dir=" + out, "npm", "test"}}
	}
	return plan
}

func pythonPlan(dir, out string) Plan {
	py := pythonFor(dir)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	runner := []string{"-m", "unittest", "discover"}
	if exec.Command(py, "-c", "import pytest").Run() == nil {
		runner = []string{"-m", "pytest", "-q"}
	}
	run := append([]string{py, "-m", "coverage", "run", "--data-file=" + data, "--source=" + dir}, runner...)
	return Plan{
		Language: "python",
		Dir:      dir,
		Commands: [][]string{run, {py, "-m", "coverage", "lcov", "--data-file=" + data, "-o", report}},
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "lcov.info"), filepath.Join(dir, "coverage", "lcov.info")},
	}
}

func kotlinPlan(dir string) Plan {
	kover := filepath.Join(dir, "build", "reports", "kover", "report.xml")
	jacoco := filepath.Join(dir, "build", "reports", "jacoco", "test", "jacocoTestReport.xml")
	maven := filepath.Join(dir, "target", "site", "jacoco", "jacoco.xml")
	plan := Plan{Language: "kotlin", Dir: dir, Existing: []string{kover, jacoco, maven}}
	if exists(filepath.Join(dir, "pom.xml")) {
		const jacocoPlugin = "org.jacoco:jacoco-maven-plugin:0.8.12:"
		plan.Commands = [][]string{{"mvn", "-q", jacocoPlugin + "prepare-agent", "test", jacocoPlugin + "report"}}
		plan.Reports = []string{maven}
		return plan
	}
	gradle := "gradle"
	if root := lang.FindUp(filepath.Join(dir, "x"), "gradlew"); root != "" {
		gradle = filepath.Join(root, "gradlew")
	}
	if buildMentions(dir, "kover") {
		plan.Commands = [][]string{{gradle, "-p", dir, "koverXmlReport"}}
		plan.Reports = []string{kover}
	} else {
		plan.Commands = [][]string{{gradle, "-p", dir, "test", "jacocoTestReport"}}
		plan.Reports = []string{jacoco}
	}
	return plan
}

// Run executes each plan and returns the coverage of sources. A plan whose
// commands fail still contributes any report it wrote, because failing tests
// still measure the code they ran. Problems are written to log.
func Run(plans []Plan, sources []string, log io.Writer) *Report {
	var reports []*Report
	for _, p := range plans {
		for _, r := range p.Reports {
			os.Remove(r)
		}
		os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755)
		for _, args := range p.Commands {
			fmt.Fprintf(log, "coverage: %s$ %s\n", p.Dir, strings.Join(args, " "))
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = p.Dir
			cmd.Stdout = log
			cmd.Stderr = log
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(log, "coverage: %s: %v\n", p.Language, err)
			}
		}
		reports = append(reports, load(p.Reports, p.Dir, sources, log))
	}
	return Merge(reports...)
}

// IgnoreDir creates dir with a .gitignore that ignores everything in it, so
// raw reports stay out of version control while the rest of .metrics is
// committed.
func IgnoreDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644)
}

// Existing reads reports already on disk, the first one found per plan.
func Existing(plans []Plan, sources []string, log io.Writer) *Report {
	var reports []*Report
	for _, p := range plans {
		for _, r := range p.Existing {
			if exists(r) {
				reports = append(reports, load([]string{r}, p.Dir, sources, log))
				break
			}
		}
	}
	return Merge(reports...)
}

// Files reads reports named on the command line, relative to the working
// directory.
func Files(paths []string, sources []string, log io.Writer) *Report {
	wd, _ := os.Getwd()
	return load(paths, wd, sources, log)
}

func load(paths []string, base string, sources []string, log io.Writer) *Report {
	var all [][]Entry
	for _, path := range paths {
		entries, err := Load(path)
		if err != nil {
			fmt.Fprintf(log, "coverage: %v\n", err)
			continue
		}
		all = append(all, entries)
	}
	return Build(sources, base, all...)
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (p packageJSON) has(dep string) bool {
	return p.Dependencies[dep] != "" || p.DevDependencies[dep] != ""
}

func readPackageJSON(dir string) packageJSON {
	var pkg packageJSON
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		json.Unmarshal(data, &pkg)
	}
	return pkg
}

func installedVersion(dir, dep string) string {
	var pkg struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "node_modules", dep, "package.json"))
	if err == nil {
		json.Unmarshal(data, &pkg)
	}
	return pkg.Version
}

// pythonFor prefers the project's own virtualenv, so tests run with the
// project's dependencies.
func pythonFor(dir string) string {
	for _, venv := range []string{".venv", "venv"} {
		py := filepath.Join(dir, venv, "bin", "python")
		if exists(py) {
			return py
		}
	}
	return "python3"
}

func buildMentions(dir, word string) bool {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), word) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
