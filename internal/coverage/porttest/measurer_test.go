package porttest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
)

func fixture(t *testing.T, language string, files map[string]string, d Measurement) (*Measurer, string, []string) {
	t.Helper()
	root := t.TempDir()
	held := map[string]string{}
	var sources []string
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		held[path] = text
		sources = append(sources, path)
	}
	d.Language = language
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	m := &Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: held}, Declarations: map[coverage.Unloaded]Measurement{{Language: language, Dir: root}: d}}
	return m, root, sources
}

func TestBuiltInFormatsAndIntegrationKeepMeasuredInventory(t *testing.T) {
	for _, tc := range []struct{ language, file, report string }{
		{"go", "a.go", "mode: set\nexample/a.go:2.3,4.7 2 1\n"},
		{"python", "a.py", "SF:a.py\nDA:2,1\nend_of_record\n"},
		{"typescript", "a.ts", "SF:a.ts\nDA:2,1\nend_of_record\n"},
		{"kotlin", "a.kt", `<report><package name=""><sourcefile name="a.kt"><line nr="2" mi="0" ci="1" mb="0" cb="0"/></sourcefile></package></report>`},
	} {
		t.Run(tc.language, func(t *testing.T) {
			m, root, sources := fixture(t, tc.language, map[string]string{tc.file: "source"}, Measurement{Report: tc.report})
			r, failure := m.Measure(t.Context(), port.Request{Sources: sources})
			if failure != nil {
				t.Fatal(failure)
			}
			file := filepath.Join(root, tc.file)
			if hit, ok := r.LineCovered(file, 2); !hit || !ok || !r.Measures(tc.language) {
				t.Fatalf("missing actual format facts: %+v", r)
			}
			if tc.language == "go" {
				b := r.GoBlocks(file)
				if len(b) != 1 || b[0].Column != 3 || b[0].EndColumn != 7 || b[0].Weight != 2 {
					t.Fatalf("Go spans lost: %+v", b)
				}
			}
		})
	}
	m, root, sources := fixture(t, "typescript", map[string]string{"a.ts": "source"}, Measurement{Report: "SF:a.ts\nDA:2,0\nDA:4,1\nend_of_record\n", IntegrationReport: "SF:a.ts\nDA:2,1\nDA:3,1\nend_of_record\n"})
	r, failure := m.Measure(t.Context(), port.Request{Sources: sources})
	if failure != nil {
		t.Fatal(failure)
	}
	file := filepath.Join(root, "a.ts")
	if got := r.LineSources(file, 2); !reflect.DeepEqual(got, []string{"integration"}) {
		t.Fatalf("integration: %v", got)
	}
	if got := r.LineSources(file, 4); !reflect.DeepEqual(got, []string{"in-process"}) {
		t.Fatalf("own: %v", got)
	}
	if _, ok := r.LineCovered(file, 3); ok {
		t.Fatal("collector-only brace line entered inventory")
	}
}

func TestMissingIntegrationAppliesOnlyToBuiltinRequests(t *testing.T) {
	m, root, sources := fixture(t, "typescript", map[string]string{"a.ts": "source"}, Measurement{Report: "SF:a.ts\nDA:2,1\nend_of_record\n", WithoutC8: true})
	raw := filepath.Join(root, "raw.info")
	m.Aggregate.Files[raw] = "SF:a.ts\nDA:2,1\nend_of_record\n"
	for _, req := range []port.Request{{Sources: sources}, {Sources: sources, Reports: []string{raw}}, {Sources: sources, Reports: []string{raw}, Command: "write raw"}, {Sources: sources, Existing: true}} {
		r, failure := m.Measure(t.Context(), req)
		if failure != nil {
			t.Fatal(failure)
		}
		want := len(req.Reports) == 0 && !req.Existing
		if (len(r.IntegrationMissing()) > 0) != want {
			t.Fatalf("request %+v missing integration=%v", req, r.IntegrationMissing())
		}
		if want {
			if _, ok := r.IntegrationMissing()[0].Cause.(port.ToolMissing); !ok {
				t.Fatal("untyped missing collector")
			}
		}
	}
}

func TestInventoryUsesOnlyUnloadedSupportedPopulation(t *testing.T) {
	for _, variant := range []string{"valid", "absent", "failed"} {
		t.Run(variant, func(t *testing.T) {
			d := Measurement{Report: "SF:hit.py\nDA:2,1\nend_of_record\n", InventoryReport: "SF:miss.py\nDA:7,9\nend_of_record\nSF:empty.py\nend_of_record\nSF:hit.py\nDA:9,1\nend_of_record\nSF:unrelated.py\nDA:8,1\nend_of_record\n"}
			if variant == "absent" {
				d.InventoryReport = ""
			}
			if variant == "failed" {
				d.InventoryExit = 1
				d.InventoryOutput = "collector failed"
			}
			m, root, sources := fixture(t, "python", map[string]string{"hit.py": "source", "miss.py": "source", "empty.py": "# empty"}, d)
			r, failure := m.Measure(t.Context(), port.Request{Sources: sources})
			if failure != nil {
				t.Fatal(failure)
			}
			if f := m.Inventory(t.Context(), r, "python"); f != nil {
				t.Fatal(f)
			}
			lines, complete := r.Lines(filepath.Join(root, "miss.py"))
			if complete != (variant == "valid") {
				t.Fatalf("%s proof=%v lines=%v", variant, complete, lines)
			}
			if complete && (len(lines) != 1 || lines[0].Covered || lines[0].Line != 7) {
				t.Fatalf("inventory not forced unexecuted: %+v", lines)
			}
			_, empty := r.Lines(filepath.Join(root, "empty.py"))
			if empty != (variant == "valid") {
				t.Fatalf("explicit empty inventory: %v", empty)
			}
			if _, ok := r.LineCovered(filepath.Join(root, "hit.py"), 9); ok {
				t.Fatal("inventory changed measured sibling")
			}
			if r.Has(filepath.Join(root, "unrelated.py")) {
				t.Fatal("unrelated file admitted")
			}
			for _, language := range []string{"go", "kotlin"} {
				r.Prove(language, root, []string{filepath.Join(root, "other.py")})
				if f := m.Inventory(t.Context(), r, language); f != nil {
					t.Fatal(f)
				}
				if _, ok := r.Lines(filepath.Join(root, "other.py")); ok {
					t.Fatal("invented static collector")
				}
			}
		})
	}
}

func TestListedConversionRetainsSiblingCollectorsAndActualArtifact(t *testing.T) {
	m, root, sources := fixture(t, "go", map[string]string{"a.go": "source", "a.py": "source"}, Measurement{Report: "mode: set\nexample/a.go:2.1,2.9 1 1\n"})
	goFile, pyFile := filepath.Join(root, "a.go"), filepath.Join(root, "a.py")
	d := m.Declarations[coverage.Unloaded{Language: "go", Dir: root}]
	d.Metadata = []byte("independent opaque covmeta bytes")
	d.ConversionExit = 1
	d.ConversionOutput = "external conversion failed"
	d.Split = true
	d.Listed = map[string]map[string]*coverage.Report{"go": {"A": coverage.FromLines("go", map[string][]coverage.Line{goFile: {{Line: 2, Covered: true}}})}, "python": {"A": coverage.FromLines("python", map[string][]coverage.Line{pyFile: {{Line: 3, Covered: true}}}), "B": coverage.FromLines("python", map[string][]coverage.Line{pyFile: {{Line: 4, Covered: true}}})}}
	m.Declarations[coverage.Unloaded{Language: "go", Dir: root}] = d
	p := &coverage.PerTest{Root: root, Tests: []coverage.Test{{ID: "A"}, {ID: "B"}}, All: "all", Select: func(ids []string) string { return strings.Join(ids, ",") }}
	var log bytes.Buffer
	m.Log = &log
	r, failure := m.MeasureListed(t.Context(), port.Request{Sources: sources}, p)
	if failure != nil {
		t.Fatal(failure)
	}
	if got := r.LineTests(goFile, 2); len(got) != 0 {
		t.Fatalf("failed Go converter kept %v", got)
	}
	if !reflect.DeepEqual(r.LineTests(pyFile, 3), []string{"A"}) || !reflect.DeepEqual(r.LineTests(pyFile, 4), []string{"B"}) {
		t.Fatal("sibling collector/ID lost")
	}
	artifact := filepath.Join(root, ".metrics", "coverage", "virtual-listed", "A", "covmeta.fixture")
	want := sha256.Sum256(d.Metadata)
	if m.Aggregate.Files[artifact] != string(d.Metadata) || m.Conversions[artifact] != hex.EncodeToString(want[:]) {
		t.Fatal("conversion not tied to actual artifact")
	}
	if hit, ok := r.LineCovered(goFile, 2); !hit || !ok {
		t.Fatal("aggregate coverage lost")
	}
	if !strings.Contains(log.String(), "external conversion failed") {
		t.Fatal("failed converter not logged")
	}
	d.ListedFailed = true
	m.Declarations[coverage.Unloaded{Language: "go", Dir: root}] = d
	// A failed run of every listed test is the measurement's failure, as
	// the real measurer's (slice-15), and gives no report.
	r, failure = m.MeasureListed(t.Context(), port.Request{Sources: sources}, p)
	if failed, ok := failure.(port.TestsFailed); !ok || r != nil || failed.Command != "all" || failed.ExitCode != 1 {
		t.Fatalf("collection failure: %v %T %v", r, failure, failure)
	}
}

func TestMeasurementDeclarationRejectsMalformedFactsAndCancellation(t *testing.T) {
	for _, d := range []Measurement{{Report: "nonsense"}, {Report: "SF:a.py\nDA:bad,1\n"}, {IntegrationReport: "<broken"}, {InventoryReport: "invalid"}, {Language: "python", WithoutC8: true}} {
		if d.Validate() == nil {
			t.Fatalf("accepted malformed declaration %+v", d)
		}
	}
	m, _, sources := fixture(t, "python", map[string]string{"a.py": "source"}, Measurement{Report: "SF:a.py\nDA:1,1\nend_of_record\n"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, failure := m.Measure(ctx, port.Request{Sources: sources}); failure == nil {
		t.Fatal("cancelled measurement accepted")
	}
	if failure := m.Inventory(ctx, &coverage.Report{}, "python"); failure == nil {
		t.Fatal("cancelled inventory accepted")
	}
}

func TestListedCollectionIsOnceAcrossActualPlanGroups(t *testing.T) {
	m, root, sources := fixture(t, "python", map[string]string{"a.py": "source"}, Measurement{Report: "SF:a.py\nDA:2,1\nend_of_record\n"})
	nested := filepath.Join(root, "nested")
	goFile := filepath.Join(nested, "a.go")
	m.Aggregate.Files[goFile] = "source"
	sources = append(sources, goFile)
	pyFile := filepath.Join(root, "a.py")
	py := m.Declarations[coverage.Unloaded{Language: "python", Dir: root}]
	py.Listed = map[string]map[string]*coverage.Report{"python": {"A": coverage.FromLines("python", map[string][]coverage.Line{pyFile: {{Line: 2, Covered: true}}})}}
	m.Declarations[coverage.Unloaded{Language: "python", Dir: root}] = py
	m.Declarations[coverage.Unloaded{Language: "go", Dir: nested}] = Measurement{Language: "go", Report: "mode: set\nexample/a.go:2.1,2.9 1 1\n", Listed: map[string]map[string]*coverage.Report{"go": {"A": coverage.FromLines("go", map[string][]coverage.Line{goFile: {{Line: 2, Covered: true}}})}}}
	p := &coverage.PerTest{Root: root, Tests: []coverage.Test{{ID: "A"}, {ID: "B"}}, All: "all", Select: func(ids []string) string { return strings.Join(ids, ",") }}
	r, failure := m.MeasureListed(t.Context(), port.Request{Sources: sources}, p)
	if failure != nil {
		t.Fatal(failure)
	}
	if !reflect.DeepEqual(m.Commands, []string{"all", "A", "B"}) {
		t.Fatalf("multiplied collector commands: %v", m.Commands)
	}
	if !reflect.DeepEqual(r.LineTests(pyFile, 2), []string{"A"}) || !reflect.DeepEqual(r.LineTests(goFile, 2), []string{"A"}) {
		t.Fatal("sibling plan's ID report lost")
	}
	m.Commands = nil
	for group, d := range m.Declarations {
		d.Listed = nil
		m.Declarations[group] = d
	}
	r, failure = m.MeasureListed(t.Context(), port.Request{Sources: sources}, p)
	if failure != nil || len(m.Commands) != 0 || len(r.LineTests(pyFile, 2)) != 0 {
		t.Fatal("undeclared collector invented execution/evidence")
	}
}

func TestMemoryMutationMeasurerContract(t *testing.T) {
	MutationMeasurerContract(t, func(t *testing.T, files map[string]string) (coverage.MutationMeasurer, string) {
		root := t.TempDir()
		held := map[string]string{}
		for path, text := range files {
			held[filepath.Join(root, filepath.FromSlash(path))] = text
		}
		return &Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: held}}, root
	})
}

func TestMemoryListedContract(t *testing.T) {
	ListedContract(t, func(t *testing.T, split bool, fail string) (coverage.MutationMeasurer, string) {
		m, root, _ := fixture(t, "go", map[string]string{"main.go": ListedMain}, Measurement{Report: "mode: set\n"})
		file := filepath.Join(root, "main.go")
		d := m.Declarations[coverage.Unloaded{Language: "go", Dir: root}]
		d.Split = split
		switch fail {
		case "":
		case "all":
			d.ListedFailed = true
		default:
			d.Failing = []string{fail}
		}
		d.Listed = map[string]map[string]*coverage.Report{"go": {"A": coverage.FromLines("go", map[string][]coverage.Line{file: {{Line: 5, Covered: true}}}), "B": coverage.FromLines("go", map[string][]coverage.Line{file: {{Line: 7, Covered: true}}})}}
		m.Declarations[coverage.Unloaded{Language: "go", Dir: root}] = d
		return m, root
	})
}
func TestMemoryStaticInventoryContract(t *testing.T) {
	StaticInventoryContract(t, func(t *testing.T) (coverage.MutationMeasurer, string) {
		m, root, _ := fixture(t, "python", map[string]string{"hit.py": "hit=1\n", "miss.py": "def hidden():\n    return 1\n", "empty.py": "# no statements\n"}, Measurement{Report: "TN:\n", InventoryReport: "SF:miss.py\nDA:1,0\nDA:2,0\nend_of_record\nSF:empty.py\nend_of_record\n"})
		return m, root
	})
}

func TestFailedInventoryWithoutReportStillLogsItsActualDiagnostic(t *testing.T) {
	m, root, sources := fixture(t, "python", map[string]string{"hit.py": "source", "miss.py": "source"}, Measurement{Report: "SF:hit.py\nDA:1,1\nend_of_record\n", InventoryExit: 1, InventoryOutput: "collector entered then failed before report"})
	var log bytes.Buffer
	m.Log = &log
	r, failure := m.Measure(t.Context(), port.Request{Sources: sources})
	if failure != nil {
		t.Fatal(failure)
	}
	if failure := m.Inventory(t.Context(), r, "python"); failure != nil {
		t.Fatal(failure)
	}
	if _, ok := r.Lines(filepath.Join(root, "miss.py")); ok {
		t.Fatal("failed collector supplied proof")
	}
	if !strings.Contains(log.String(), "collector entered then failed before report") {
		t.Fatalf("actual failed collector diagnostic lost: %q", log.String())
	}
}

// A collector whose command succeeds prints what its tools print, as the
// real collectors pass it on; one whose command fails says its failure
// instead, and an undeclared root prints nothing.
func TestMeasurementPrintsTheToolsOutputWhenItsCommandSucceeds(t *testing.T) {
	m, root, sources := fixture(t, "go", map[string]string{"a.go": "source"}, Measurement{Report: "mode: set\nexample/a.go:2.1,2.9 1 1\n", Output: "ok  \texample\t0.002s\n"})
	var log bytes.Buffer
	m.Log = &log
	if _, failure := m.Measure(t.Context(), port.Request{Sources: sources}); failure != nil {
		t.Fatal(failure)
	}
	if log.String() != "ok  \texample\t0.002s\n" {
		t.Fatalf("logged %q, want the tool's output alone", log.String())
	}
	log.Reset()
	if _, err := m.CollectFresh(t.Context(), port.Request{Sources: sources}); err != nil || log.String() != "ok  \texample\t0.002s\n" {
		t.Fatalf("a fresh collection: %v, logged %q; want the tool's output", err, log.String())
	}
	d := m.Declarations[coverage.Unloaded{Language: "go", Dir: root}]
	d.CommandExit, d.CommandOutput = 1, "FAIL example"
	m.Declarations[coverage.Unloaded{Language: "go", Dir: root}] = d
	log.Reset()
	if _, failure := m.Measure(t.Context(), port.Request{Sources: sources}); failure != nil {
		t.Fatal(failure)
	}
	if strings.Contains(log.String(), "ok  ") || !strings.Contains(log.String(), "FAIL example (exit 1)") {
		t.Fatalf("a failed command logged %q, want its failure alone", log.String())
	}
}
