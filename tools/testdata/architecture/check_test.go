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
	// command slices, each under internal/slices, and
	// internal/next, a package under internal that no rule names, the UI, the packages that are no command, cmd,
	// internal/project, the project and git history engine the workdir and
	// metricsdir adapters wrap, and internal/covrun, the runner of each
	// language's tools coveragetools wraps.
	// internal/lang is a domain every command measures by, its port the one
	// workdir implements beside selection's; internal/snapshot is the domain
	// of the snapshots under .metrics/, its port metricsdir's (decision 41).
	// internal/release/releasetest is what the release tools' tests may
	// import from outside internal/, and internal/project what features
	// may (ADR-0061).
	for _, path := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", "internal/slices/mutation/domain", "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "internal/release/releasetest", "internal/project", "cmd/probe", "internal/covrun", "internal/slices/graph"} {
		write(path+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	for _, fake := range fakes {
		write(fake+"/stub.go", "package probe\n\nconst Value = 1\n")
	}
	for _, slice := range []string{"internal/slices/units", "internal/slices/version", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/mutation"} {
		write(slice+"/own/stub.go", "package probe\n\nconst Value = 1\n")
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
	// diagnostic must say, and for depguard the lists any of which may
	// refuse it: a shared domain importing a slice is refused by the domain
	// rule and the shared-domains rule alike, and the linter reports either
	// or both. An accepted one names none.
	type expectation struct {
		path, linter string
		texts, lists []string
	}
	var cases []expectation
	addAny := func(name, layer, imported, symbol string, rules []string, test bool) {
		path := layer + "/" + name + "/probe.go"
		body := "var Probe = dependency." + symbol + "\n"
		imports := fmt.Sprintf("import dependency %q\n", imported)
		if test {
			path = layer + "/" + name + "/probe_test.go"
			imports = fmt.Sprintf("import (\n dependency %q\n \"testing\"\n)\n", imported)
			body = "func TestProbe(t *testing.T) {\n _ = dependency." + symbol + "\n}\n"
			if imported == "os" && len(rules) == 0 {
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
		if len(rules) > 0 {
			refusal = expectation{path, "depguard", []string{imported}, rules}
		}
		cases = append(cases, refusal)
	}
	add := func(name, layer, imported, symbol, rule string, test bool) {
		var rules []string
		if rule != "" {
			rules = []string{rule}
		}
		addAny(name, layer, imported, symbol, rules, test)
	}
	// The domain arrow: each domain and its ports, code and tests, import
	// nothing of ours but the domain and its port subpackages, neither an
	// adapter nor the libraries an adapter wraps. Every domain may import
	// internal/lang and its port, the measure every command shares. The kong
	// rule keeps kong out of them, below. internal/slices/dry/domain,
	// internal/slices/scrap/domain and internal/slices/crap/domain, dry's,
	// scrap's and crap's own domains (ADR-0051), reach nothing outside, so
	// they have no port: they take files internal/lang parsed, their tests
	// parsing them through lang's fake, crap's scoring them with a coverage
	// Report. internal/coverage asks its port what a path a report names is
	// on disk (decision 46). internal/slices/mutation/domain, the mutation
	// slice's own (ADR-0051), declares its model-bearing ports beside their
	// values and keeps its fakes in its porttest. Mutation's settings
	// adapter, whose Reader implements mutants' SettingsReader and whose
	// Config aliases the domain's (T-97, T-197), is outside every domain too.
	mutation := "internal/slices/mutation/domain"
	domains := []string{"internal/selection", "internal/lang", "internal/snapshot", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage"}
	for _, domain := range domains {
		ported := domain != "internal/slices/dry/domain" && domain != "internal/slices/scrap/domain" && domain != "internal/slices/crap/domain" && domain != mutation
		// A slice's own domain is no shared one. The rule leaves dry's,
		// scrap's and crap's out of what a domain may import, so every other
		// domain importing them is refused, and they have no subpackage of
		// their own to import them. Mutation's is on the allow list, since
		// its external tests and porttest import it: the shared domains
		// importing it are refused by the shared-domains rule instead, which
		// refuses every package under internal/slices beside the domain
		// rule, and another slice's domain by architecture-check's sibling
		// rule, below.
		owned := strings.HasPrefix(domain, "internal/slices/")
		shared := !owned
		bases := []string{domain}
		if domain == mutation || domain == "internal/coverage" {
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
				for _, target := range []string{"internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/dry/domain", "internal/slices/scrap", "internal/slices/scrap/domain", "internal/slices/crap", "internal/slices/crap/domain", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "internal/project", "internal/covrun"} {
					if target == domain {
						continue
					}
					rules := []string{"domain"}
					if shared && strings.HasPrefix(target, "internal/slices/") {
						rules = append(rules, "shared-domains")
					}
					addAny(strings.ReplaceAll(target, "/", "_")+"_"+kind, base, module+"/"+target, "Value", rules, test)
				}
				switch {
				case shared:
					add("mutation_domain_"+kind, base, module+"/"+mutation, "Value", "shared-domains", test)
					add("mutation_fake_"+kind, base, module+"/"+mutation+"/porttest", "Value", "shared-domains", test)
				case domain != mutation:
					// depguard lets it by; architecture-check's sibling
					// rule refuses it, below.
					add("mutation_domain_"+kind, base, module+"/"+mutation, "Value", "", test)
				}
				if !owned || domain == mutation {
					add("domain_allowed_"+kind, base, module+"/"+domain, "Value", "", test)
				}
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
	add("own_fake_allowed", mutation, module+"/"+mutation+"/porttest", "Value", "", true)
	add("own_fake_refused", mutation, module+"/"+mutation+"/porttest", "Value", "mutant-fakes", false)
	add("own_fake_allowed", "internal/coverage", module+"/internal/coverage/porttest", "Value", "", true)
	add("own_fake_refused", "internal/coverage", module+"/internal/coverage/porttest", "Value", "coverage-fakes", false)
	add("coverage_fake_refused", mutation, module+"/internal/coverage/porttest", "Value", "coverage-fakes", false)
	// The adapter arrow: an adapter's code imports nothing of ours but the
	// port it implements, not a domain, another adapter, a slice, the UI,
	// cmd, the fakes, another adapter's port or a top-level library it does
	// not wrap; it may do I/O, and its tests may use the fakes. workdir wraps
	// project and implements internal/lang's port too, under a rule of its own;
	// metricsdir wraps project for the snapshot port, under its own, with the
	// snapshot library folded into it (T-195), and imports the snapshot
	// domain it serves. coveragetools wraps the coverage runner and
	// metricsdir's Dir for the coverage port, under its own, and imports the
	// coverage domain it serves, whose
	// Report it returns (decision 47): allowed lists what an adapter's rule
	// lets it import beside its port. testtools and testinputs (T-159),
	// freshenv, githistory, importgraph and worktrees (T-160), snapshots,
	// exceptions and sources (T-161) and settings (T-197) are the mutation
	// slice's own adapters, under internal/slices/mutation/adapters:
	// slice-packages, slice-adapters and slice-fakes leave their folder out,
	// so their own rules alone hold them, each accepting the domain and what
	// it wraps, worktrees the process-ownership engine, freshenv and
	// exceptions the settings adapter, importgraph the import-resolution
	// engine, never graph or project (T-162), snapshots the snapshot port,
	// and settings selection's port, whose SettingsReader it implements too,
	// and project, for the project root, among them. workdir and
	// metricsdir gave mutation's source edits and snapshot codec to sources
	// and snapshots (T-161), so their rules refuse the mutation domain now.
	for _, adapter := range []struct {
		base, domain, rule string
		others, allowed    []string
	}{
		{"internal/workdir", "internal/selection", "workdir", []string{"internal/snapshot/port", "internal/coverage/port", "internal/covrun"}, nil},
		{"internal/metricsdir", "internal/snapshot", "metricsdir", []string{"internal/selection/port", "internal/lang/port", "internal/coverage/port", "internal/covrun"}, []string{"internal/snapshot"}},

		{"internal/slices/mutation/adapters/testtools", mutation, "testtools", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/slices/graph"}, []string{mutation, "internal/lang", "internal/project", "internal/covrun"}},
		{"internal/slices/mutation/adapters/testinputs", mutation, "testinputs", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/slices/graph"}, []string{mutation, "internal/lang", "internal/project", "internal/covrun"}},
		{"internal/slices/mutation/adapters/freshenv", mutation, "freshenv", []string{"internal/snapshot/port", "internal/slices/graph"}, []string{mutation, "internal/coverage", "internal/coverage/port", "internal/selection", "internal/selection/port", "internal/lang", "internal/lang/port", "internal/slices/mutation/adapters/settings", "internal/project", "internal/covrun"}},
		{"internal/slices/mutation/adapters/githistory", mutation, "githistory", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/covrun", "internal/slices/graph"}, []string{mutation, "internal/project"}},
		{"internal/slices/mutation/adapters/importgraph", mutation, "importgraph", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/covrun", "internal/project", "internal/slices/graph"}, []string{mutation, "internal/imports"}},
		{"internal/slices/mutation/adapters/worktrees", mutation, "worktrees", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/covrun"}, []string{mutation, "internal/processes", "internal/project"}},
		{"internal/slices/mutation/adapters/snapshots", mutation, "snapshots", []string{"internal/selection/port", "internal/lang/port", "internal/coverage/port", "internal/project", "internal/covrun", "internal/slices/graph"}, []string{mutation, "internal/snapshot/port"}},
		{"internal/slices/mutation/adapters/exceptions", mutation, "exceptions", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/project", "internal/covrun", "internal/slices/graph"}, []string{mutation, "internal/slices/mutation/adapters/settings"}},
		{"internal/slices/mutation/adapters/sources", mutation, "sources", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/project", "internal/covrun", "internal/slices/graph"}, []string{mutation}},
		{"internal/slices/mutation/adapters/settings", mutation, "settings", []string{"internal/lang/port", "internal/snapshot/port", "internal/coverage/port", "internal/covrun", "internal/slices/graph"}, []string{mutation, "internal/selection/port", "internal/project"}},
		{"internal/coveragetools", "internal/coverage", "coveragetools", []string{"internal/selection/port", "internal/lang/port", "internal/snapshot/port", "internal/project"},
			[]string{"internal/coverage", "internal/covrun", "internal/metricsdir"}},
	} {
		base := adapter.base
		for _, target := range append([]string{"internal/selection", "internal/snapshot", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/selection/port/porttest", "internal/snapshot/port/porttest", "internal/lang", "internal/lang/port/porttest", "internal/cli", "internal/version", "internal/release", "cmd/probe"}, adapter.others...) {
			if slices.Contains(adapter.allowed, target) {
				continue
			}
			add(strings.ReplaceAll(target, "/", "_"), base, module+"/"+target, "Value", adapter.rule, false)
		}
		for _, target := range adapter.allowed {
			add(strings.ReplaceAll(target, "/", "_")+"_allowed", base, module+"/"+target, "Value", "", false)
		}
		if adapter.domain == mutation {
			add("root_contract_allowed", base, module+"/"+mutation, "Value", "", false)
			add("fake_allowed", base, module+"/"+mutation+"/porttest", "Value", "", true)
			add("fake_refused", base, module+"/"+mutation+"/porttest", "Value", adapter.rule, false)
		} else {
			add("port_allowed", base, module+"/"+adapter.domain+"/port", "Value", "", false)
			add("fake_allowed", base, module+"/"+adapter.domain+"/port/porttest", "Value", "", true)
		}
		add("os_allowed", base, "os", "ReadFile", "", false)
		add("fixture_io_allowed", base, "os", "ReadFile", "", true)
	}
	add("project_allowed", "internal/workdir", module+"/internal/project", "Value", "", false)
	add("lang_port_allowed", "internal/workdir", module+"/internal/lang/port", "Value", "", false)
	add("lang_fake_allowed", "internal/workdir", module+"/internal/lang/port/porttest", "Value", "", true)
	add("project_allowed", "internal/metricsdir", module+"/internal/project", "Value", "", false)
	add("selection_fake_allowed", "internal/slices/mutation/adapters/settings", module+"/internal/selection/port/porttest", "Value", "", true)
	// The slice arrow, code and tests alike: a slice imports the domain, its
	// ports, internal/cli and internal/lang with its port, never another slice, an adapter, the
	// libraries an adapter wraps, a package that is no command or cmd. The
	// snapshot domain is a domain like the others, its port a slice's to hold.
	// internal/next is held with no rule naming it, as a package no rule
	// names is, by slices; a slice under internal/slices, units, dry, scrap,
	// crap, version and mutation, by slice-packages, which lets it import its
	// own subpackages, and refuses what slices does, a package no rule names
	// among them. A package no rule names imports none under internal/slices,
	// a slice's own domain least of all. A slice's adapters, under
	// internal/slices/mutation/adapters (T-159), are refused to every slice
	// under internal/slices, its own too, by slice-adapters, since
	// slice-packages allows internal/slices/. internal/slices/mutation
	// reads settings through mutants' SettingsReader, never the settings
	// adapter (T-97, T-197).
	for _, slice := range []struct{ base, other string }{{"internal/next", "internal/slices/units"}, {"internal/slices/units", "internal/next"}, {"internal/slices/dry", "internal/next"}, {"internal/slices/scrap", "internal/next"}, {"internal/slices/crap", "internal/next"}, {"internal/slices/version", "internal/next"}, {"internal/slices/mutation", "internal/next"}, {"internal/next", "internal/slices/crap/domain"}, {"internal/next", mutation}, {"internal/next", "internal/slices/mutation"}} {
		rule, moved := "slices", strings.HasPrefix(slice.base, "internal/slices/")
		if moved {
			rule = "slice-packages"
		}
		for _, test := range []bool{false, true} {
			kind := "code"
			if test {
				kind = "test"
			}
			for _, target := range []string{slice.other, "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/version", "internal/release", "cmd/probe", "internal/project", "internal/covrun"} {
				refusing := rule
				if moved && strings.HasPrefix(target, "internal/slices/mutation/adapters/") {
					refusing = "slice-adapters"
				}
				add(strings.ReplaceAll(target, "/", "_")+"_"+kind, slice.base, module+"/"+target, "Value", refusing, test)
			}
			for _, target := range []string{"internal/selection", "internal/selection/port", "internal/snapshot", "internal/snapshot/port", "internal/coverage", "internal/coverage/port", "internal/cli", "internal/lang", "internal/lang/port"} {
				add(strings.ReplaceAll(target, "/", "_")+"_allowed_"+kind, slice.base, module+"/"+target, "Value", "", test)
			}
			if moved {
				// A subpackage of the slice imports the slice and another of
				// its subpackages, which depguard cannot tell from a
				// sibling's (architecture-check's siblings, below).
				add("own_slice_allowed_"+kind, slice.base, module+"/"+slice.base, "Value", "", test)
				add("own_subpackage_allowed_"+kind, slice.base, module+"/"+slice.base+"/own", "Value", "", test)
			}
			if slice.base == "internal/slices/mutation" {
				add("own_domain_allowed_"+kind, slice.base, module+"/"+mutation, "Value", "", test)
			}
		}
		add("selection_fake_allowed", slice.base, module+"/internal/selection/port/porttest", "Value", "", true)
		add("lang_fake_allowed", slice.base, module+"/internal/lang/port/porttest", "Value", "", true)
		add("snapshot_fake_allowed", slice.base, module+"/internal/snapshot/port/porttest", "Value", "", true)
		add("coverage_fake_allowed", slice.base, module+"/internal/coverage/port/porttest", "Value", "", true)
		// A slice's own code links no fake (T-56): every porttest of the
		// module, as go list finds them, so a new port's fake that the
		// slice-fakes rule does not name fails here. A fake under
		// internal/slices, mutation's, is no import the slices rule allows
		// either, so either may refuse it there.
		for _, fake := range fakes {
			rules := []string{"slice-fakes"}
			if !moved && strings.HasPrefix(fake, "internal/slices/") {
				rules = append(rules, "slices")
			}
			addAny(strings.ReplaceAll(fake, "/", "_")+"_code", slice.base, module+"/"+fake, "Value", rules, false)
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
		for _, target := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/version", "internal/release", "cmd/probe"} {
			add(strings.ReplaceAll(target, "/", "_")+"_"+kind, "internal/cli", module+"/"+target, "Value", "ui", test)
		}
		add("stdlib_allowed_"+kind, "internal/cli", "strings", "ToLower", "", test)
	}
	// The engine arrow, code and tests alike: internal/processes, the
	// process-ownership engine (T-151), imports nothing of ours, no domain
	// above all, so no domain's values reach it; it starts and kills
	// processes itself. Who may import it is held above: no domain or
	// slice, and of the adapters only worktrees, whose workers run through it.
	for _, test := range []bool{false, true} {
		kind := "code"
		if test {
			kind = "test"
		}
		for _, target := range []string{"internal/selection", "internal/selection/port", "internal/lang", "internal/lang/port", "internal/snapshot", "internal/slices/crap/domain", mutation, mutation + "/porttest", "internal/coverage", "internal/coverage/port", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/imports", "internal/slices/mutation/adapters/worktrees", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/units", "internal/slices/version", "internal/slices/crap", "internal/slices/mutation", "internal/cli", "cmd/probe", "internal/project", "internal/covrun"} {
			add(strings.ReplaceAll(target, "/", "_")+"_"+kind, "internal/processes", module+"/"+target, "Value", "processes", test)
		}
		add("os_allowed_"+kind, "internal/processes", "os", "ReadFile", "", test)
	}
	add("exec_allowed", "internal/processes", "os/exec", "Command", "", false)
	// The engine arrow for internal/imports, the import-resolution engine
	// (T-162), code and tests alike: it imports internal/lang, whose parsed
	// files it resolves, project, through which it parses them and finds
	// what surrounds them, and the coverage runner, for pytest's naming of a
	// test file, and nothing else of ours: no other domain or its port, no
	// slice, adapter or fake, the UI, cmd, the build's version or graph.
	// Who may import it is held above: no domain or slice, and
	// of the adapters only importgraph, whose Reach calls it.
	for _, test := range []bool{false, true} {
		kind := "code"
		if test {
			kind = "test"
		}
		for _, target := range []string{"internal/selection", "internal/selection/port", "internal/lang/port", "internal/snapshot", "internal/snapshot/port", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/coverage/port", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/processes", "internal/slices/mutation/adapters/worktrees", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "internal/slices/graph"} {
			add(strings.ReplaceAll(target, "/", "_")+"_"+kind, "internal/imports", module+"/"+target, "Value", "imports", test)
		}
		for _, fake := range fakes {
			add(strings.ReplaceAll(fake, "/", "_")+"_"+kind, "internal/imports", module+"/"+fake, "Value", "imports", test)
		}
		for _, target := range []string{"internal/lang", "internal/project", "internal/covrun"} {
			add(strings.ReplaceAll(target, "/", "_")+"_allowed_"+kind, "internal/imports", module+"/"+target, "Value", "", test)
		}
		add("os_allowed_"+kind, "internal/imports", "os", "ReadFile", "", test)
		add("json_allowed_"+kind, "internal/imports", "encoding/json", "Marshal", "", test)
	}
	// The engine arrow for internal/project, the project and git history
	// engine (ADR-0061, T-194), its code: it imports internal/lang and its
	// port, whose Surroundings it answers from the disk, and
	// internal/selection and internal/snapshot, whose values it names paths
	// by, and nothing else of ours: no other domain or port, slice, adapter,
	// engine or fake, the UI, cmd, the build's version, graph or the
	// coverage runner. Its tests may use lang's fake. Who may
	// import it is held above: no domain or slice, and of the adapters only
	// those wrapping it.
	for _, target := range []string{"internal/selection/port", "internal/snapshot/port", "internal/slices/dry/domain", mutation, "internal/coverage", "internal/coverage/port", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/worktrees", "internal/slices/mutation/adapters/githistory", "internal/slices/crap", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "internal/covrun", "internal/slices/graph", "internal/slices/mutation/adapters/settings"} {
		add(strings.ReplaceAll(target, "/", "_"), "internal/project", module+"/"+target, "Value", "project", false)
	}
	for _, fake := range fakes {
		add(strings.ReplaceAll(fake, "/", "_")+"_code", "internal/project", module+"/"+fake, "Value", "project", false)
	}
	for _, target := range []string{"internal/lang", "internal/lang/port", "internal/selection", "internal/snapshot"} {
		add(strings.ReplaceAll(target, "/", "_")+"_allowed", "internal/project", module+"/"+target, "Value", "", false)
	}
	add("os_allowed", "internal/project", "os", "ReadFile", "", false)
	add("exec_allowed", "internal/project", "os/exec", "Command", "", false)
	add("lang_fake_allowed", "internal/project", module+"/internal/lang/port/porttest", "Value", "", true)
	// The engine arrow for internal/covrun, the coverage runner (decision
	// 46, ADR-0061, T-196), its code: it imports the coverage domain and its
	// port, whose Report it builds through the domain's API, internal/lang
	// and internal/project, and nothing else of ours: no other domain or
	// port, slice, adapter, engine or fake, the UI, cmd, the build's
	// version or graph. Its tests may use the coverage port's fake.
	// Who may import it is held above: no domain or slice, and of the
	// adapters only those wrapping it.
	for _, target := range []string{"internal/selection", "internal/selection/port", "internal/snapshot", "internal/snapshot/port", "internal/slices/crap/domain", mutation, "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/crap", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/release", "cmd/probe", "internal/slices/graph", "internal/slices/mutation/adapters/settings"} {
		add(strings.ReplaceAll(target, "/", "_"), "internal/covrun", module+"/"+target, "Value", "covrun", false)
	}
	for _, fake := range fakes {
		add(strings.ReplaceAll(fake, "/", "_")+"_code", "internal/covrun", module+"/"+fake, "Value", "covrun", false)
	}
	for _, target := range []string{"internal/coverage", "internal/coverage/port", "internal/lang", "internal/project"} {
		add(strings.ReplaceAll(target, "/", "_")+"_allowed", "internal/covrun", module+"/"+target, "Value", "", false)
	}
	add("os_allowed", "internal/covrun", "os", "ReadFile", "", false)
	add("exec_allowed", "internal/covrun", "os/exec", "Command", "", false)
	add("coverage_fake_allowed", "internal/covrun", module+"/internal/coverage/port/porttest", "Value", "", true)
	// The graph slice's own arrow, which graph_test.go writes (T-208).
	addGraph(module, mutation, fakes, add, addAny)
	// The project engine importing a slice is refused by its own rule and
	// by architecture-check --outside alike, below.
	add("units", "internal/project", module+"/internal/slices/units", "Value", "project", false)
	// What is outside internal/slices and no rule holds, which
	// architecture-check --outside refuses or accepts, below: depguard lets
	// every one by.
	add("mutation_domain", "internal/covrun", module+"/"+mutation, "Value", "", true)
	add("dry_domain", "features", module+"/internal/slices/dry/domain", "Value", "", false)
	// What is outside internal/ and cmd/, which architecture-check
	// --toplevel refuses or accepts by its rows, below (ADR-0061): depguard
	// lets every one by.
	addToplevel(module, add)
	// What assembles them is free to import the domain, the adapters and
	// the slices.
	for _, target := range []string{"internal/selection", "internal/lang", "internal/snapshot", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/cli", "internal/version", "internal/project", "internal/covrun"} {
		add(strings.ReplaceAll(target, "/", "_")+"_allowed", "cmd/probe", module+"/"+target, "Value", "", false)
	}
	for i, imported := range mocks {
		for _, test := range []bool{false, true} {
			add(fmt.Sprintf("mock%d_%t", i, test), "internal/next", imported, "Value", "mocks", test)
		}
	}
	// internal/lang's, internal/snapshot's, internal/slices/dry/domain's,
	// internal/slices/scrap/domain's, internal/slices/crap/domain's and
	// internal/coverage's tests have no property, so rapid is refused
	// there.
	for _, domain := range []string{"internal/selection"} {
		add("rapid_allowed", domain, "pgregory.net/rapid", "Value", "", true)
	}
	// kong is the UI's: the entry point and internal/cli, nowhere else, but
	// for mutation's settings adapter, which imports the fork alone, for its
	// Suggest (decision 62): kong itself is refused there as anywhere else.
	for i, kong := range kongs {
		rule, fork := "kong", kong == "github.com/donvargax/kong"
		if fork {
			rule = "kong-fork"
		}
		for _, test := range []bool{false, true} {
			for _, layer := range []string{"cmd/probe", "internal/cli"} {
				add(fmt.Sprintf("kong%d_allowed_%t", i, test), layer, kong, "Value", "", test)
			}
			settings := "internal/slices/mutation/adapters/settings"
			if fork {
				add(fmt.Sprintf("kong%d_allowed_%t", i, test), settings, kong, "Value", "", test)
			} else {
				add(fmt.Sprintf("kong%d_%t", i, test), settings, kong, "Value", rule, test)
			}
			for _, layer := range []string{"internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/project", "internal/covrun", "internal/version", "internal/release"} {
				add(fmt.Sprintf("kong%d_%t", i, test), layer, kong, "Value", rule, test)
			}
		}
	}
	// Rapid only in the domain package's own tests, not its code, a
	// subpackage's tests (its ports and fakes) or anything else.
	for _, layer := range []string{"internal/selection", "internal/selection/port", "internal/selection/port/porttest", "internal/lang", "internal/lang/port", "internal/lang/port/porttest", "internal/snapshot", "internal/snapshot/port", "internal/snapshot/port/porttest", "internal/slices/dry/domain", "internal/slices/scrap/domain", "internal/slices/crap/domain", mutation, "internal/coverage", "internal/coverage/port", "internal/coverage/port/porttest", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/slices/mutation/adapters/worktrees", "internal/processes", "internal/imports", "internal/slices/mutation/adapters/testtools", "internal/slices/mutation/adapters/testinputs", "internal/slices/mutation/adapters/importgraph", "internal/slices/mutation/adapters/githistory", "internal/slices/mutation/adapters/freshenv", "internal/slices/mutation/adapters/snapshots", "internal/slices/mutation/adapters/exceptions", "internal/slices/mutation/adapters/sources", "internal/slices/mutation/adapters/settings", "internal/slices/dry", "internal/slices/scrap", "internal/slices/crap", "internal/slices/units", "internal/slices/version", "internal/slices/mutation", "internal/next", "internal/project", "internal/covrun"} {
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
			refusal = expectation{path, "gochecksumtype", []string{"Failure", "missing cases for Unreadable"}, nil}
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
			refusal = expectation{path, linter, []string{text}, nil}
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
	// Slices under internal/slices: alpha's code importing its own
	// subpackage and an engine, beta's code importing alpha, gamma's tests
	// importing alpha's subpackage, and cmd importing every slice.
	// slice-packages accepts each, and architecture-check --siblings, below,
	// refuses beta's and gamma's sibling imports.
	slicePath := func(name string) string { return module + "/internal/slices/" + name }
	for _, fixture := range []struct{ path, contents string }{
		{"internal/slices/alpha/store/stub.go", "package probe\n\nconst Value = 1\n"},
		{"internal/slices/alpha/alpha.go", fmt.Sprintf("package probe\n\nimport (\n lang %q\n store %q\n)\n\nconst Value = lang.Value + store.Value\n", module+"/internal/lang", slicePath("alpha/store"))},
		{"internal/slices/beta/beta.go", fmt.Sprintf("package probe\n\nimport alpha %q\n\nconst Value = alpha.Value\n", slicePath("alpha"))},
		{"internal/slices/gamma/gamma.go", "package probe\n\nconst Value = 1\n"},
		{"internal/slices/gamma/gamma_test.go", fmt.Sprintf("package probe\n\nimport (\n store %q\n \"testing\"\n)\n\nfunc TestProbe(t *testing.T) {\n _ = t\n _ = store.Value\n}\n", slicePath("alpha/store"))},
		{"cmd/slices/main.go", fmt.Sprintf("package main\n\nimport (\n _ %q\n _ %q\n _ %q\n)\n\nfunc main() {}\n", slicePath("alpha"), slicePath("beta"), slicePath("gamma"))},
	} {
		write(fixture.path, fixture.contents)
		cases = append(cases, expectation{path: fixture.path})
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
		if matches && len(expected.lists) > 0 {
			matches = slices.ContainsFunc(expected.lists, func(list string) bool {
				return strings.Contains(issue.Text, "from list '"+list+"'")
			})
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
				t.Errorf("missing %s refusal: %s, from one of the lists %s", c.linter, strings.Join(c.texts, ", "), strings.Join(c.lists, ", "))
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
	// slice-packages allows internal/slices/ for a slice's own subpackages.
	// architecture-check --siblings refuses a sibling's import, from code or
	// tests, naming both packages, and passes a slice importing its own
	// subpackage and an engine, and cmd importing every slice. The fixtures
	// are in the temporary module with the others, written before it is
	// compiled and linted above, so the lint holds that slice-packages
	// accepts every one of them, the sibling imports among them.
	checkSiblings(t, check, dir, compile.Env, module)
	// Engines stay behind stable APIs the slices import, never the reverse
	// (ADR-0051, T-167): a package outside internal/slices imports none
	// under it but cmd, which assembles them; graph's row went when it
	// became a slice (T-208). depguard holds none of them, so
	// architecture-check --outside refuses, from code or tests, naming both
	// packages; the lint above holds that depguard accepts every fixture.
	checkOutside(t, check, dir, compile.Env, module)
	// Every package but cmd lives under internal/ (ADR-0061): a package
	// outside internal/ and cmd/ imports none under it, from its code or
	// its tests, but what its row in docs/go.md allows, which
	// architecture-check --toplevel holds over the same go list fields.
	checkToplevel(t, check, dir, compile.Env, module)
	// The rows that allow an import --outside or --toplevel refuses are
	// tools/architecture-allow.txt's, beside the script (T-222): a copy of
	// it with rows of its own proves how it reads them.
	checkRows(t, check, dir, compile.Env, module)
}
