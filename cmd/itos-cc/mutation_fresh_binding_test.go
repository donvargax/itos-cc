package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestMutationCountedUsesActualFreshAndTrialBindings(t *testing.T) {
	requireCountedPlatform(t)
	for _, shape := range []string{"bound", "missing Fresh", "wrong Fresh", "missing workers", "wrong workers", "missing tree", "wrong tree"} {
		t.Run(shape, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)
			gitOut(t, root, "init", "-q")
			// The host supplies only unchanged CLI discovery. Product reads,
			// preparation and trial requests must use the independent bindings.
			decoy := "package board\nfunc Value(x int) int { return x + 999 }\n"
			if err := os.WriteFile(filepath.Join(root, "board.go"), []byte(decoy), 0o600); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{filepath.Join(root, "go.mod"): "module example.test/board\n", filepath.Join(root, "board.go"): "package board\nfunc Value(x int) int { return x + 1 }\n", filepath.Join(root, "board_test.go"): "package board\nimport \"testing\"\nfunc TestValue(t *testing.T) { _ = Value(1) }\n"}
			repository := porttest.NewHistoryRepository()
			for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "independent fixture"}} {
				if err := repository.Git(root, root, files, args...); err != nil {
					t.Fatal(err)
				}
			}
			inputs := &porttest.Inputs{Dir: root, Files: files, Imports: map[string][]string{filepath.Join(root, "board.go"): {}, filepath.Join(root, "board_test.go"): {filepath.Join(root, "board.go")}}}
			tree := seltest.TreeOf(root, map[string]string{}, nil)
			workers := &porttest.Workers{Files: files, Tests: map[string]map[string]porttest.Tests{root: {"go test -count=1 -failfast ./.": {Elapsed: time.Millisecond}}}}
			fresh := &porttest.Fresh{History: porttest.History{Root: root, Dir: root, Repository: repository, Working: files}, Inputs: inputs, Tree: &tree, Commands: workers, Tools: []mutants.PreparationTool{{Name: "go", Path: "independent-installed-go"}},
				Coverage: &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: files}, Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: root}: {Language: "go", Report: "mode: set\nboard.go:2.1,2.44 1 1\n"}}},
				DecodeConfig: func(data []byte) ([]string, *mutants.Tests, []mutants.Exception, *mutants.FreshInvalid) {
					c, err := settings.Parse(data)
					if err != nil {
						invalid := err.(*settings.InvalidError)
						return nil, nil, nil, &mutants.FreshInvalid{Reason: invalid.Reason, Key: invalid.Key}
					}
					return c.Selection.Exclude, c.Tests, c.Exceptions, nil
				},
			}
			a := diskAdapters(&bytes.Buffer{}, nil)
			a.Fresh, a.FreshWorkers, a.Tree, a.Surroundings, a.Toolchain = fresh, workers, &tree, inputs, inputs
			switch shape {
			case "missing Fresh":
				a.Fresh = nil
			case "wrong Fresh":
				fresh.History.Root = t.TempDir()
			case "missing workers":
				a.FreshWorkers = nil
			case "wrong workers":
				a.FreshWorkers = &porttest.Workers{Files: files, Tests: map[string]map[string]porttest.Tests{}}
			case "missing tree":
				a.Tree = nil
			case "wrong tree":
				a.Tree = seltest.TreeOf(root, map[string]string{}, nil)
			}
			var out, log bytes.Buffer
			code := 0
			capturedOut, capturedLog := captured(t, func() {
				code = run(t.Context(), []string{"mutation", "run", "--count", "1", "--json"}, strings.NewReader(""), &out, &log, a)
			})
			out.WriteString(capturedOut)
			log.WriteString(capturedLog)
			if shape != "bound" {
				token := map[string]string{"missing Fresh": "Fresh binding is missing", "wrong Fresh": "repository", "missing workers": "Workers binding is missing", "wrong workers": "no tests declared", "missing tree": "fresh trial bindings are missing", "wrong tree": "fake"}[shape]
				if code == 0 || !strings.Contains(out.String()+log.String(), token) {
					t.Fatalf("%s did not expose actual missing/wrong binding: %d %s %s", shape, code, &out, &log)
				}
				return
			}
			var result countedJSON
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err, out.String(), log.String())
			}
			if code != 0 || result.number("executed") != 1 || len(result.Selected) != 1 || result.Selected[0]["outcome"] != mutants.Killed || len(workers.Opened) != 1 || workers.Opened[0] != mutants.FrozenTrials {
				t.Fatalf("actual counted binding: %d %s %s", code, &out, &log)
			}
			if bytes, err := os.ReadFile(filepath.Join(root, "board.go")); err != nil || string(bytes) != decoy {
				t.Fatalf("physical decoy changed: %q %v", bytes, err)
			}
			if _, err := os.Stat(filepath.Join(root, ".metrics")); !os.IsNotExist(err) {
				t.Fatalf("counted binding wrote persistent evidence: %v", err)
			}
			for path := range files {
				if strings.Contains(filepath.ToSlash(path), "/.git/itos/fresh-plan-memory-") {
					t.Fatal("owned frozen bytes remained after actual consumer returned")
				}
			}
		})
	}
}
