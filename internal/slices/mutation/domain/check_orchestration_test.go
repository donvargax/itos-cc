package domain_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	selected "github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

func checkOptions(f *runFixture) mutants.CheckOptions {
	o := f.options
	return mutants.CheckOptions{Store: o.Store, Tree: o.Tree, Surroundings: o.Surroundings, Root: o.Root, Dir: o.Dir, Toolchain: o.Toolchain, Fingerprints: o.Fingerprints, Tests: o.Tests, Support: o.Support, Judge: o.Judge, Exceptions: o.Exceptions}
}
func TestCheckUsesActualSnapshotsAndCurrentInputsWithoutWrites(t *testing.T) {
	f := newRunFixture(t)
	f.run(t)
	before := f.snapshots.Values["board.go"]
	written := slices.Clone(f.snapshots.Written)
	checks, err := mutants.Check(t.Context(), []string{f.path}, checkOptions(f))
	if err != nil || len(checks) != 1 || len(checks[0].Functions) != 1 || checks[0].Functions[0].State != mutants.SnapshotFresh || len(checks[0].Functions[0].Mutants) == 0 {
		t.Fatalf("actual fresh check: %+v %v", checks, err)
	}
	f.files[filepath.Join(f.root, "board_test.go")] += "// independently edited test\n"
	checks, err = mutants.Check(t.Context(), []string{f.path}, checkOptions(f))
	if err != nil || checks[0].Functions[0].State != mutants.Stale || checks[0].Functions[0].Tests == nil || checks[0].Functions[0].Mutants != nil {
		t.Fatalf("actual changed tests: %+v %v", checks, err)
	}
	if !reflect.DeepEqual(before, f.snapshots.Values["board.go"]) || !slices.Equal(written, f.snapshots.Written) || len(f.edits.Writes) != 0 {
		t.Fatal("read-only check changed source/snapshot")
	}
	delete(f.snapshots.Values, "board.go")
	checks, err = mutants.Check(t.Context(), []string{f.path}, checkOptions(f))
	if err != nil || checks[0].Functions[0].State != mutants.Missing {
		t.Fatalf("missing: %+v %v", checks, err)
	}
}

func TestParsedCheckPreservesJudgedOutcomesForSharedConsumers(t *testing.T) {
	f := newRunFixture(t)
	f.run(t)
	value := f.snapshots.Values["board.go"]
	if len(value.Units[0].Mutants) < 2 {
		t.Fatal("fixture needs multiple actual sites")
	}
	value.Units[0].Mutants = value.Units[0].Mutants[1:]
	f.snapshots.Values["board.go"] = value
	opt := checkOptions(f)
	parsed, err := selected.Parse(t.Context(), opt.Tree, opt.Surroundings, f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	shared, err := mutants.CheckParsed(t.Context(), parsed, f.path, opt)
	if err != nil || !shared.Functions[0].Judged() || len(shared.Functions[0].Unrecorded) != 1 || len(shared.Functions[0].Mutants) != len(value.Units[0].Mutants) {
		t.Fatalf("shared stale-but-judged: %+v %v", shared, err)
	}
	// Parsed consumers require observation bindings, not a second Tree read.
	opt.Tree, opt.Surroundings = nil, nil
	if _, err := mutants.CheckParsed(t.Context(), parsed, f.path, opt); err != nil {
		t.Fatalf("already parsed input required a Tree: %v", err)
	}
	for _, missing := range []string{"reader", "fingerprints"} {
		absent := opt
		if missing == "reader" {
			absent.Store = nil
		} else {
			absent.Fingerprints = nil
		}
		if _, err := mutants.CheckParsed(t.Context(), parsed, f.path, absent); err == nil {
			t.Fatalf("parsed consumer accepted missing %s", missing)
		}
	}
	opt = checkOptions(f)
	checked, err := mutants.Check(t.Context(), []string{f.path}, opt)
	if err != nil || checked[0].Functions[0].State != mutants.Stale || checked[0].Functions[0].Mutants != nil {
		t.Fatalf("check presentation filter: %+v %v", checked, err)
	}
}

func TestCheckBroadAndListedEvidenceUsesCurrentFacts(t *testing.T) {
	for _, scope := range []string{mutants.ScopeAllTests, mutants.ScopeListed} {
		t.Run(scope, func(t *testing.T) {
			f := newRunFixture(t)
			f.run(t)
			value := f.snapshots.Values["board.go"]
			for i := range value.Units[0].Mutants {
				value.Units[0].Mutants[i].Scope = scope
			}
			if scope == mutants.ScopeAllTests {
				suite, failure := f.inputs.Suite(t.Context(), f.path, f.root)
				if failure != nil {
					t.Fatal(failure)
				}
				mutants.RecordSuiteEvidence(&value, suite, nil)
			} else {
				value.Listed = map[string]string{"actual-id": "board_test.go"}
				hashes, failure := f.inputs.Tests(t.Context(), f.root, []string{filepath.Join(f.root, "board_test.go")})
				if failure != nil {
					t.Fatal(failure)
				}
				value.ListedFiles = hashes
				for i := range value.Units[0].Mutants {
					value.Units[0].Mutants[i].Tests = []string{"actual-id"}
				}
			}
			f.snapshots.Values["board.go"] = value
			opt := checkOptions(f)
			opt.Tests = nil // isolate broad/listed dependencies from own-test freshness.
			value.Tests = map[string]string{}
			f.snapshots.Values["board.go"] = value
			checks, err := mutants.Check(t.Context(), []string{f.path}, opt)
			if err != nil || checks[0].Functions[0].State != mutants.SnapshotFresh {
				t.Fatalf("independent scope evidence: %+v %v", checks, err)
			}
			f.files[filepath.Join(f.root, "board_test.go")] += "// edited actual dependency\n"
			checks, err = mutants.Check(t.Context(), []string{f.path}, opt)
			if err != nil || checks[0].Functions[0].State != mutants.Stale {
				t.Fatalf("scope change: %+v %v", checks, err)
			}
		})
	}
}

func TestCheckStrictInventoryIncludesZeroSiteUnitsAndGoAlone(t *testing.T) {
	f := newRunFixture(t)
	source := "package board\nfunc Empty() {}\nfunc Value(x int) bool { return x > 1 }\n"
	f.files[f.path] = source
	f.options.Tree = seltest.TreeOf(f.root, map[string]string{"board.go": source, "go.mod": "module example.test/board\n", "other.py": "def other(): return 1\n"}, nil)
	opt := checkOptions(f)
	files := []string{f.path, filepath.Join(f.root, "other.py")}
	checks, err := mutants.CheckCoverage(t.Context(), files, opt, map[string]bool{"go": true}, false, false)
	if err != nil || len(checks) != 2 || checks[0].Function != "example.test/board#Empty" || checks[0].State != mutants.Missing {
		t.Fatalf("zero-site Go inventory: %+v %v", checks, err)
	}
	f.inputs.Files[filepath.Join(f.root, "go.work")] = "go 1.27\nuse .\n"
	checks, err = mutants.CheckCoverage(t.Context(), files, opt, map[string]bool{"go": true}, false, false)
	if err != nil || len(checks) != 2 || checks[0].State != "unsupported" || checks[1].State != "unsupported" {
		t.Fatalf("unsupported inventory: %+v %v", checks, err)
	}
	f.snapshots.Failures = map[string]snapport.Failure{"board.go": &snapport.Unreadable{Reason: "actual unreadable evidence"}}
	_, err = mutants.CheckCoverage(t.Context(), files, opt, map[string]bool{"go": true}, false, false)
	var read mutants.SnapshotReadFailure
	if !errors.As(err, &read) || read.Key != "board.go" {
		t.Fatalf("unsupported must still read before current inputs: %v", err)
	}
}

func TestReadOperationsRejectMissingBindingsAndKeepCallerCause(t *testing.T) {
	for _, binding := range []string{"reader", "tree", "surroundings", "tools", "fingerprints"} {
		t.Run(binding, func(t *testing.T) {
			f := newRunFixture(t)
			opt := checkOptions(f)
			switch binding {
			case "reader":
				opt.Store = nil
			case "tree":
				opt.Tree = nil
			case "surroundings":
				opt.Surroundings = nil
			case "tools":
				opt.Toolchain = nil
			case "fingerprints":
				opt.Fingerprints = nil
			}
			if _, err := mutants.Check(t.Context(), []string{f.path}, opt); err == nil {
				t.Fatal("missing binding accepted")
			}
			if _, err := mutants.CheckCoverage(t.Context(), []string{f.path}, opt, map[string]bool{"go": true}, false, false); err == nil {
				t.Fatal("missing strict binding accepted")
			}
		})
	}
	f := newRunFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("actual read caller stopped")
	cancel(cause)
	if _, err := mutants.Check(ctx, []string{f.path}, checkOptions(f)); !errors.Is(err, cause) {
		t.Fatalf("check cause: %v", err)
	}
	if _, err := mutants.CheckCoverage(ctx, []string{f.path}, checkOptions(f), nil, false, false); !errors.Is(err, cause) {
		t.Fatalf("inventory cause: %v", err)
	}
	if _, err := mutants.List(ctx, []string{f.path}, f.options.Tree, f.inputs, f.root); !errors.Is(err, cause) {
		t.Fatalf("list cause: %v", err)
	}
}

func TestListKeepsOrderedPrefixWhenALaterSourceCannotBeRead(t *testing.T) {
	f := newRunFixture(t)
	sites, err := mutants.List(t.Context(), []string{f.path, filepath.Join(f.root, "missing.go")}, f.options.Tree, f.inputs, f.root)
	if err == nil || len(sites) < 2 {
		t.Fatalf("partial list lost: %+v %v", sites, err)
	}
	for i, site := range sites {
		if site.File != "board.go" || site.Function != "example.test/board#Value" {
			t.Fatalf("actual site identity: %+v", site)
		}
		if i > 0 && mutants.LineOrder(sites[i-1].Site, site.Site) > 0 {
			t.Fatal("site order changed")
		}
	}
	if _, err := mutants.List(t.Context(), []string{f.path}, nil, f.inputs, f.root); err == nil {
		t.Fatal("missing tree accepted")
	}
}

var _ mutants.MutationSnapshotReader = (*porttest.Snapshots)(nil)
