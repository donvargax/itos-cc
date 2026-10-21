package domain

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// ObserveCommand applies pure command policy to actual bound tool facts.
func ObserveCommand(ctx context.Context, tools Toolchain, request CommandRequest) (Command, InputFailure) {
	facts, failure := tools.Observe(ctx, request)
	if failure != nil {
		return Command{}, failure
	}
	return ChooseCommand(request, facts), nil
}

// ChooseCommand constructs the narrowest command from observations, without
// discovering files, executing probes or knowing their concrete implementation.
func ChooseCommand(request CommandRequest, facts ToolFacts) Command {
	c := Command{Root: facts.Root, Dir: facts.Root}
	switch facts.Language {
	case "go":
		pkg, _ := filepath.Rel(facts.Root, filepath.Dir(request.Path))
		target := "./" + filepath.ToSlash(pkg)
		if request.All {
			target = "./..."
		}
		c.Args = []string{"go", "test", "-count=1", "-failfast", target}
	case "typescript":
		rel, _ := filepath.Rel(facts.Root, request.Path)
		switch {
		case facts.Vitest != "" && request.All:
			c.Args = []string{facts.Vitest, "run", "--bail=1"}
		case facts.Vitest != "":
			c.Args = []string{facts.Vitest, "related", "--run", "--bail=1", filepath.ToSlash(rel)}
		case facts.Jest != "" && request.All:
			c.Args = []string{facts.Jest, "--bail"}
		case facts.Jest != "":
			c.Args = []string{facts.Jest, "--bail", "--findRelatedTests", filepath.ToSlash(rel)}
		default:
			c.Args = []string{facts.PackageManager, "run", "test"}
			if facts.PackageManager == "npm" {
				c.Args = append(c.Args, "--silent")
			}
			c.Suite = true
		}
	case "python":
		c.PathEnv, c.PathDirs = "PYTHONPATH", []string{".", "src"}
		if request.Shell == "" && (request.All || len(facts.PythonFiles) > 0) {
			var run, modules []string
			if !request.All {
				run, modules = facts.PythonFiles, facts.PythonModules
			}
			c.Args = PythonArgs(facts.Interpreter, facts.Pytest, run, modules)
		}
	case "dart":
		c.Args, c.Ready = DartArgs(request, facts)
		if request.Shell != "" {
			// A --test-command is the person's own: it readies its copy
			// as it needs, and may need no flutter at all.
			c.Ready = nil
		}
	case "kotlin":
		if facts.Runner == "mvn" {
			c = mavenChoice(request, facts)
		} else {
			c = gradleChoice(request, facts)
		}
	}
	if request.Shell != "" {
		c.Args, c.Shell = nil, request.Shell
	}
	return c
}

func gradleChoice(request CommandRequest, facts ToolFacts) Command {
	c := Command{Root: facts.Root, Dir: facts.Root}
	if request.All {
		c.Args = []string{facts.Runner, "test", "--fail-fast"}
		return c
	}
	classes := classesWithin(facts.Root, facts.Classes)
	switch len(classes) {
	case 0:
		return c
	case 1:
		for dir, names := range classes {
			rel, _ := filepath.Rel(facts.Root, dir)
			c.Args = []string{facts.Runner, "-p", filepath.ToSlash(rel), "test", "--fail-fast"}
			c.Args = append(c.Args, GradleTests(names)...)
		}
		return c
	}
	c.Args = []string{facts.Runner}
	for _, dir := range slices.Sorted(maps.Keys(classes)) {
		rel, _ := filepath.Rel(facts.Root, dir)
		task := ":test"
		if rel != "." {
			task = ":" + strings.ReplaceAll(filepath.ToSlash(rel), "/", ":") + task
		}
		c.Args = append(c.Args, task, "--fail-fast")
		c.Args = append(c.Args, GradleTests(classes[dir])...)
	}
	return c
}

func mavenChoice(request CommandRequest, facts ToolFacts) Command {
	c := Command{Root: facts.Module, Dir: facts.Module}
	if request.All {
		c.Args = []string{"mvn", "-q", "test"}
		return c
	}
	classes := classesWithin(facts.Reactor, facts.Classes)
	var names, others []string
	for _, dir := range slices.Sorted(maps.Keys(classes)) {
		names = append(names, classes[dir]...)
		if dir != facts.Module {
			rel, _ := filepath.Rel(facts.Reactor, dir)
			others = append(others, filepath.ToSlash(rel))
		}
	}
	if len(names) == 0 {
		return c
	}
	slices.Sort(names)
	c.Args = []string{"mvn", "-q", "test"}
	if len(others) > 0 {
		c.Root, c.Dir = facts.Reactor, facts.Reactor
		var pl []string
		if classes[facts.Module] != nil {
			rel, _ := filepath.Rel(facts.Reactor, facts.Module)
			pl = append(pl, filepath.ToSlash(rel))
		}
		c.Args = append(c.Args, "-pl", strings.Join(append(pl, others...), ","), "-am")
	}
	// These are the existing coverage runner's Maven selection arguments.
	c.Args = append(c.Args, MavenTests(slices.Compact(names))...)
	return c
}

func classesWithin(root string, classes map[string][]string) map[string][]string {
	out := map[string][]string{}
	for dir, names := range classes {
		if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			out[dir] = names
		}
	}
	return out
}

// PythonArgs is shared by ordinary policy and frozen preparation after each
// independently observes its probe and runnable files/modules.
func PythonArgs(py string, pytest bool, run, modules []string) []string {
	switch {
	case pytest:
		return append([]string{py, "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"}, run...)
	case run != nil:
		return append([]string{py, "-m", "unittest", "-f"}, modules...)
	}
	return []string{py, "-m", "unittest", "discover", "-f"}
}

// DartArgs runs a Dart package's tests with flutter test --no-pub, never
// resolving anything: the test files that reach the source (DartFiles), the
// whole suite of test/ for All, and nothing for a source no test reaches.
// flutter test has no fail-fast flag, so its run is the files' whole run.
// ready resolves a worker's copy offline before its first run.
func DartArgs(request CommandRequest, facts ToolFacts) (args, ready []string) {
	if len(facts.Flutter) == 0 {
		return nil, nil
	}
	ready = append(slices.Clone(facts.Flutter), "pub", "get", "--offline")
	if !request.All && len(facts.DartFiles) == 0 {
		return nil, ready
	}
	args = append(slices.Clone(facts.Flutter), "test", "--no-pub")
	if !request.All {
		args = append(args, facts.DartFiles...)
	}
	return args, ready
}
