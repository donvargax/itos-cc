package architecture_test

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The fixtures are independent packages in a temporary module, not broken
// mutations of the module's own packages. Every package/test compiles before lint.
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
	// Every port's fake the module holds, as paths from its root: each is
	// stubbed below, and a slice's code importing it must be refused.
	listed := exec.Command("go", "list", "./...")
	listed.Dir = root
	packages, err := listed.Output()
	if err != nil {
		t.Fatalf("list the module's packages: %v", err)
	}
	var fakes []string
	for _, path := range strings.Fields(string(packages)) {
		if strings.HasSuffix(path, "/porttest") {
			fakes = append(fakes, strings.TrimPrefix(path, module+"/"))
		}
	}
	if len(fakes) == 0 {
		t.Fatal("the module lists no porttest package")
	}
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
	// Decision 29's domains and their port subpackages, the adapters, the
	// command slices and internal/next, a made project's next slice
	// that no rule names, the UI, the packages that are no command, cmd,
	// project, the module's top-level library the workdir and metricsdir
	// adapters wrap, metrics, the one metricsdir wraps beside it, and
	// coverage, the runner of each language's tools coveragetools wraps.
	// internal/lang is a domain every command measures by, its port the one
	// workdir implements beside selection's; internal/snapshot is the domain
	// of the snapshots under .metrics/, its port metricsdir's (decision 41).
	for _, path := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "project", "metrics", "coverage", "graph", "config"} {
		write(path+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	for _, fake := range fakes {
		write(fake+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	mocks := []string{"github.com/golang/mock/gomock", "go.uber.org/mock/gomock", "github.com/stretchr/testify/mock", "github.com/vektra/mockery/v2", "github.com/maxbrunsfeld/counterfeiter/v6"}
	mod := "module " + module + "\n\ngo 1.27\n"
	// kong, and the fork itos-cc builds with until kong releases its typed
	// parse errors.
	kongs := []string{"github.com/alecthomas/kong", "github.com/donvargax/kong"}
	for i, path := range append(append(mocks, "pgregory.net/rapid"), kongs...) {
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
	// The domain arrow: each domain and its ports, code and tests, import
	// nothing of ours but the domain and its port subpackages, neither an
	// adapter nor the libraries an adapter wraps. Every domain may import
	// internal/lang and its port, the measure every command shares. The kong
	// rule keeps kong out of them, below. internal/duplication and
	// internal/teststructure reach nothing outside, so they have no port:
	// they take files internal/lang parsed, their tests parsing them through
	// lang's fake, as internal/changerisk, which scores them with a coverage
	// Report, does. internal/coverage asks its port what a path a report
	// names is on disk (decision 46). config, the settings codec whose
	// Reader implements mutants' SettingsReader and aliases its Config
	// (T-97), is outside every domain too.
	domains := []string{"internal/selection", "internal/lang", "internal/snapshot", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage"}
	for _, domain := range domains {
		ported := domain != "internal/duplication" && domain != "internal/teststructure" && domain != "internal/changerisk" && domain != "internal/mutants"
		bases := []string{domain}
		if domain == "internal/mutants" || domain == "internal/coverage" {
			bases = append(bases, domain+"/porttest")
		}
		if ported {
			bases = append(bases, domain+"/port", domain+"/port/porttest")
		}
		for _, base := range bases {
			for _, test := range []bool{false, true} {
				kind := "code"
				if test {
					kind = "test"
				}
				for _, target := range []string{"internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "project", "metrics", "coverage", "config"} {
					add(strings.ReplaceAll(target, "/", "_")+"_"+kind, base, module+"/"+target, "Value", "domain", test)
				}
				add("domain_allowed_"+kind, base, module+"/"+domain, "Value", "", test)
				if ported {
					add("port_allowed_"+kind, base, module+"/"+domain+"/port", "Value", "", test)
				}
				add("lang_allowed_"+kind, base, module+"/internal/lang", "Value", "", test)
				add("lang_port_allowed_"+kind, base, module+"/internal/lang/port", "Value", "", test)
			}
			// The domain-io arrow: no I/O in the domain's own code; its tests
			// may read fixtures and use porttest's fakes.
			add("os_code", base, "os", "ReadFile", "domain-io", false)
			add("exec_code", base, "os/exec", "Command", "domain-io", false)
			add("net_code", base, "net", "Dial", "domain-io", false)
			add("http_code", base, "net/http", "Get", "domain-io", false)
			add("fixture_io_allowed", base, "os", "ReadFile", "", true)
			if ported {
				add("fake_allowed", base, module+"/"+domain+"/port/porttest", "Value", "", true)
			}
			add("lang_fake_allowed", base, module+"/internal/lang/port/porttest", "Value", "", true)
		}
	}
	add("own_fake_allowed", "internal/mutants", module+"/internal/mutants/porttest", "Value", "", true)
	add("own_fake_refused", "internal/mutants", module+"/internal/mutants/porttest", "Value", "mutant-fakes", false)
	add("own_fake_allowed", "internal/coverage", module+"/internal/coverage/porttest", "Value", "", true)
	add("own_fake_refused", "internal/coverage", module+"/internal/coverage/porttest", "Value", "coverage-fakes", false)
	add("coverage_fake_refused", "internal/mutants", module+"/internal/coverage/porttest", "Value", "coverage-fakes", false)
	// The adapter arrow: an adapter's code imports nothing of ours but the
	// port it implements, not a domain, another adapter, a slice, the UI,
	// cmd, the fakes, another adapter's port or a top-level library it does
	// not wrap; it may do I/O, and its tests may use the fakes. workdir wraps
	// project and implements internal/lang's port too, under a rule of its own;
	// metricsdir wraps metrics and project for the snapshot port, under its own.
	// coveragetools wraps the coverage runner and metrics for the coverage
	// port, under its own, and imports the coverage domain it serves, whose
	// Report it returns (decision 47): allowed lists what an adapter's rule
	// lets it import beside its port.
	for _, adapter := range []struct {
		base, domain, rule string
		others, allowed    []string
	}{
		{"internal/workdir", "internal/selection", "workdir", []string{"internal/snapshot/port", "internal/coverage/port", "metrics", "coverage"}, []string{"internal/mutants"}},
		{"internal/metricsdir", "internal/snapshot", "metricsdir", []string{"internal/selection/port", "internal/lang/port", "internal/coverage/port", "coverage"}, []string{"internal/mutants"}},

		{"internal/testtools", "internal/mutants", "testtools", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "metrics", "graph"}, []string{"internal/mutants", "internal/lang", "project", "coverage"}},
		{"internal/testinputs", "internal/mutants", "testinputs", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "metrics", "graph"}, []string{"internal/mutants", "internal/lang", "project", "coverage"}},
		{"internal/freshenv", "internal/mutants", "freshenv", []string{"internal/snapshot/port", "metrics", "graph"}, []string{"internal/mutants", "internal/coverage", "internal/coverage/port", "internal/selection", "internal/selection/port", "internal/lang", "internal/lang/port", "config", "project", "coverage"}},
		{"internal/githistory", "internal/mutants", "githistory", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "metrics", "coverage", "graph"}, []string{"internal/mutants", "project"}},
		{"internal/importgraph", "internal/mutants", "importgraph", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "metrics", "coverage"}, []string{"internal/mutants", "project", "graph"}},
		{"internal/worktrees", "internal/mutants", "worktrees", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "metrics", "coverage"}, []string{"internal/mutants", "project"}},
		{"internal/coveragetools", "internal/coverage", "coveragetools", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "project"},
			[]string{"internal/coverage", "coverage", "metrics"}},
	} {
		base := adapter.base
		for _, target := range append([]string{"internal/selection", "internal/snapshot", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/selection/port/porttest", "internal/snapshot/port/porttest", "internal/lang", "internal/lang/port/porttest", "internal/cli", "internal/version", "internal/release", "cmd/probe"}, adapter.others...) {
			if slices.Contains(adapter.allowed, target) {
				continue
			}
			add(strings.ReplaceAll(target, "/", "_"), base, module+"/"+target, "Value", adapter.rule, false)
		}
		for _, target := range adapter.allowed {
			add(strings.ReplaceAll(target, "/", "_")+"_allowed", base, module+"/"+target, "Value", "", false)
		}
		if adapter.domain == "internal/mutants" {
			add("root_contract_allowed", base, module+"/internal/mutants", "Value", "", false)
			add("fake_allowed", base, module+"/internal/mutants/porttest", "Value", "", true)
			add("fake_refused", base, module+"/internal/mutants/porttest", "Value", adapter.rule, false)
		} else {
			add("port_allowed", base, module+"/"+adapter.domain+"/port", "Value", "", false)
			add("fake_allowed", base, module+"/"+adapter.domain+"/port/porttest", "Value", "", true)
		}
		add("os_allowed", base, "os", "ReadFile", "", false)
		add("fixture_io_allowed", base, "os", "ReadFile", "", true)
	}
	add("project_allowed", "internal/workdir", module+"/project", "Value", "", false)
	add("lang_port_allowed", "internal/workdir", module+"/internal/lang/port", "Value", "", false)
	add("lang_fake_allowed", "internal/workdir", module+"/internal/lang/port/porttest", "Value", "", true)
	add("project_allowed", "internal/metricsdir", module+"/project", "Value", "", false)
	add("metrics_allowed", "internal/metricsdir", module+"/metrics", "Value", "", false)
	// The slice arrow, code and tests alike: a slice imports the domain, its
	// ports, internal/cli and internal/lang with its port, never another slice, an adapter, the
	// libraries an adapter wraps, a package that is no command or cmd. The
	// snapshot domain is a domain like the others, its port a slice's to hold.
	// internal/units, internal/dry, internal/scrap, internal/crap and internal/next are held with no rule
	// naming them, as a made project's next slice is. internal/mutation reads
	// settings through mutants' SettingsReader, never the config codec (T-97).
	for _, slice := range []struct{ base, other string }{{"internal/next", "internal/units"}, {"internal/units", "internal/next"}, {"internal/dry", "internal/units"}, {"internal/scrap", "internal/dry"}, {"internal/crap", "internal/scrap"}, {"internal/mutation", "internal/units"}, {"internal/units", "internal/mutation"}} {
		for _, test := range []bool{false, true} {
			kind := "code"
			if test {
				kind = "test"
			}
			for _, target := range []string{slice.other, "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/version", "internal/release", "cmd/probe", "project", "metrics", "coverage", "config"} {
				add(strings.ReplaceAll(target, "/", "_")+"_"+kind, slice.base, module+"/"+target, "Value", "slices", test)
			}
			for _, target := range []string{"internal/selection", "internal/selection/port", "internal/snapshot", "internal/snapshot/port", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port", "internal/cli", "internal/lang", "internal/lang/port"} {
				add(strings.ReplaceAll(target, "/", "_")+"_allowed_"+kind, slice.base, module+"/"+target, "Value", "", test)
			}
		}
		add("selection_fake_allowed", slice.base, module+"/internal/selection/port/porttest", "Value", "", true)
		add("lang_fake_allowed", slice.base, module+"/internal/lang/port/porttest", "Value", "", true)
		add("snapshot_fake_allowed", slice.base, module+"/internal/snapshot/port/porttest", "Value", "", true)
		add("coverage_fake_allowed", slice.base, module+"/internal/coverage/port/porttest", "Value", "", true)
		// A slice's own code links no fake (T-56): every porttest of the
		// module, as go list finds them, so a new port's fake that the
		// slice-fakes rule does not name fails here.
		for _, fake := range fakes {
			add(strings.ReplaceAll(fake, "/", "_")+"_code", slice.base, module+"/"+fake, "Value", "slice-fakes", false)
		}
	}
	// What is no slice is not held as one: the release library's packages
	// import each other, as releasetest does.
	add("release_allowed", "internal/release", module+"/internal/release", "Value", "", false)
	add("version_allowed", "internal/version", module+"/internal/version", "Value", "", false)
	// The UI arrow, code and tests alike: internal/cli imports nothing of
	// ours, no domain or port above all, since cmd hands it each domain's
	// classifier; the standard library is its own.
	for _, test := range []bool{false, true} {
		kind := "code"
		if test {
			kind = "test"
		}
		for _, target := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/version", "internal/release", "cmd/probe"} {
			add(strings.ReplaceAll(target, "/", "_")+"_"+kind, "internal/cli", module+"/"+target, "Value", "ui", test)
		}
		add("stdlib_allowed_"+kind, "internal/cli", "strings", "ToLower", "", test)
	}
	// What assembles them is free to import the domain, the adapters and
	// the slices.
	for _, target := range []string{"internal/selection", "internal/lang", "internal/snapshot", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/cli", "internal/version", "project", "metrics", "coverage"} {
		add(strings.ReplaceAll(target, "/", "_")+"_allowed", "cmd/probe", module+"/"+target, "Value", "", false)
	}
	for i, imported := range mocks {
		for _, test := range []bool{false, true} {
			add(fmt.Sprintf("mock%d_%t", i, test), "internal/next", imported, "Value", "mocks", test)
		}
	}
	// internal/lang's, internal/snapshot's, internal/duplication's,
	// internal/teststructure's, internal/changerisk's and
	// internal/coverage's tests have no property, so rapid is refused
	// there.
	for _, domain := range []string{"internal/selection"} {
		add("rapid_allowed", domain, "pgregory.net/rapid", "Value", "", true)
	}
	// kong is the UI's: the entry point and internal/cli, nowhere else.
	for i, kong := range kongs {
		for _, test := range []bool{false, true} {
			for _, layer := range []string{"cmd/probe", "internal/cli"} {
				add(fmt.Sprintf("kong%d_allowed_%t", i, test), layer, kong, "Value", "", test)
			}
			for _, layer := range []string{"internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next", "internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/version", "internal/release"} {
				add(fmt.Sprintf("kong%d_%t", i, test), layer, kong, "Value", "kong", test)
			}
		}
	}
	// Rapid only in the domain package's own tests, not its code, a
	// subpackage's tests (its ports and fakes) or anything else.
	for _, layer := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/duplication", "internal/teststructure", "internal/changerisk", "internal/mutants", "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/worktrees", "internal/testtools", "internal/testinputs", "internal/importgraph", "internal/githistory", "internal/freshenv", "internal/dry", "internal/scrap", "internal/crap", "internal/units", "internal/mutation", "internal/next"} {
		add("rapid_code", layer, "pgregory.net/rapid", "Value", "rapid", false)
		add("rapid_test", layer, "pgregory.net/rapid", "Value", "rapid", true)
	}
	// A sealed failure set as the ports declare one, and a slice's
	// classifier switching over it shaped as cmd's classifiers are:
	// starting from the internal error's 70, each kind its code. Only the switch naming every kind
	// passes; a default arm does not count as naming the kind it leaves out.
	sealed := module + "/internal/selection/port/sealed"
	write("internal/selection/port/sealed/sealed.go", `package sealed

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
		path := "internal/next/" + name + "/probe.go"
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
	// Decision 30: a context is the first parameter, never a struct's field,
	// in code and tests alike. containedctx refuses the field and revive's
	// context-as-argument a context after another parameter; a context
	// first, named or blank as lang's Surroundings takes it, passes. The
	// stubs' exported names have no comments, so revive's default rules,
	// which would ask for them, are shown to stay off.
	contexts := func(name, body, linter, text string, test bool) {
		path := "internal/next/" + name + "/probe.go"
		imports := "import \"context\"\n"
		if test {
			path = "internal/next/" + name + "/probe_test.go"
			imports = "import (\n \"context\"\n \"testing\"\n)\n"
			body += "func TestProbe(t *testing.T) {\n _ = t\n _ = Probe\n}\n"
		}
		write(path, "package probe\n\n"+imports+"\n"+body)
		refusal := expectation{path: path}
		if linter != "" {
			refusal = expectation{path, linter, []string{text}}
		}
		cases = append(cases, refusal)
	}
	held := "type Holder struct {\n Ctx context.Context\n}\n\nfunc Probe(ctx context.Context, n int) Holder {\n _ = n\n return Holder{Ctx: ctx}\n}\n"
	second := "func Probe(n int, ctx context.Context) error {\n _ = n\n return ctx.Err()\n}\n"
	first := "func Probe(ctx context.Context, n int) error {\n _ = n\n return ctx.Err()\n}\n"
	blank := "func Probe(_ context.Context, n int) int {\n return n\n}\n"
	for _, test := range []bool{false, true} {
		kind := "code"
		if test {
			kind = "test"
		}
		contexts("context_held_"+kind, held, "containedctx", "contains a context.Context field", test)
		contexts("context_second_"+kind, second, "revive", "context.Context should be the first parameter", test)
		contexts("context_first_allowed_"+kind, first, "", "", test)
		contexts("context_blank_allowed_"+kind, blank, "", "", test)
	}
	// itos-cc's binary links no fake (T-56), which depguard cannot hold,
	// seeing only the imports a file names: architecture-check --linked refuses a
	// command linking a porttest, naming it, and passes one whose fake enters
	// only through a _test.go file.
	fake := module + "/" + fakes[0]
	write("cmd/linked/main.go", fmt.Sprintf("package main\n\nimport _ %q\n\nfunc main() {}\n", fake))
	write("cmd/tested/main.go", "package main\n\nfunc main() {}\n")
	write("cmd/tested/main_test.go", fmt.Sprintf("package main\n\nimport (\n _ %q\n \"testing\"\n)\n\nfunc TestProbe(t *testing.T) {\n _ = t\n}\n", fake))
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
	check := filepath.Join(root, "tools", "bin", "architecture-check")
	for _, c := range []struct {
		pkg     string
		refused bool
	}{{"./cmd/linked", true}, {"./cmd/tested", false}} {
		t.Run("linked "+c.pkg, func(t *testing.T) {
			run := exec.Command("sh", check, "--linked", c.pkg)
			run.Dir = dir
			run.Env = compile.Env
			output, err := run.CombinedOutput()
			if !c.refused {
				if err != nil {
					t.Errorf("refused a fake entering only through a _test.go file: %v\n%s", err, output)
				}
				return
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("missing refusal of %s linking %s: %v\n%s", c.pkg, fake, err, output)
			}
			for _, text := range []string{c.pkg, fake, "_test.go"} {
				if !strings.Contains(string(output), text) {
					t.Errorf("the refusal does not say %q:\n%s", text, output)
				}
			}
		})
	}
	// No package under internal/slices/X imports one under internal/slices/Y
	// (ADR-0051), which depguard cannot hold: its deny list is a prefix, so
	// refusing the slices would refuse a slice's own subpackages too.
	// architecture-check --siblings refuses a sibling's import, from code or
	// tests, naming both packages, and passes a slice importing its own
	// subpackage and an engine, and cmd importing every slice. These fixtures
	// are a module of their own inside the temporary one, so the parent's
	// ./... and its lint, whose slices rule refuses a slice's own
	// subpackages, leave them out.
	stub := "package probe\n\nconst Value = 1\n"
	slicePath := func(name string) string { return module + "/internal/slices/" + name }
	for path, contents := range map[string]string{
		"go.mod":                              "module " + module + "\n\ngo 1.27\n",
		"internal/lang/stub.go":               stub,
		"internal/slices/alpha/store/stub.go": stub,
		"internal/slices/alpha/alpha.go":      fmt.Sprintf("package probe\n\nimport (\n lang %q\n store %q\n)\n\nconst Value = lang.Value + store.Value\n", module+"/internal/lang", slicePath("alpha/store")),
		"internal/slices/beta/beta.go":        fmt.Sprintf("package probe\n\nimport alpha %q\n\nconst Value = alpha.Value\n", slicePath("alpha")),
		"internal/slices/gamma/gamma.go":      stub,
		"internal/slices/gamma/gamma_test.go": fmt.Sprintf("package probe\n\nimport (\n store %q\n \"testing\"\n)\n\nfunc TestProbe(t *testing.T) {\n _ = t\n _ = store.Value\n}\n", slicePath("alpha/store")),
		"cmd/slices/main.go":                  fmt.Sprintf("package main\n\nimport (\n _ %q\n _ %q\n _ %q\n)\n\nfunc main() {}\n", slicePath("alpha"), slicePath("beta"), slicePath("gamma")),
	} {
		write("siblings/"+path, contents)
	}
	siblings := filepath.Join(dir, "siblings")
	compileSiblings := exec.Command("go", "test", "./...")
	compileSiblings.Dir = siblings
	compileSiblings.Env = compile.Env
	if output, err := compileSiblings.CombinedOutput(); err != nil {
		t.Fatalf("the sibling fixtures must compile and their tests pass: %v\n%s", err, output)
	}
	for _, c := range []struct{ pkg, imported string }{
		{"./internal/slices/alpha/...", ""},
		{"./cmd/slices", ""},
		{"./internal/slices/beta", "alpha"},
		{"./internal/slices/gamma", "alpha/store"},
	} {
		t.Run("siblings "+c.pkg, func(t *testing.T) {
			run := exec.Command("sh", check, "--siblings", c.pkg)
			run.Dir = siblings
			run.Env = compile.Env
			output, err := run.CombinedOutput()
			if c.imported == "" {
				if err != nil {
					t.Errorf("refused a slice's own subpackage, an engine or cmd importing the slices: %v\n%s", err, output)
				}
				return
			}
			importer := module + "/" + strings.TrimPrefix(c.pkg, "./")
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("missing refusal of %s importing %s: %v\n%s", importer, slicePath(c.imported), err, output)
			}
			for _, text := range []string{importer + " imports " + slicePath(c.imported), "ADR-0051"} {
				if !strings.Contains(string(output), text) {
					t.Errorf("the refusal does not say %q:\n%s", text, output)
				}
			}
		})
	}
}
