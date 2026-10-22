package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	covrun "github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	covporttest "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/lang"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The fakes the in-memory driver of TestScenarios builds from its memory
// for each command it runs: its git, coverage measurer, tool setup, test
// inputs, workers and test lister.

// commandOf is the command args name, past the flags before it, as run
// reads them.
func commandOf(args []string) string {
	for _, a := range args {
		switch {
		case a == "--":
			return ""
		case !strings.HasPrefix(a, "-"):
			return a
		}
	}
	return ""
}

// repoOf is the fake repository dir is in, and its folder: the nearest at
// or above dir, as git finds its top level; nil when there is none.
func (m *memory) repoOf(dir string) (string, *repo) {
	for p := dir; ; p = filepath.Dir(p) {
		if r := m.repos[p]; r != nil {
			return p, r
		}
		if filepath.Dir(p) == p {
			return "", nil
		}
	}
}

// changed is what git reports changed in the repository at root, as
// project.Changed asks it, from dir: each file of a supported language
// under dir that differs from the last commit or is not in it (the index
// standing in before the first commit), relative to dir, slash-separated.
func (m *memory) changed(dir, root string, r *repo) []string {
	if r == nil {
		return nil
	}
	base := map[string]string{}
	if head, failure := m.history(dir).Head(m.t.Context()); failure == nil {
		base = r.Commits[head].Files
	}
	var out []string
	for rel, text := range m.under(root) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if !under(abs, dir) || lang.Detect(rel) == nil {
			continue
		}
		if old, ok := base[rel]; ok && old == text {
			continue
		}
		fromDir, err := filepath.Rel(dir, abs)
		if err != nil {
			continue
		}
		out = append(out, filepath.ToSlash(fromDir))
	}
	slices.Sort(out)
	return out
}

// under is the files below dir, by path relative to it, slash-separated.
func (m *memory) under(dir string) map[string]string {
	files := map[string]string{}
	for path, text := range m.files {
		if under(path, dir) {
			rel, err := filepath.Rel(dir, path)
			if err == nil {
				files[filepath.ToSlash(rel)] = text
			}
		}
	}
	return files
}

// under is whether path is below dir.
func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// pathSet is the paths of files, as a set.
func pathSet(files map[string]string) map[string]bool {
	set := map[string]bool{}
	for p := range files {
		set[p] = true
	}
	return set
}

// measurer is coverage's Measurer fake over the memory, from dir: every
// file, the reports a run names among them, and the report of each build
// root, as the runner groups the sources by language and the nearest
// folder holding that language's marker, or their own folder. A root whose
// Coverage the scenario declared is measured as it says: by its actual Go,
// LCOV or JaCoCo report, every source of its language proven, or nothing, a
// report its tools do not write; a root declared for none is a tool missing.
// An earlier run left no report of any root, so reading what one left finds
// none.
func (m *memory) measurer(dir string) *covtest.Measurer {
	type group struct{ language, root string }
	groups := map[group][]string{}
	for _, path := range slices.Sorted(maps.Keys(m.files)) {
		spec := lang.Detect(path)
		if spec == nil || spec.IsTest(path) || covrun.Markers(spec.Name) == nil || under(path, filepath.Join(m.scratch(path), ".metrics")) {
			continue
		}
		g := group{spec.Name, m.buildRoot(path, covrun.Markers(spec.Name))}
		groups[g] = append(groups[g], path)
	}
	roots, left := map[string]*coverage.Report{}, map[string]*coverage.Report{}
	declarations := map[coverage.Unloaded]covtest.Measurement{}
	for g := range groups {
		report := &coverage.Report{}
		declared, ok := m.coverage[g.root]
		switch {
		case !ok || declared.Language != g.language:
			report.SetMissing(coverage.Unmeasured{Dir: g.root, Language: g.language, Cause: covport.ToolMissing{},
				Reason: "no tool the in-process driver declares measures it"})
		default:
			d := measurementOf(declared)
			if listed := m.listed[g.root]; listed != nil {
				d.Listed = map[string]map[string]*coverage.Report{}
				d.ListedFailed = listed.CoverageExit != 0
				d.ListedExit = listed.CoverageExit
				d.Failing = slices.Clone(listed.Failing)
				d.ListedOutput = listed.CoverageOutput
				d.Split = listed.Split
				d.Metadata = slices.Clone(listed.CoverageMetadata)
				d.ConversionExit = listed.ConversionExit
				d.ConversionOutput = listed.ConversionOutput
				lines := map[string]map[string][]coverage.Line{}
				for file, byLine := range listed.Lines {
					path := filepath.Join(g.root, filepath.FromSlash(file))
					spec := lang.Detect(path)
					if spec == nil {
						continue
					}
					if d.Listed[spec.Name] == nil {
						d.Listed[spec.Name] = map[string]*coverage.Report{}
					}
					for line, ids := range byLine {
						for _, id := range ids {
							if lines[id] == nil {
								lines[id] = map[string][]coverage.Line{}
							}
							lines[id][path] = append(lines[id][path], coverage.Line{Line: line, Covered: true})
						}
					}
				}
				for id, files := range lines {
					byLanguage := map[string]map[string][]coverage.Line{}
					for file, observed := range files {
						language := lang.Detect(file).Name
						if byLanguage[language] == nil {
							byLanguage[language] = map[string][]coverage.Line{}
						}
						byLanguage[language][file] = observed
					}
					for language, files := range byLanguage {
						d.Listed[language][id] = coverage.FromLines(language, files)
					}
				}
			}
			declarations[coverage.Unloaded{Language: g.language, Dir: g.root}] = d

		}
		roots[g.root] = coverage.Merge(roots[g.root], report)
		existing := &coverage.Report{}
		existing.SetMissing(coverage.Unmeasured{Dir: g.root, Language: g.language, Cause: covport.NoReport{}, Reason: "no report on disk measures its files"})
		left[g.root] = coverage.Merge(left[g.root], existing)
	}
	return &covtest.Measurer{Aggregate: covporttest.Measurer{Dir: dir, Files: maps.Clone(m.files), Roots: roots, Left: left}, Declarations: declarations}
}

// buildRoot is the nearest folder above path holding one of names, or
// path's own folder when none does, up to the scratch folder path is in.
func (m *memory) buildRoot(path string, names []string) string {
	top := m.scratch(path)
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		for _, name := range names {
			if _, ok := m.files[filepath.Join(dir, name)]; ok {
				return dir
			}
		}
		if dir == top || filepath.Dir(dir) == dir {
			return filepath.Dir(path)
		}
	}
}

// scratch is the scratch folder path is in, a folder Folder made.
func (m *memory) scratch(path string) string {
	return scratchOf(filepath.Dir(path))
}

// scratchOf is the scratch folder dir is, or is in: a folder Folder made.
func scratchOf(dir string) string {
	for d := dir; filepath.Dir(d) != d; d = filepath.Dir(d) {
		if filepath.Base(filepath.Dir(d)) == "itos-cc-in-process" {
			return d
		}
	}
	return dir
}

// from is every file the memory holds, as the disk holds it around a run
// in dir, the scratch folder dir is in and every other folder the scenario
// made, by path relative to dir, slash-separated, ../ leading out of it.
func (m *memory) from(dir string) map[string]string {
	files := map[string]string{}
	for path, text := range m.files {
		if rel, err := filepath.Rel(dir, path); err == nil {
			files[filepath.ToSlash(rel)] = text
		}
	}
	return files
}

// workers builds the actual Workers binding from current file and command
// declarations, the run's environment, its left-off programs and platform.
func (m *memory) workers(runs ...features.Run) *mutantstest.Workers {
	declarations := map[string]map[string]mutantstest.Tests{}
	for dir, commands := range m.tests {
		declarations[dir] = map[string]mutantstest.Tests{}
		for command, t := range commands {
			d := mutantstest.Tests{Baseline: t.Baseline, Elapsed: t.Elapsed}
			for _, outcome := range []struct {
				sites   []features.Site
				outcome string
			}{{t.Surviving, mutation.Survived}, {t.TimingOut, mutation.Timeout}, {capsWriting(t.Writing), mutation.Survived}, {t.Flooding, mutation.Timeout}} {
				for _, site := range outcome.sites {
					d.Verdicts = append(d.Verdicts, mutantstest.Verdict{Path: site.File, Site: mutation.Site{Line: site.Line, Original: site.Original, Replacement: site.Replacement}, Outcome: outcome.outcome})
				}
			}
			if t.Trace != nil {
				d.Trace = &mutantstest.Trace{File: t.Trace.File, Source: t.Trace.Source, Tests: maps.Clone(t.Trace.Tests), Coverage: t.Trace.Coverage, CoverDir: t.Trace.CoverDir}
			}
			declarations[dir][command] = d
		}
	}
	var run features.Run
	if len(runs) > 0 {
		run = runs[0]
	}
	env := runVariables(run.Env)
	return &mutantstest.Workers{Files: m.files, Tests: declarations, Write: m.Write, Env: env, Without: slices.Clone(run.Without), Platform: m.Platform()}
}

// capsWriting are the sites of writing, whose mutants make the tests
// pass writing more: as a run with no output cap judges them, survivors,
// as one whose tests write without end runs to its timeout and one whose
// tests hold ever more memory fails them, killed.
func capsWriting(writing map[features.Site]int64) []features.Site {
	return slices.Collect(maps.Keys(writing))
}

// Tools records independently declared tool/probe setup, never Tests.Command.
func (m *memory) Tools(ctx context.Context, dir string, setup features.ToolSetup) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	for _, name := range setup.NodeBins {
		if filepath.Base(name) != name {
			return fmt.Errorf("invalid fixture NodeBin %q", name)
		}
		bin := name
		if m.Platform() == "windows" {
			bin += ".cmd"
		}
		if err := m.Write(filepath.Join(dir, "node_modules", ".bin", bin), nil); err != nil {
			return err
		}
	}
	if setup.Python != nil && setup.Python.Interpreter == "" {
		return fmt.Errorf("Python setup probe needs its actual interpreter identity")
	}
	if setup.Python != nil {
		env := map[string]string{}
		for _, value := range setup.Env {
			key, text, _ := strings.Cut(value, "=")
			env[key] = text
		}
		key := mutantstest.ProbeKey(setup.Python.Interpreter, dir, env)
		if old, ok := m.toolSetup[key]; ok && old != setup.Python.Importable {
			return fmt.Errorf("conflicting Python probe setup for %s", key.Interpreter)
		}
		m.toolSetup[key] = setup.Python.Importable
	}
	return nil
}

// runVariables are a run's variables by name: each NAME=value set and each
// NAME alone unset, in order, as the binary driver applies them.
func runVariables(vars []string) map[string]string {
	env := map[string]string{}
	for _, value := range vars {
		key, text, set := strings.Cut(value, "=")
		if !set {
			delete(env, key)
			continue
		}
		env[key] = text
	}
	return env
}

// inputs supplies independent tool/import/runtime facts to actual bound ports.
// It never derives availability or reach from a Workers outcome declaration.
func (m *memory) inputs(r features.Run) *mutantstest.Inputs {
	env := runVariables(r.Env)
	imports := map[string][]string(nil)
	for root, edges := range m.imports {
		if filepath.Clean(r.Dir) == root || under(filepath.Clean(r.Dir), root) {
			if imports == nil {
				imports = map[string][]string{}
			}
			// A file no longer there, removed or moved by git, resolves
			// no import, and none resolves to it.
			for path, targets := range edges {
				if _, ok := m.files[path]; !ok {
					continue
				}
				imports[path] = slices.DeleteFunc(slices.Clone(targets), func(target string) bool { _, ok := m.files[target]; return !ok })
			}
		}
	}
	// What git tracks is the index of the fake repository the run's
	// folder is in, and nil in none.
	var tracked map[string]bool
	if root, git := m.repoOf(filepath.Clean(r.Dir)); git != nil {
		tracked = map[string]bool{}
		for path := range git.Index {
			tracked[filepath.Join(root, filepath.FromSlash(path))] = true
		}
	}
	return &mutantstest.Inputs{Dir: filepath.Clean(r.Dir), Files: m.files, Dirs: m.dirs, Tracked: tracked,
		Runtime: mutation.RuntimeInputs{Platform: m.Platform(), Architecture: runtime.GOARCH, Env: env},
		Without: slices.Clone(r.Without), Probes: maps.Clone(m.toolSetup), Imports: imports}
}

// measurementOf translates fixture artifact declarations, not application models.
func measurementOf(c features.Coverage) covtest.Measurement {
	return covtest.Measurement{Language: c.Language, Report: c.Report, IntegrationReport: c.IntegrationReport, CommandExit: c.CommandExit, CommandOutput: c.CommandOutput, Output: c.Output, WithoutC8: c.WithoutC8, InventoryReport: c.InventoryReport, InventoryExit: c.InventoryExit, InventoryOutput: c.InventoryOutput}
}

// lister composes only the independently retained root setup with the actual
// configured command. It never infers inventory from command-keyed outcomes.
func (m *memory) lister(root string, text []byte, log io.Writer) *mutantstest.Lister {
	facts := map[string]map[string]mutantstest.Listing{}
	cfg, err := config.Parse(text)
	if err == nil && cfg.Tests != nil {
		if setup, ok := m.listed[root]; ok && setup != nil {
			value := mutantstest.Listing{Stdout: setup.ListOutput}
			if value.Stdout == "" {
				for _, id := range slices.Sorted(maps.Keys(setup.Files)) {
					value.Stdout += id + "\t" + setup.Files[id] + "\n"
				}
			}
			if setup.ListFails {
				value.ExitCode = 1
				value.Reason = "exit status 1"
			}
			if trace := setup.ListTrace; trace != "" {
				value.Record = func() error {
					return m.Write(trace, append([]byte(m.files[trace]), []byte(`{"test":"list"}`+"\n")...))
				}
			}
			facts[root] = map[string]mutantstest.Listing{cfg.Tests.List: value}
		}
	}
	return &mutantstest.Lister{Listings: facts, Log: log}
}
