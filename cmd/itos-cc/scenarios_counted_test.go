package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/coverage"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

// counted are the bindings a counted or fail-fast mutation run starts from,
// over the memory at dir, as the disk's adapters bind them: porttest's Fresh
// over the fake repository's committed bytes, the same actual Workers for its
// trials, the fake repository's root, dir as the physical working directory
// and the configured platform value.
type counted struct {
	Fresh        mutation.Fresh
	FreshWorkers mutation.Workers
	Repository   func(context.Context) (string, error)
	PhysicalDir  func(context.Context) (string, error)
	Platform     string
}

// counted binds them for a run in dir, over its tree, inputs, coverage,
// workers and lister, the very fakes the rest of the run is bound to.
func (m *memory) counted(dir string, tree *selporttest.Tree, inputs *mutantstest.Inputs, measurer *covtest.Measurer, workers *mutantstest.Workers, lister mutation.TestLister) counted {
	root, repository := m.repoOf(dir)
	return counted{
		Fresh: &mutantstest.Fresh{
			History:  mutantstest.History{Root: root, Dir: dir, Repository: repository, Working: m.files},
			Coverage: measurer, Commands: workers, Lister: lister, Tools: m.preparationTools(inputs),
			DecodeConfig: decodeFreshSettings, Inputs: inputs, Tree: tree,
			Measured: m.freshMeasured,
		},
		FreshWorkers: workers,
		Repository: func(ctx context.Context) (string, error) {
			if err := context.Cause(ctx); err != nil {
				return "", err
			}
			if repository == nil {
				return "", fmt.Errorf("fatal: not a git repository (the in-process driver's): %s", dir)
			}
			return root, nil
		},
		PhysicalDir: func(ctx context.Context) (string, error) {
			if err := context.Cause(ctx); err != nil {
				return "", err
			}
			return dir, nil
		},
		Platform: m.Platform(),
	}
}

// freshMeasured observes a collection a fresh preparation made in p's
// frozen inputs, as the harness of each declared trace records the runs it
// makes: the own tests' collection by the commands declared for them, the
// listed tests' by the run command selecting them all, each record naming
// the frozen inputs it ran in and the original root, and that it measured
// coverage where the harness says so.
func (m *memory) freshMeasured(_ context.Context, p *mutation.FreshPlan, listed *coverage.PerTest) error {
	root := filepath.Clean(p.Root)
	seen := map[string]bool{}
	for _, command := range slices.Sorted(maps.Keys(m.tests[root])) {
		t := m.tests[root][command]
		if t.Trace == nil || seen[t.Trace.File] || strings.Contains(command, "{pattern}") != (listed != nil) {
			continue
		}
		shell := command
		if listed != nil {
			shell = listed.All
		}
		names, err := mutantstest.SelectedTests(mutation.Command{Shell: shell, Root: p.FrozenRoot, Dir: p.FrozenRoot}, t.Trace.Tests)
		if err != nil {
			return err
		}
		seen[t.Trace.File] = true
		text, ok := m.files[filepath.Join(p.FrozenRoot, filepath.FromSlash(t.Trace.Source))]
		if !ok {
			return fmt.Errorf("frozen trace source missing: %s", t.Trace.Source)
		}
		digest := sha256.Sum256([]byte(text))
		for _, name := range names {
			record := map[string]any{"cwd": p.FrozenRoot, "original_root": root, "source_sha256": hex.EncodeToString(digest[:])}
			if name != "" {
				record["test"] = name
			}
			if t.Trace.Coverage {
				record["coverage"] = true
			}
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := m.Write(t.Trace.File, append([]byte(m.files[t.Trace.File]), append(data, '\n')...)); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeFreshSettings is the one settings codec over committed bytes.
func decodeFreshSettings(data []byte) ([]string, *mutation.Tests, []mutation.Exception, *mutation.FreshInvalid) {
	c, err := config.Parse(data)
	if err != nil {
		var invalid *config.InvalidError
		if !errors.As(err, &invalid) {
			return nil, nil, nil, &mutation.FreshInvalid{Reason: err.Error()}
		}
		return nil, nil, nil, &mutation.FreshInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	return c.Exclude, c.Tests, c.Exceptions, nil
}

// hostTools are the programs the in-process driver's host has on its PATH
// but for those a run leaves off it (Run.Without), as the Workers fake runs
// any program a run does not leave off.
var hostTools = []string{"go", "python3", "python", "node", "npm", "npx", "pnpm", "yarn", "bun", "gradle", "mvn", "java", "make", "sh", "cmd"}

// preparationTools are the tools a fresh preparation resolves before it
// runs any: each host tool the run leaves on its PATH, and each program a
// project installed, by its path, as an installed node_modules/.bin or a
// virtualenv's bin holds it.
func (m *memory) preparationTools(inputs *mutantstest.Inputs) []mutation.PreparationTool {
	var tools []mutation.PreparationTool
	for _, name := range hostTools {
		if !leftOff(name, inputs.Without, inputs.Runtime.Platform) {
			tools = append(tools, mutation.PreparationTool{Name: name, Path: filepath.Join(string(filepath.Separator)+"memory-host", "bin", name)})
		}
	}
	for _, path := range slices.Sorted(maps.Keys(m.files)) {
		if dir := filepath.Base(filepath.Dir(path)); dir == "bin" && filepath.Base(filepath.Dir(filepath.Dir(path))) == ".venv" || dir == ".bin" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "node_modules" {
			tools = append(tools, mutation.PreparationTool{Name: path, Path: path})
		}
	}
	return tools
}

// leftOff is whether a run leaving the programs without off its PATH, each
// a name or a prefix ending in *, leaves name off it, on platform.
func leftOff(name string, without []string, platform string) bool {
	for _, w := range without {
		if platform == "windows" {
			w, name = strings.ToLower(w), strings.ToLower(name)
		}
		if prefix, ok := strings.CutSuffix(w, "*"); ok && strings.HasPrefix(name, prefix) || w == name {
			return true
		}
	}
	return false
}
