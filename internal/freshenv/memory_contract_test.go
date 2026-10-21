package freshenv

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/coverage"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func decodeFreshSettings(data []byte) ([]string, *mutants.Tests, []mutants.Exception, *mutants.FreshInvalid) {
	c, err := config.Parse(data)
	if err != nil {
		invalid := err.(*config.InvalidError)
		return nil, nil, nil, &mutants.FreshInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	return c.Selection.Exclude, c.Tests, c.Exceptions, nil
}

func memoryCodecFreshFixture(t *testing.T) porttest.FreshFixture {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{}
	repo := porttest.NewHistoryRepository()
	inputs := &porttest.Inputs{Dir: root, Files: files}
	tree := seltest.TreeOf(root, map[string]string{}, nil)
	f := &porttest.Fresh{History: porttest.History{Root: root, Dir: root, Repository: repo, Working: files}, DecodeConfig: decodeFreshSettings, Inputs: inputs, Tree: &tree}
	h := porttest.HistoryFixture{Root: root, At: func(dir string) mutants.History {
		return porttest.History{Root: root, Dir: dir, Repository: repo, Working: files}
	},
		Put: func(_ context.Context, name, text string) error {
			files[filepath.Join(root, filepath.FromSlash(name))] = text
			return nil
		},
		Git: func(_ context.Context, args ...string) error { return repo.Git(root, root, files, args...) },
	}
	return porttest.FreshFixture{HistoryFixture: h, Fresh: f, Read: func(ctx context.Context, path string) ([]byte, error) {
		if text, ok := inputs.Read(ctx, path); ok {
			return text, nil
		}
		return nil, fmt.Errorf("missing virtual frozen input %s", path)
	}}
}

func TestMemoryFreshContract(t *testing.T) { porttest.FreshContract(t, memoryCodecFreshFixture) }

func TestMemoryFreshPreparationContract(t *testing.T) {
	porttest.FreshPreparationContract(t, func(t *testing.T) porttest.FreshPreparationFixture {
		b := memoryCodecFreshFixture(t)
		for name, text := range map[string]string{
			"go.mod":       "module example.com/prep\n\ngo 1.22\n",
			"main.go":      "package main\nfunc Value(x int) int { return x + 1 }\nfunc Empty() {}\n",
			"main_test.go": "package main\nimport \"testing\"\nfunc TestValue(t *testing.T) { _ = Value(1) }\n",
			"itos-cc.yaml": "mutation:\n  tests:\n    list: fresh-list\n    run: fresh-listed-select {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n    whole: fresh-listed-all\n",
		} {
			if err := b.Put(t.Context(), name, text); err != nil {
				t.Fatal(err)
			}
		}
		for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "actual Go fixture"}} {
			if err := b.Git(t.Context(), args...); err != nil {
				t.Fatal(err)
			}
		}
		f := b.Fresh.(*porttest.Fresh)
		return porttest.FreshPreparationFixture{Fresh: f, Root: b.Root, Collector: func(p *mutants.FreshPlan, exit int) {
			root := p.FrozenRoot
			source, test := filepath.Join(root, "main.go"), filepath.Join(root, "main_test.go")
			f.Inputs.Imports = map[string][]string{source: {}, test: {source}}
			profile := fmt.Sprintf("mode: set\n%s:2.1,2.40 1 1\n", filepath.ToSlash(source))
			entries, err := coverage.Parse(strings.NewReader(profile))
			if err != nil {
				t.Fatal(err)
			}
			r := coverage.Build(t.Context(), aggregate.Files{Dir: root, Regular: []string{source}}, []string{source}, root, entries)
			f.Coverage = &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: f.Inputs.Files}, Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: root}: {Language: "go", Report: profile, CommandExit: exit, CommandOutput: "independent collector failed after report write", Metadata: []byte("actual declared converter input"), Split: true, Listed: map[string]map[string]*coverage.Report{"go": {"T-1": r}}}}}
			f.Tools = []mutants.PreparationTool{{Name: "go", Path: "independent-installed-go"}}
			f.Lister = &porttest.Lister{Listings: map[string]map[string]porttest.Listing{root: {"fresh-list": {Stdout: "T-1\tmain_test.go\n"}}}}
			f.Commands = &porttest.Workers{Files: f.Inputs.Files, Tests: map[string]map[string]porttest.Tests{root: {"go test -count=1 -failfast ./.": {Elapsed: time.Millisecond}}}}
		}}
	})
}
