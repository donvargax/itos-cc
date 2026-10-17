package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
)

// These parser-boundary observations cannot be inferred from a successful
// command: the legacy handlers intentionally distinguish omitted flags from
// explicit empty/zero values and report the caller's original number text.
func parsedMutationFixture(t *testing.T, a adapters, args ...string) *invocation {
	t.Helper()
	var c commandLine
	ui := &cli.UI{Stdout: io.Discard, Stderr: io.Discard}
	parser, err := parserOf(t.Context(), &c, ui, a)
	if err != nil {
		t.Fatal(err)
	}
	admitted, help, problem := cli.GroupLine(moved(parser)["mutation"], args)
	if problem != nil || help {
		t.Fatalf("fixture admission: help=%v problem=%v", help, problem)
	}
	parsed, err := cli.Parse(parser, admitted)
	if err != nil {
		t.Fatal(err)
	}
	return mutationInvocation(parsed, &c, a)
}

func TestMutationParserKeepsProvidedValuesAndSpellings(t *testing.T) {
	for _, tc := range []struct {
		args        []string
		name, value string
		set         bool
	}{
		{[]string{"run"}, "count", "", false},
		{[]string{"run", "--count", "00"}, "count", "00", true},
		{[]string{"run", "--count=+0"}, "count", "+0", true},
		{[]string{"run", "--seed="}, "seed", "", true},
		{[]string{"sample", "--seed="}, "seed", "", true},
		{[]string{"sample"}, "seed", "", false},
		{[]string{"sample"}, "count", "20", false},
		{[]string{"except"}, "reason", "", false},
		{[]string{"except", "--reason="}, "reason", "", true},
		{[]string{"run", "--timeout-factor=0e0"}, "timeout-factor", "0e0", true},
		{[]string{"run", "--fail-uncovered"}, "fail-uncovered", "true", true},
		{[]string{"check", "--fail-uncovered=lines"}, "fail-uncovered", "lines", true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			in := parsedMutationFixture(t, adapters{}, tc.args...)
			if in.set(tc.name) != tc.set || in.str(tc.name) != tc.value {
				t.Fatalf("set=%v raw=%q, want %v %q", in.set(tc.name), in.str(tc.name), tc.set, tc.value)
			}
		})
	}
	in := parsedMutationFixture(t, adapters{}, "run", "a,b.go", "--coverage-report=x,y.info", "b.go", "--coverage-report", "other.info", "--", "--no-annotate")
	if !slices.Equal(in.args, []string{"a,b.go", "b.go", "--no-annotate"}) || !slices.Equal(in.strs("coverage-report"), []string{"x,y.info", "other.info"}) || in.set("no-annotate") {
		t.Fatalf("argument/report identity lost: %+v", in)
	}
}

func TestMutationParserRefusesFlagsWithoutReadingAdapters(t *testing.T) {
	for _, tc := range []struct {
		args        string
		rule, value string
	}{
		{"run --coverage", "flags.unknown", ""},
		{"run --annotate", "flags.unknown", ""},
		{"run --no-coverage=true", "flags.switch-value", ""},
		{"run --no-annotate=false", "flags.switch-value", ""},
		{"run --count", "flags.value-missing", ""},
		{"run --count --json", "flags.value-missing", ""},
		{"run --count=1.5", "flags.value-invalid", "1.5"},
		{"run --count=0x10", "flags.value-invalid", "0x10"},
		{"run --workers=1 --workers=2", "flags.repeated", ""},
		{"run --count=wrong -changed", "flags.value-invalid", "wrong"},
		{"run --count -changed", "flags.value-missing", ""},
		{"run -changed --count=wrong", "flags.unknown", ""},
		{"except --reason x --reason y", "flags.repeated", ""},
		{"check --fail-uncovered=other", "flags.value-invalid", "other"},
	} {
		t.Run(tc.args, func(t *testing.T) {
			var c commandLine
			ui := &cli.UI{Stdout: io.Discard, Stderr: io.Discard}
			parser, err := parserOf(t.Context(), &c, ui, adapters{})
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{"mutation"}, strings.Fields(tc.args)...)
			_, err = cli.Parse(parser, args)
			if err == nil {
				t.Fatal("invalid input accepted")
			}
			p := cli.ParseProblem(err, args)
			if p.Rule != tc.rule {
				t.Fatalf("%+v, want %s", p, tc.rule)
			}
			if tc.value != "" && !strings.Contains(p.Message, tc.value) {
				t.Fatalf("lost original invalid value: %+v", p)
			}
		})
	}
}

// Help and group refusals happen with empty adapters, proving they do not
// discover configuration, snapshots or tools. Every run owns fresh state.
func TestMutationGroupHelpRoutingAndErrorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		code     int
		fragment string
	}{
		{[]string{"mutation"}, 0, "Subcommands:"},
		{[]string{"help", "mutation", "run"}, 0, "usage: itos-cc mutation run"},
		{[]string{"help", "mutation", "except"}, 0, "<file>:<line>:<column> --reason TEXT"},
		{[]string{"mutation", "--help", "nosuch"}, 0, "Subcommands:"},
		{[]string{"mutation", "run", "--bad", "--help"}, 0, "usage: itos-cc mutation run"},
		{[]string{"mutation", "run", "--help", "--json"}, 0, "usage: itos-cc mutation run"},
		{[]string{"--json", "mutation", "rn"}, 2, "Did you mean 'mutation run'?"},
		{[]string{"mutation", "rn", "--help", "--json"}, 2, "The subcommands of mutation are run, list, check, sample, and except."},
		{[]string{"mutation", "--changed", "run", "--json"}, 2, "before a subcommand"},
		{[]string{"mutation", "--json"}, 2, "command.missing"},
		{[]string{"--json", "help", "mutation", "run", "extra"}, 2, "help takes one command"},
		{[]string{"--json", "help", "mutation", "unknown"}, 2, "The subcommands of mutation are"},
		{[]string{"--json", "help", "mutation", "--changed"}, 2, "there is no command"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run(t.Context(), tc.args, strings.NewReader(""), &out, &errOut, adapters{})
			if code != tc.code || !strings.Contains(out.String()+errOut.String(), tc.fragment) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want %d %q", code, out.String(), errOut.String(), tc.code, tc.fragment)
			}
		})
	}
}

func TestMutationGroupJSONPlacementRetainsLegacyDispatch(t *testing.T) {
	// Invalid count stops before configuration or tools, but reaches the actual
	// child handler and its retained range error. stdout capture follows the
	// legacy handler's process streams until those handlers move.
	for _, tc := range []struct {
		args []string
		json bool
	}{
		{[]string{"mutation", "--json", "run", "--count=00"}, false},
		{[]string{"mutation", "--json", "--json", "run", "--count=00"}, false},
		{[]string{"mutation", "--json", "run", "--count=00", "--json"}, true},
		{[]string{"--json", "mutation", "run", "--count=00"}, true},
	} {
		var code int
		output, errOutput := captured(t, func() { code = run(t.Context(), tc.args, strings.NewReader(""), io.Discard, io.Discard, adapters{}) })
		if code != 2 {
			t.Fatalf("%q exit=%d out=%q err=%q", tc.args, code, output, errOutput)
		}
		if json.Valid([]byte(output)) != tc.json {
			t.Fatalf("%q stdout=%q, wantJSON=%v", tc.args, output, tc.json)
		}
		if !strings.Contains(output+errOutput, "00") {
			t.Fatalf("original numeric spelling missing: %q %q", output, errOutput)
		}
	}
}

// mutationHelpBytes is fixed from pre-migration 0ebb527 command/flag literals
// and the existing Page layout. @WORKERS@ is the supplied boundary default;
// the fixture was not obtained by executing either product binary.
//
//go:embed testdata/mutation-help.json
var mutationHelpBytes []byte

func TestMutationHelpPreservesOriginalBytes(t *testing.T) {
	var expected map[string]string
	if err := json.Unmarshal(mutationHelpBytes, &expected); err != nil {
		t.Fatal(err)
	}
	var c commandLine
	parser, err := parserOf(t.Context(), &c, &cli.UI{Stdout: io.Discard, Stderr: io.Discard}, adapters{})
	if err != nil {
		t.Fatal(err)
	}
	expectedNames := []string{"mutation run", "mutation list", "mutation check", "mutation sample", "mutation except"}
	if len(expected) != len(expectedNames)+1 {
		t.Fatalf("golden must hold the group and all five children, got %d pages", len(expected))
	}
	var actualNames []string
	for _, node := range moved(parser)["mutation"].Children {
		actualNames = append(actualNames, "mutation "+node.Name)
	}
	if !slices.Equal(actualNames, expectedNames) {
		t.Fatalf("mutation child population/order changed: %q, want %q", actualNames, expectedNames)
	}
	visited := map[string]int{}
	for _, node := range movedLeaves(parser) {
		name := node.Path()
		if !strings.HasPrefix(name, "mutation ") {
			continue
		}
		want, ok := expected[name]
		if !ok {
			t.Fatalf("unexpected mutation leaf %q", name)
		}
		visited[name]++
		page, err := cli.PageOf(node)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range node.Flags {
			if f.Name == "workers" {
				want = strings.ReplaceAll(want, "@WORKERS@", f.Default)
			}
		}
		if got := page.String(); got != want {
			t.Errorf("%s help changed\ngot:\n%s\nwant:\n%s", name, got, want)
		}
	}
	for _, name := range expectedNames {
		if _, ok := expected[name]; !ok {
			t.Fatalf("missing golden page %q", name)
		}
		if visited[name] != 1 {
			t.Fatalf("golden page %q visited %d times, want once", name, visited[name])
		}
	}
	group, err := cli.GroupHelp(moved(parser)["mutation"])
	if err != nil {
		t.Fatal(err)
	}
	if group != expected["mutation"] {
		t.Fatalf("group help changed\ngot:\n%s\nwant:\n%s", group, expected["mutation"])
	}
	// The group stays flattened in the top-level page, in original child order.
	top := usage(parser)
	prior := -1
	for _, name := range []string{"run", "list", "check", "sample", "except"} {
		node := moved(parser)["mutation"]
		for _, child := range node.Children {
			if child.Name != name {
				continue
			}
			row := fmt.Sprintf("  %-15s %s\n", "mutation "+name, child.Help)
			i := strings.Index(top, row)
			if i < 0 || i <= prior {
				t.Fatalf("flattened help row/order lost: %q\n%s", row, top)
			}
			prior = i
		}
	}
}

// Sample's query takes its seed and range from the model: an explicit empty
// one is there, empty, and an omitted one is not, so only that one is read
// from HEAD, as the handler's seed holds.
func TestMutationSampleModelKeepsExplicitEmptySeedAndSince(t *testing.T) {
	for _, tc := range []struct {
		args              []string
		seed, since       *string
		count             int
		written, spelling string
	}{
		{[]string{"sample"}, nil, nil, 20, "count", ""},
		{[]string{"sample", "--seed=", "--since="}, new(""), new(""), 20, "seed", ""},
		{[]string{"sample", "--seed", "s", "--since", "base", "--count=00"}, new("s"), new("base"), 0, "count", "00"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var c commandLine
			parser, err := parserOf(t.Context(), &c, &cli.UI{Stdout: io.Discard, Stderr: io.Discard}, adapters{})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := cli.Parse(parser, append([]string{"mutation"}, tc.args...))
			if err != nil {
				t.Fatal(err)
			}
			got := c.Mutation.Sample
			same := func(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
			if !same(got.Seed, tc.seed) || !same(got.Since, tc.since) || got.Count != tc.count {
				t.Fatalf("sample model: seed=%v since=%v count=%d", got.Seed, got.Since, got.Count)
			}
			if written := cli.Provided(parsed)[tc.written]; tc.spelling != "" && (len(written) != 1 || written[0] != tc.spelling) {
				t.Fatalf("written %s: %q, want %q", tc.written, written, tc.spelling)
			}
		})
	}
}

func TestMutationTypedModelUsesTheValidatedFlags(t *testing.T) {
	var c commandLine
	parser, err := parserOf(t.Context(), &c, &cli.UI{Stdout: io.Discard, Stderr: io.Discard}, adapters{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := cli.Parse(parser, []string{"mutation", "run", "first.go", "--count=01", "--timeout-factor=1e1", "--coverage-report=a,b.info", "--coverage-report=other.info", "--no-annotate", "--fail-uncovered=lines", "--", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	got := c.Mutation.Run
	if got.Count != 1 || got.TimeoutFactor != 10 || !got.NoAnnotate || got.JSON || got.FailUncovered == nil || *got.FailUncovered != "lines" || !slices.Equal(got.Paths, []string{"first.go", "--json"}) || !slices.Equal(got.CoverageReport, []string{"a,b.info", "other.info"}) {
		t.Fatalf("wrong typed model: %+v", got)
	}
	in := mutationInvocation(parsed, &c, adapters{})
	if in.str("count") != "01" || in.str("timeout-factor") != "1e1" || in.json {
		t.Fatalf("typed conversion erased raw/provided facts: %+v", in)
	}
	// Another parser/model has neither values nor the prior explicit flags.
	next := parsedMutationFixture(t, adapters{}, "run")
	if next.set("count") || next.set("no-annotate") || next.integer("workers") < 1 || len(next.args) != 0 {
		t.Fatalf("state crossed invocations: %+v", next)
	}
}

func TestMutationParsedAdmissionKeepsEmptyAndZeroRefusals(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		rule, flag string
	}{
		{[]string{"run", "--count=00"}, "flags.value-invalid", "--count"},
		{[]string{"run", "--seed="}, "flags.conflict", "--seed"},
		{[]string{"run", "--count=1", "--seed="}, "flags.value-invalid", "--seed"},
		{[]string{"sample", "--count=00"}, "flags.value-invalid", "--count"},
		{[]string{"sample", "--count=+0"}, "flags.value-invalid", "--count"},
		{[]string{"except"}, "flags.value-missing", "--reason"},
		{[]string{"except", "--reason="}, "flags.value-invalid", "--reason"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			args := append([]string{"--json", "mutation"}, tc.args...)
			var code int
			var given bytes.Buffer
			out, errOut := captured(t, func() { code = run(t.Context(), args, strings.NewReader(""), &given, io.Discard, adapters{}) })
			// Moved children answer on the streams given, the others on the process's.
			out += given.String()
			var result struct {
				Problems []struct{ Rule, Flag, Value string }
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("exit=%d stdout=%q stderr=%q: %v", code, out, errOut, err)
			}
			if code != 2 || len(result.Problems) != 1 || result.Problems[0].Rule != tc.rule || result.Problems[0].Flag != tc.flag {
				t.Fatalf("lost actual admission: exit=%d result=%+v", code, result)
			}
			// The refused count is quoted back as it was written.
			if _, written, ok := strings.Cut(tc.args[len(tc.args)-1], "--count="); ok && result.Problems[0].Value != written {
				t.Fatalf("count spelling lost: %+v", result.Problems[0])
			}
		})
	}
}
