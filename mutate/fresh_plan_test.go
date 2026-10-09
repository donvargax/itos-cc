package mutate

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestFreshPlanUsesCommittedInputs(t *testing.T) {
	repo := freshFixture(t, map[string]string{
		"src/a.go":            "package a\nfunc Value(x int) int { return x + 1 }\n",
		"testdata/fixture.go": "package fixture\nfunc TestValue(x int) int { return x - 1 }\n",
		"itos-cc.yaml":        "mutation: {}\n",
	})
	commit := freshGit(t, repo, "rev-parse", "HEAD")
	writeFresh(t, filepath.Join(repo, "src/a.go"), "package a\nfunc Value(x int) int { return x * 1 }\n")
	freshGit(t, repo, "add", "src/a.go") // staged changes are also excluded
	writeFresh(t, filepath.Join(repo, "src/untracked.go"), "package a\nfunc Untracked(x int) int { return x / 1 }\n")
	cache := filepath.Join(repo, ".metrics/mutate/snapshot.json")
	writeFresh(t, cache, `{"outcome":"killed"}`)

	plan, err := PlanFresh(repo, nil, "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	frozen := plan.FrozenRoot
	defer plan.Close()
	if plan.Commit != commit || plan.Seed != commit {
		t.Fatalf("commit/seed = %q/%q, want %q", plan.Commit, plan.Seed, commit)
	}
	if plan.Algorithm != FreshPlanVersion {
		t.Fatalf("algorithm = %q", plan.Algorithm)
	}
	if len(plan.Eligible) == 0 {
		t.Fatal("expected static candidates")
	}
	for _, candidate := range plan.Eligible {
		if candidate.Path == "src/untracked.go" || candidate.Path != "src/a.go" && candidate.Path != "testdata/fixture.go" {
			t.Errorf("candidate escaped committed selected inputs: %+v", candidate)
		}
	}
	frozenSource, err := os.ReadFile(filepath.Join(frozen, "src/a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(frozenSource), "x * 1") || !strings.Contains(string(frozenSource), "x + 1") {
		t.Fatalf("frozen tree did not contain committed blob: %s", frozenSource)
	}
	if _, err := os.Stat(filepath.Join(frozen, "src/untracked.go")); !os.IsNotExist(err) {
		t.Fatalf("untracked source was exported: err=%v", err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("planner changed the raw mutation cache: %v", err)
	}
	if err := plan.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frozen); !os.IsNotExist(err) {
		t.Fatalf("private frozen tree was not cleaned up: err=%v", err)
	}
}

func TestFreshPlanSelectsGloballyWithoutMutationCache(t *testing.T) {
	repo := freshFixture(t, map[string]string{
		"one/a.go": "package a\nfunc Same(x int) int { return x + 1 }\n",
		"two/a.go": "package a\nfunc Same(x int) int { return x - 1 }\n",
	})
	if err := os.MkdirAll(filepath.Join(repo, ".metrics/mutate"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFresh(t, filepath.Join(repo, ".metrics/mutate/a.json"), `{"functions":[{"outcome":"killed"}]}`)
	writeFresh(t, filepath.Join(repo, ".metrics/mutate/b.json"), `{"functions":[{"outcome":"survived"}]}`)
	plan, err := PlanFresh(repo, nil, "", 2, "global-budget")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if len(plan.Selected) != 2 || len(plan.Eligible) < 4 || len(plan.Omitted) != len(plan.Eligible)-2 {
		t.Fatalf("eligible/selected/omitted = %d/%d/%d", len(plan.Eligible), len(plan.Selected), len(plan.Omitted))
	}
	for _, selected := range plan.Selected {
		if !slices.ContainsFunc(plan.Eligible, func(c FreshCandidate) bool { return c.Identity == selected.Identity }) {
			t.Errorf("selection is not from static eligible set: %+v", selected)
		}
	}
	if reflect.DeepEqual(plan.Selected[0].Identity, plan.Selected[1].Identity) {
		t.Fatal("duplicate selected identity")
	}
}

func TestFreshPlanSeedAndUnitIdentityAreStable(t *testing.T) {
	repo := freshFixture(t, map[string]string{
		"a.go": "package a\nfunc Same(x int) int { return x + 1 }\nfunc Same(y int) int { return y + 2 }\n",
		"b.go": "package b\nfunc Same(x int) int { return x + 1 }\n",
	})
	commit := freshGit(t, repo, "rev-parse", "HEAD")
	first, err := PlanFresh(repo, []string{"b.go", "a.go"}, "", 100, "fixed text seed")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := PlanFresh(repo, []string{"a.go", "b.go"}, "", 100, "fixed text seed")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got := identities(first.Eligible); !reflect.DeepEqual(got, identities(second.Eligible)) {
		t.Fatalf("input path order changed plan: %v != %v", got, identities(second.Eligible))
	}
	unitIDs := map[string]bool{}
	for _, c := range first.Eligible {
		unitIDs[c.Path+":"+c.UnitIdentity] = true
	}
	if len(unitIDs) < 3 {
		t.Fatalf("same-name units were not disambiguated: %v", unitIDs)
	}
	defaultSeed, err := PlanFresh(repo, nil, "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	defer defaultSeed.Close()
	if defaultSeed.Seed != commit {
		t.Fatalf("default seed = %q, want resolved commit %q", defaultSeed.Seed, commit)
	}
	otherSeed, err := PlanFresh(repo, nil, "", 100, "another text")
	if err != nil {
		t.Fatal(err)
	}
	defer otherSeed.Close()
	if reflect.DeepEqual(identities(defaultSeed.Eligible), identities(otherSeed.Eligible)) {
		t.Fatal("eligible inventory should be seed-independent")
	}
	if reflect.DeepEqual(ranks(defaultSeed.Selected), ranks(otherSeed.Selected)) {
		t.Fatal("different TEXT seeds unexpectedly produced identical full rank sequence")
	}
	tied := []FreshCandidate{{Rank: "same", Identity: "b"}, {Rank: "same", Identity: "a"}}
	sortFreshCandidates(tied)
	if tied[0].Identity != "a" || tied[1].Identity != "b" {
		t.Fatalf("rank ties were not broken by identity: %+v", tied)
	}
}

func TestFreshPlanSinceAndPathNarrowingUseCommittedHead(t *testing.T) {
	repo := freshFixture(t, map[string]string{
		"old.go":   "package a\nfunc Changed(x int) int { return x + 1 }\nfunc Untouched(y int) int { return y + 2 }\n",
		"other.go": "package b\nfunc Other(x int) int { return x + 3 }\n",
	})
	base := freshGit(t, repo, "rev-parse", "HEAD")
	if err := os.Rename(filepath.Join(repo, "old.go"), filepath.Join(repo, "renamed.go")); err != nil {
		t.Fatal(err)
	}
	writeFresh(t, filepath.Join(repo, "renamed.go"), "package a\nfunc Changed(x int) int { return x * 1 }\nfunc Untouched(y int) int { return y + 2 }\n")
	freshGit(t, repo, "add", "-A")
	freshGit(t, repo, "commit", "-m", "rename and change")
	plan, err := PlanFresh(repo, nil, base, 100, "since")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	for _, c := range plan.Eligible {
		if c.Path != "renamed.go" || !strings.HasPrefix(c.Function, "a#Changed") {
			t.Errorf("--since selected nonchanged committed unit: %s %s", c.Path, c.Function)
		}
	}
	narrow, err := PlanFresh(repo, []string{"other.go"}, base, 100, "since")
	if err != nil {
		t.Fatal(err)
	}
	defer narrow.Close()
	if len(narrow.Eligible) != 0 {
		t.Fatalf("explicit path narrowing failed, got %v", identities(narrow.Eligible))
	}
}

func TestFreshPlanRejectsUnsafeAndUnsupportedInputs(t *testing.T) {
	t.Run("path escape", func(t *testing.T) {
		repo := freshFixture(t, map[string]string{"a.go": "package a\nfunc A() int { return 1 }\n"})
		if _, err := PlanFresh(repo, []string{"../outside.go"}, "", 1, ""); err == nil {
			t.Fatal("expected escaping path rejection")
		}
	})
	t.Run("unresolved path", func(t *testing.T) {
		repo := freshFixture(t, map[string]string{"a.go": "package a\nfunc A() int { return 1 }\n"})
		if _, err := PlanFresh(repo, []string{"missing.go"}, "", 1, ""); err == nil || !strings.Contains(err.Error(), "no committed supported source") {
			t.Fatalf("expected unresolved source path rejection, got %v", err)
		}
	})
	t.Run("committed symlink", func(t *testing.T) {
		repo := freshFixture(t, map[string]string{"a.go": "package a\nfunc A() int { return 1 }\n"})
		outside := filepath.Join(t.TempDir(), "outside.go")
		writeFresh(t, outside, "package a\nfunc Outside() int { return 1 }\n")
		if err := os.Symlink(outside, filepath.Join(repo, "link.go")); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		freshGit(t, repo, "add", "link.go")
		freshGit(t, repo, "commit", "-m", "symlink")
		if _, err := PlanFresh(repo, nil, "", 1, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected clear symlink refusal, got %v", err)
		}
		if entries, err := os.ReadDir(filepath.Join(repo, ".git", "itos")); err != nil || len(entries) != 0 {
			t.Fatalf("failed plan leaked private scratch data: entries=%v err=%v", entries, err)
		}
		contents, err := os.ReadFile(outside)
		if err != nil || !strings.Contains(string(contents), "Outside") {
			t.Fatalf("external source was changed/read through link: %v", err)
		}
	})
	t.Run("private scratch symlink", func(t *testing.T) {
		repo := freshFixture(t, map[string]string{"a.go": "package a\nfunc A() int { return 1 }\n"})
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(repo, ".git", "itos")); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		if _, err := PlanFresh(repo, nil, "", 1, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected private scratch symlink refusal, got %v", err)
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatalf("planner wrote through scratch symlink: entries=%v err=%v", entries, err)
		}
	})
	t.Run("submodule", func(t *testing.T) {
		repo := freshFixture(t, map[string]string{"a.go": "package a\nfunc A() int { return 1 }\n"})
		nested := filepath.Join(repo, "vendor", "dep")
		if err := os.MkdirAll(nested, 0700); err != nil {
			t.Fatal(err)
		}
		freshGit(t, nested, "init", "-q")
		freshGit(t, nested, "config", "user.email", "fresh-plan@example.invalid")
		freshGit(t, nested, "config", "user.name", "Fresh Plan Test")
		writeFresh(t, filepath.Join(nested, "dep.go"), "package dep\n")
		freshGit(t, nested, "add", ".")
		freshGit(t, nested, "commit", "-q", "-m", "nested fixture")
		freshGit(t, repo, "add", "vendor/dep")
		freshGit(t, repo, "commit", "-m", "submodule")
		if _, err := PlanFresh(repo, nil, "", 1, ""); err == nil || !strings.Contains(err.Error(), "submodule") {
			t.Fatalf("expected clear submodule refusal, got %v", err)
		}
	})
	t.Run("invalid count", func(t *testing.T) {
		if _, err := PlanFresh(t.TempDir(), nil, "", 0, ""); err == nil || !strings.Contains(err.Error(), "positive") {
			t.Fatalf("expected positive count validation, got %v", err)
		}
	})
}

func identities(candidates []FreshCandidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.Identity
	}
	return out
}

func ranks(candidates []FreshCandidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.Rank
	}
	return out
}

func freshFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	freshGit(t, repo, "init", "-q")
	freshGit(t, repo, "config", "user.email", "fresh-plan@example.invalid")
	freshGit(t, repo, "config", "user.name", "Fresh Plan Test")
	for name, contents := range files {
		writeFresh(t, filepath.Join(repo, filepath.FromSlash(name)), contents)
	}
	freshGit(t, repo, "add", "--", ".")
	freshGit(t, repo, "commit", "-q", "-m", "fixture")
	return repo
}

func freshGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFresh(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}
