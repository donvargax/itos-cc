package main

import (
	"os"
	"strings"
	"testing"
)

// @ID-MUT-150
func TestBinaryOnlyEndToEndTestChangeMakesBroadScopeKillStale(t *testing.T) {
	for _, scope := range []struct {
		name string
		args []string
	}{
		{name: "all-tests", args: []string{"--all-tests"}},
		{name: "test-command", args: []string{"--test-command", "go test ./..."}},
	} {
		t.Run(scope.name, func(t *testing.T) {
			greetRepo(t, false, false)
			first := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			o := mutateCovered(t, first...)
			if o.code > 1 {
				t.Fatalf("initial broad run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
			}

			path := "e2e/e2e_test.go"
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			test := string(contents)
			test = strings.Replace(test, `got != "one"`, `got != "one" && got != "0 more"`, 1)
			writeFile(t, path, test)

			check := cli(t, "mutation", "check", "--json", greetSource)
			if check.code != 1 || !strings.Contains(check.stdout, `"rule":"mutation.stale"`) || !strings.Contains(check.stdout, path) {
				t.Fatalf("after changing binary-only test: want stale naming %s, exit 1; got exit %d\n%s%s", path, check.code, check.stdout, check.stderr)
			}

			again := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			run := mutateCovered(t, again...)
			if run.code != 0 {
				t.Fatalf("rerun after test change: exit %d\n%s%s", run.code, run.stdout, run.stderr)
			}
			if !strings.Contains(run.stdout, `"outcome":"survived"`) {
				t.Fatalf("changed test must rerun and let the mutant survive\n%s%s", run.stdout, run.stderr)
			}
		})
	}
}

// @ID-MUT-151
func TestBroadScopeSupportFilesAreHashed(t *testing.T) {
	for _, scope := range []string{"--all-tests", "--test-command"} {
		t.Run(scope, func(t *testing.T) {
			greetRepo(t, false, false)
			writeFile(t, "itos-cc.yaml", listedFiles["itos-cc.yaml"]+"    support: [\"features/*.feature\"]\n")
			writeFile(t, "features/example.feature", "Feature: first\n")
			args := []string{scope, "--no-coverage", "--json", greetSource}
			if scope == "--test-command" {
				args = []string{"--test-command", "go test ./...", "--no-coverage", "--json", greetSource}
			}
			if o := mutateCovered(t, args...); o.code > 1 {
				t.Fatalf("initial run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
			}
			writeFile(t, "features/example.feature", "Feature: changed\n")
			check := cli(t, "mutation", "check", "--json", greetSource)
			if check.code != 1 || !strings.Contains(check.stdout, `"rule":"mutation.stale"`) || !strings.Contains(check.stdout, "features/example.feature") {
				t.Fatalf("support edit: want stale naming features/example.feature, got exit %d\n%s%s", check.code, check.stdout, check.stderr)
			}
		})
	}
}

// @ID-MUT-155
func TestGoBroadScopeIncludesBuildTaggedTestsButExcludesNestedModules(t *testing.T) {
	greetRepo(t, false, false)
	if o := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource); o.code > 1 {
		t.Fatalf("initial run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeFile(t, "tagged_test.go", "//go:build freshness_test\n\npackage main\n\nimport \"testing\"\n\nfunc TestTagged(t *testing.T) {}\n")
	check := cli(t, "mutation", "check", "--json", greetSource)
	if check.code != 1 || !strings.Contains(check.stdout, "tagged_test.go") {
		t.Fatalf("build-tagged test: want stale naming tagged_test.go, got exit %d\n%s%s", check.code, check.stdout, check.stderr)
	}
}
