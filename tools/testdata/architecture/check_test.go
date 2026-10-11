package architecture_test

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures are independent packages in a temporary module, not broken
// mutations of the counting example. Every package/test compiles before lint.
// Local replacement modules supply mock/Rapid symbols without new dependencies.
// They hold the import boundaries (depguard) and the exit-code switches over
// a sealed failure set (gochecksumtype).
func TestArchitecture(t *testing.T) {
	root := os.Getenv("ARCHITECTURE_ROOT")
	linter := os.Getenv("ARCHITECTURE_LINTER")
	if root == "" || linter == "" {
		t.Fatal("run tools/bin/architecture-check")
	}
	dir, err := os.MkdirTemp("", "T-9-architecture-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	moduleBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := strings.Fields(string(moduleBytes))[1]
	write := func(path, contents string) {
		t.Helper()
		if strings.HasSuffix(path, ".go") {
			formatted, err := format.Source([]byte(contents))
			if err != nil {
				t.Fatalf("format %s: %v", path, err)
			}
			contents = string(formatted)
		}
		path = filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Decision 29's domain and its port subpackages, the adapters, the count
	// slice and internal/next, a made project's next slice that no rule
	// names, the UI, the packages that are no command, and cmd.
	for _, path := range []string{"internal/counting", "internal/counting/port", "internal/counting/port/porttest", "internal/disk", "internal/stdin", "internal/count", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe"} {
		write(path+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	mocks := []string{"github.com/golang/mock/gomock", "go.uber.org/mock/gomock", "github.com/stretchr/testify/mock", "github.com/vektra/mockery/v2", "github.com/maxbrunsfeld/counterfeiter/v6"}
	mod := "module " + module + "\n\ngo 1.27\n"
	const kong = "github.com/alecthomas/kong"
	for i, path := range append(mocks, "pgregory.net/rapid", kong) {
		stub := fmt.Sprintf("stubs/library%d", i)
		version := "v0.0.0"
		if strings.HasSuffix(path, "/v2") {
			version = "v2.0.0"
		}
		if strings.HasSuffix(path, "/v6") {
			version = "v6.0.0"
		}
		mod += fmt.Sprintf("\nrequire %s %s\nreplace %s => ./%s\n", path, version, path, stub)
		write(stub+"/go.mod", "module "+path+"\n\ngo 1.27\n")
		write(stub+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	write("go.mod", mod)
	// A refused fixture names the linter refusing it and what its
	// diagnostic must say; an accepted one names neither.
	type expectation struct {
		path, linter string
		texts        []string
	}
	var cases []expectation
	add := func(name, layer, imported, symbol, rule string, test bool) {
		path := layer + "/" + name + "/probe.go"
		body := "var Probe = dependency." + symbol + "\n"
		imports := fmt.Sprintf("import dependency %q\n", imported)
		if test {
			path = layer + "/" + name + "/probe_test.go"
			imports = fmt.Sprintf("import (\n dependency %q\n \"testing\"\n)\n", imported)
			body = "func TestProbe(t *testing.T) {\n _ = dependency." + symbol + "\n}\n"
			if imported == "os" && rule == "" {
				body = "func TestProbe(t *testing.T) {\n path := t.TempDir() + \"/fixture\"\n if err := dependency.WriteFile(path, []byte(\"fixture\"), 0o600); err != nil { t.Fatal(err) }\n data, err := dependency.ReadFile(path)\n if err != nil || string(data) != \"fixture\" { t.Fatalf(\"fixture: %q, %v\", data, err) }\n}\n"
			}
		}
		// Rapid's permitted case must be an immediate domain test, just as
		// the branch configuration (and its existing properties) require.
		if name == "rapid_allowed" {
			path = layer + "/" + name + "_test.go"
		}
		write(path, "package probe\n\n"+imports+"\n"+body)
		refusal := expectation{path: path}
		if rule != "" {
			refusal = expectation{path, "depguard", []string{imported, "from list '" + rule + "'"}}
		}
		cases = append(cases, refusal)
	}
	// The domain arrow: the domain and its ports, code and tests, import
	// nothing of ours but the domain and its port subpackages. The kong
	// rule keeps kong out of them, below.
	for _, base := range []string{"internal/counting", "internal/counting/port", "internal/counting/port/porttest"} {
		for _, test := range []bool{false, true} {
			kind := "code"
			if test {
				kind = "test"
			}
			for _, target := range []string{"internal/disk", "internal/stdin", "internal/count", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe"} {
				add(strings.ReplaceAll(target, "/", "_")+"_"+kind, base, module+"/"+target, "Value", "domain", test)
			}
			add("domain_allowed_"+kind, base, module+"/internal/counting", "Value", "", test)
			add("port_allowed_"+kind, base, module+"/internal/counting/port", "Value", "", test)
		}
		// The domain-io arrow: no I/O in the domain's own code; its tests
		// may read fixtures and use porttest's fakes.
		add("os_code", base, "os", "ReadFile", "domain-io", false)
		add("exec_code", base, "os/exec", "Command", "domain-io", false)
		add("net_code", base, "net", "Dial", "domain-io", false)
		add("http_code", base, "net/http", "Get", "domain-io", false)
		add("fixture_io_allowed", base, "os", "ReadFile", "", true)
		add("fake_allowed", base, module+"/internal/counting/port/porttest", "Value", "", true)
	}
	// The adapter arrow: an adapter's code imports nothing of ours but the
	// port, not the domain, another adapter, a slice, the UI, cmd or the
	// fakes; it may do I/O, and its tests may use the fakes.
	for _, base := range []string{"internal/disk", "internal/stdin"} {
		for _, target := range []string{"internal/counting", "internal/disk", "internal/stdin", "internal/count", "internal/next", "internal/counting/port/porttest", "internal/cli", "internal/version", "internal/release", "cmd/probe"} {
			add(strings.ReplaceAll(target, "/", "_"), base, module+"/"+target, "Value", "infra", false)
		}
		add("port_allowed", base, module+"/internal/counting/port", "Value", "", false)
		add("fake_allowed", base, module+"/internal/counting/port/porttest", "Value", "", true)
		add("os_allowed", base, "os", "ReadFile", "", false)
		add("fixture_io_allowed", base, "os", "ReadFile", "", true)
	}
	// The slice arrow, code and tests alike: a slice imports the domain, its
	// ports and internal/cli, never another slice, an adapter, a package
	// that is no command or cmd. internal/next is held as count is with no
	// rule naming it, as a made project's next slice is.
	for _, slice := range []struct{ base, other string }{{"internal/count", "internal/next"}, {"internal/next", "internal/count"}} {
		for _, test := range []bool{false, true} {
			kind := "code"
			if test {
				kind = "test"
			}
			for _, target := range []string{slice.other, "internal/disk", "internal/stdin", "internal/version", "internal/release", "cmd/probe"} {
				add(strings.ReplaceAll(target, "/", "_")+"_"+kind, slice.base, module+"/"+target, "Value", "slices", test)
			}
			for _, target := range []string{"internal/counting", "internal/counting/port", "internal/cli"} {
				add(strings.ReplaceAll(target, "/", "_")+"_allowed_"+kind, slice.base, module+"/"+target, "Value", "", test)
			}
		}
		add("fake_allowed", slice.base, module+"/internal/counting/port/porttest", "Value", "", true)
	}
	// What is no slice is not held as one: the release library's packages
	// import each other, as releasetest does, and the UI imports the ports.
	add("release_allowed", "internal/release", module+"/internal/release", "Value", "", false)
	add("version_allowed", "internal/version", module+"/internal/version", "Value", "", false)
	add("port_allowed", "internal/cli", module+"/internal/counting/port", "Value", "", false)
	// What assembles them is free to import the domain, the adapters and
	// the slices.
	for _, target := range []string{"internal/counting", "internal/disk", "internal/stdin", "internal/count", "internal/next", "internal/cli", "internal/version"} {
		add(strings.ReplaceAll(target, "/", "_")+"_allowed", "cmd/probe", module+"/"+target, "Value", "", false)
	}
	for i, imported := range mocks {
		for _, test := range []bool{false, true} {
			add(fmt.Sprintf("mock%d_%t", i, test), "internal/next", imported, "Value", "mocks", test)
		}
	}
	add("rapid_allowed", "internal/counting", "pgregory.net/rapid", "Value", "", true)
	// kong is the UI's: the entry point and internal/cli, nowhere else.
	for _, test := range []bool{false, true} {
		for _, layer := range []string{"cmd/probe", "internal/cli"} {
			add(fmt.Sprintf("kong_allowed_%t", test), layer, kong, "Value", "", test)
		}
		for _, layer := range []string{"internal/count", "internal/next", "internal/counting", "internal/counting/port", "internal/counting/port/porttest", "internal/disk", "internal/stdin", "internal/version", "internal/release"} {
			add(fmt.Sprintf("kong_%t", test), layer, kong, "Value", "kong", test)
		}
	}
	// Rapid only in the domain package's own tests, not its code, a
	// subpackage's tests (its ports and fakes) or anything else.
	for _, layer := range []string{"internal/counting", "internal/counting/port", "internal/counting/port/porttest", "internal/disk", "internal/stdin", "internal/count", "internal/next"} {
		add("rapid_code", layer, "pgregory.net/rapid", "Value", "rapid", false)
		add("rapid_test", layer, "pgregory.net/rapid", "Value", "rapid", true)
	}
	// A sealed failure set as the ports declare one, and internal/cli's
	// switches over it shaped as readProblem is: starting from the internal
	// error's 70, each kind its code. Only the switch naming every kind
	// passes; a default arm does not count as naming the kind it leaves out.
	sealed := module + "/internal/counting/port/sealed"
	write("internal/counting/port/sealed/sealed.go", `package sealed

// Failure is a sealed set of two kinds.
//
//sumtype:decl
type Failure interface {
	error
	failure()
}

// Missing is one kind.
type Missing struct{ Path string }

func (*Missing) failure() {}
func (e *Missing) Error() string { return "missing: " + e.Path }

// Unreadable is the other.
type Unreadable struct{ Path string }

func (*Unreadable) failure() {}
func (e *Unreadable) Error() string { return "unreadable: " + e.Path }
`)
	sumtype := func(name, arms string, refused bool) {
		path := "internal/cli/" + name + "/probe.go"
		write(path, fmt.Sprintf(`package probe

import sealed %q

// Code is failure's exit code.
func Code(failure sealed.Failure) int {
	code := 70
	switch failure.(type) {
%s	}
	return code
}
`, sealed, arms))
		refusal := expectation{path: path}
		if refused {
			refusal = expectation{path, "gochecksumtype", []string{"Failure", "missing cases for Unreadable"}}
		}
		cases = append(cases, refusal)
	}
	missing := "case *sealed.Missing:\n code = 3\n"
	sumtype("sumtype_allowed", missing+"case *sealed.Unreadable:\n code = 4\n", false)
	sumtype("sumtype_omitted", missing, true)
	sumtype("sumtype_default", missing+"default:\n", true)
	compile := exec.Command("go", "test", "./...")
	compile.Dir = dir
	compile.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("fixtures must compile and their tests pass before lint: %v\n%s", err, output)
	}
	t.Logf("all %d independent architecture fixtures compiled; fixture-I/O tests passed", len(cases))
	outputPath := filepath.Join(dir, "issues.json")
	// Absolute output paths avoid the config/fixture being on different
	// Windows drives. This changes presentation only, not any lint rule.
	lint := exec.Command(linter, "run", "--config", filepath.Join(root, ".golangci.yml"), "--output.json.path", outputPath, "--path-mode=abs", "--max-issues-per-linter=0", "--max-same-issues=0", "./...")
	lint.Dir = dir
	lint.Env = compile.Env
	output, lintErr := lint.CombinedOutput()
	if lintErr != nil {
		if exit, ok := lintErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("linter failed to run, not a refusal: %v\n%s", lintErr, output)
		}
	}
	var report struct {
		Issues []struct {
			FromLinter, Text string
			Pos              struct{ Filename string }
		}
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read actual linter JSON: %v\n%s", err, output)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.Issues {
		if !filepath.IsAbs(issue.Pos.Filename) {
			t.Fatalf("linter did not emit an absolute path: %s", issue.Pos.Filename)
		}
		// macOS's temporary directory is reached through /var -> /private/var;
		// compare the physical files, not their different lexical spellings.
		filename, err := filepath.EvalSymlinks(issue.Pos.Filename)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(canonicalDir, filename)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.ToSlash(rel)
		var expected *expectation
		for i := range cases {
			if cases[i].path == path {
				expected = &cases[i]
				break
			}
		}
		matches := expected != nil && expected.linter != "" && issue.FromLinter == expected.linter
		for i := 0; matches && i < len(expected.texts); i++ {
			matches = strings.Contains(issue.Text, expected.texts[i])
		}
		if !matches {
			t.Errorf("unexpected diagnostic (not a passed negative): %s: %s: %s", path, issue.FromLinter, issue.Text)
			continue
		}
		seen[path] = true
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if c.linter != "" && !seen[c.path] {
				t.Errorf("missing %s refusal: %s", c.linter, strings.Join(c.texts, ", "))
			}
		})
	}
}
