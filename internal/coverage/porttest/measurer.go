// Package porttest holds the model-bearing coverage fake and shared contracts.
// No production domain code imports this package.
package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	"github.com/donvargax/itos-cc/internal/lang"
	langtest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// Measurement describes independently observed collector artifacts and results.
// Reports are actual tool-format bytes, not a desired measurement verdict.
type Measurement struct {
	Language, Report, IntegrationReport string
	CommandExit                         int
	CommandOutput                       string
	// Output is what the collector's command prints when it succeeds, as
	// its tools print it, such as go test's ok line: the fake writes it on
	// Log, where the real collectors pass a tool's output on.
	Output          string
	WithoutC8       bool
	InventoryReport string
	InventoryExit   int
	InventoryOutput string
	// Listed holds each test's measured report, independently of the request.
	Listed       map[string]map[string]*coverage.Report
	ListedFailed bool
	// ListedExit is the exit code of the failed run of every listed test,
	// 1 when it is 0.
	ListedExit int
	// ListedOutput is what the failed listed collection printed.
	ListedOutput string
	// Failing is the selection of listed IDs that fails alone: one ID of
	// it fails its own coverage run when the harness does not split.
	Failing          []string
	Split            bool
	Metadata         []byte
	ConversionExit   int
	ConversionOutput string
	// Collector artifacts and entered commands are retained by the fake below.

}

// Validate rejects malformed fixture artifacts before a command can use them.
func (d Measurement) Validate() error {
	for _, raw := range []string{d.Report, d.IntegrationReport, d.InventoryReport} {
		if raw == "" {
			continue
		}
		head := strings.TrimSpace(raw)
		if !strings.HasPrefix(head, "mode:") && !strings.HasPrefix(head, "<") && !strings.HasPrefix(head, "SF:") && !strings.HasPrefix(head, "TN:") {
			return fmt.Errorf("unsupported declared coverage format")
		}
		if _, err := coverage.Parse(strings.NewReader(raw)); err != nil {
			return err
		}
	}
	if d.WithoutC8 && (d.Language != "typescript" || d.IntegrationReport != "") {
		return fmt.Errorf("WithoutC8 requires TypeScript and no integration report")
	}
	return nil
}

// Measurer reads actual virtual files and independently declared collector facts.
// Aggregate retains named/existing report behavior; Declarations apply only to
// actual built-in measurement/inventory requests, never named reports.
type Measurer struct {
	// RunDirectoryFailure models an unavailable private collection directory.
	// Context cancellation takes precedence; no collector operation enters.
	RunDirectoryFailure *port.Unwritable
	Aggregate           aggregate.Measurer
	Declarations        map[coverage.Unloaded]Measurement
	Log                 io.Writer
	Commands            []string
	Entered             []coverage.Unloaded
	Conversions         map[string]string
}

var _ coverage.MutationMeasurer = (*Measurer)(nil)

func (m *Measurer) Measure(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	return m.MeasureListed(ctx, req, nil)
}

func (m *Measurer) MeasureListed(ctx context.Context, req port.Request, tests *coverage.PerTest) (*coverage.Report, port.Failure) {
	if ctx.Err() != nil {
		return nil, port.Cancelled{Cause: context.Cause(ctx)}
	}
	if len(req.Reports) > 0 || req.Existing || m.Declarations == nil {
		return m.Aggregate.Measure(ctx, req)
	}
	if m.RunDirectoryFailure != nil {
		return nil, *m.RunDirectoryFailure
	}
	// The sources the caller serves itself no plan measures; the listed
	// tests' coverage measures every one.
	groups := m.groups(slices.DeleteFunc(slices.Clone(req.Sources), func(s string) bool { return slices.Contains(req.Served, s) }))
	var reports []*coverage.Report
	for _, group := range orderedGroups(groups) {
		root := group.Dir
		d, declared := m.Declarations[group]
		sources := groups[group]
		r := &coverage.Report{}
		// A declared collector operation entered even if it failed or wrote
		// nothing. Execution observation is separate from successful evidence.
		if declared && !unreached(req, group, sources) {
			m.Entered = append(m.Entered, group)
			if d.CommandExit == 0 {
				m.output(d.Output)
			}
		}
		switch {
		case declared && unreached(req, group, sources):
			r.MarkMeasured(group.Language)
			r.Prove(group.Language, root, sources)
		case !declared:
			r.SetMissing(coverage.Unmeasured{Dir: root, Language: group.Language, Cause: port.ToolMissing{}, Reason: "no collector declared"})
		case d.CommandExit != 0:
			m.log("coverage: %s: %s (exit %d)\n", root, d.CommandOutput, d.CommandExit)
			timeouts := coverage.ReadGoTimeouts(d.CommandOutput)
			if len(timeouts) > 0 && d.Report != "" {
				// go test stopped a package at its limit after the report
				// was written: as the real runner, a report that measured
				// some sources is read, its root Failed, naming each.
				if failed := m.timedOut(ctx, root, sources, d, timeouts); failed != nil {
					r = failed
					break
				}
			}
			r.SetMissing(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.MeasuredNothing{}, Reason: d.CommandOutput}.WithTimeouts(timeouts))
		case d.Report == "":
			r.SetMissing(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.MeasuredNothing{}, Reason: "its coverage run measured none of its files"})
		default:
			entries, err := coverage.Parse(strings.NewReader(d.Report))
			if err != nil {
				reports = append(reports, m.badArtifact(root, err))
				continue
			}
			r = coverage.Build(ctx, m.disk(), sources, root, entries)
			if d.IntegrationReport != "" {
				integrated, err := coverage.Parse(strings.NewReader(d.IntegrationReport))
				if err != nil {
					reports = append(reports, m.badArtifact(root, err))
					continue
				}
				integrated = r.Integrated(ctx, m.disk(), sources, root, integrated)
				r = coverage.Merge(r, coverage.Build(ctx, m.disk(), sources, root, integrated))
			}
			if d.WithoutC8 {
				m.log("coverage: %s: going on without integration coverage: c8 is not installed\n", root)
				r.SetIntegrationMissing(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.ToolMissing{}, Reason: "c8 is not installed"})
			}
			r.MarkMeasured(d.Language)
			r.Prove(d.Language, root, sources)

		}
		reports = append(reports, r)
	}
	r := coverage.Merge(reports...)
	if tests != nil {
		groups := m.groups(req.Sources)
		tc, _, _ := m.listed(ctx, groups, tests)
		if failure := m.listedFailure(groups, tests); failure != nil && ctx.Err() == nil {
			return nil, failure
		}
		r.SetTests(tc)
	}

	if ctx.Err() != nil {
		return nil, port.Cancelled{Cause: context.Cause(ctx)}
	}
	return r, nil
}

// Plans are the plans MeasureListed measures req's sources by: one of each
// language's declared build root holding some, sources no test reaches of
// a Python, Kotlin or Dart root unreached; none for a request of reports.
func (m *Measurer) Plans(_ context.Context, req port.Request) []coverage.Plan {
	if len(req.Reports) > 0 || req.Existing || m.Declarations == nil {
		return nil
	}
	groups := m.groups(req.Sources)
	var plans []coverage.Plan
	for _, group := range orderedGroups(groups) {
		plans = append(plans, coverage.Plan{Language: group.Language, Dir: group.Dir, Sources: groups[group], Unreached: unreached(req, group, groups[group])})
	}
	return plans
}

// timedOut is the report d's command wrote before go test stopped
// timeouts' packages at its limit, its root Failed and naming each, when it
// measured some of sources; nil when it measured none, so the root is
// Missing. A report it cannot read is that failure's report.
func (m *Measurer) timedOut(ctx context.Context, root string, sources []string, d Measurement, timeouts []coverage.GoTimeout) *coverage.Report {
	entries, err := coverage.Parse(strings.NewReader(d.Report))
	if err != nil {
		return m.badArtifact(root, err)
	}
	r := coverage.Build(ctx, m.disk(), sources, root, entries)
	if !slices.ContainsFunc(sources, r.Has) {
		return nil
	}
	r.SetFailed(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.MeasuredNothing{},
		Reason: fmt.Sprintf("its coverage command failed (exit status %d) after writing its report, which proves nothing", d.CommandExit)}.WithTimeouts(timeouts))
	return r
}

// listedFailure is the listed tests' coverage run that failed, as the real
// measurer reports it: the run of every test whose declared collection
// fails, with its declared exit code, else, when the harness does not
// split, the first test that fails alone, with exit code 1; nil when none
// does.
func (m *Measurer) listedFailure(groups map[coverage.Unloaded][]string, tests *coverage.PerTest) port.Failure {
	declared, split := false, false
	for _, group := range orderedGroups(groups) {
		d := m.Declarations[group]
		declared, split = declared || d.Listed != nil, split || d.Split
		if d.ListedFailed {
			exit := d.ListedExit
			if exit == 0 {
				exit = 1
			}
			return port.TestsFailed{Command: tests.All, Reason: fmt.Sprintf("exit status %d", exit), ExitCode: exit}
		}
	}
	if !declared || split || tests.Select == nil {
		return nil
	}
	for _, test := range tests.Tests {
		for _, group := range orderedGroups(groups) {
			if slices.Equal(m.Declarations[group].Failing, []string{test.ID}) {
				return port.TestsFailed{Command: tests.Select([]string{test.ID}), Reason: "exit status 1", ExitCode: 1}
			}
		}
	}
	return nil
}

func (m *Measurer) badArtifact(root string, err error) *coverage.Report {
	r := &coverage.Report{}
	r.SetMissing(coverage.Unmeasured{Dir: root, Cause: port.Unreadable{}, Reason: err.Error()})
	return r
}

func (m *Measurer) Inventory(ctx context.Context, report *coverage.Report, language string) port.Failure {
	if ctx.Err() != nil {
		return port.Cancelled{Cause: context.Cause(ctx)}
	}
	if report == nil || (language != "python" && language != "typescript") {
		return nil
	}
	groups := report.UnloadedByDir(language)
	for _, root := range slices.Sorted(maps.Keys(groups)) {
		d, ok := m.Declarations[coverage.Unloaded{Language: language, Dir: root}]
		if !ok || d.Language != language {
			continue
		}
		if d.InventoryExit != 0 {
			m.log("coverage: %s: static inventory: %s (exit %d)\n", language, d.InventoryOutput, d.InventoryExit)
			continue
		}
		if d.InventoryReport == "" {
			continue
		}
		entries, err := coverage.Parse(strings.NewReader(d.InventoryReport))
		if err != nil {
			m.log("coverage: %s: static inventory: %v\n", language, err)
			continue
		}
		report.List(ctx, m.disk(), groups[root], root, entries)
	}
	if ctx.Err() != nil {
		return port.Cancelled{Cause: context.Cause(ctx)}
	}
	return nil
}

// unreached is whether req measures sources, of group's build root, by
// their own tests, which none reaches, where their language runs only the
// tests that reach them: Python's, Kotlin's and Dart's.
func unreached(req port.Request, group coverage.Unloaded, sources []string) bool {
	if req.Scope != port.OwnTests || req.AllTests || req.Reach == nil || group.Language != "python" && group.Language != "kotlin" && group.Language != "dart" {
		return false
	}
	for _, source := range sources {
		if len(req.Reach(source)) > 0 {
			return false
		}
	}
	return true
}

// relative is paths from dir, slash-separated.
func relative(dir string, paths []string) []string {
	var out []string
	for _, path := range paths {
		if rel, err := filepath.Rel(dir, path); err == nil {
			out = append(out, filepath.ToSlash(rel))
		} else {
			out = append(out, path)
		}
	}
	return out
}

// log says what the measurement does on Log, as the real collectors say
// it on itos-cc's stderr.
func (m *Measurer) log(format string, args ...any) {
	if m.Log != nil {
		_, _ = fmt.Fprintf(m.Log, "itos-cc: "+format, args...)
	}
}

// output writes text, what a tool printed, on Log as the tool printed it.
func (m *Measurer) output(text string) {
	if m.Log != nil && text != "" {
		_, _ = io.WriteString(m.Log, text)
	}
}

func (m *Measurer) groups(sources []string) map[coverage.Unloaded][]string {
	groups := map[coverage.Unloaded][]string{}
	for _, source := range sources {
		spec := lang.Detect(source)
		if spec == nil {
			continue
		}
		best := ""
		roots := slices.Collect(maps.Keys(m.Aggregate.Roots))
		for group := range m.Declarations {
			if group.Language == spec.Name {
				roots = append(roots, group.Dir)
			}
		}
		for _, root := range roots {
			rel, err := filepath.Rel(root, source)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(root) > len(best) {
				best = root
			}
		}
		if best != "" {
			g := coverage.Unloaded{Language: spec.Name, Dir: best}
			groups[g] = append(groups[g], source)
		}
	}
	return groups
}
func orderedGroups(groups map[coverage.Unloaded][]string) []coverage.Unloaded {
	out := slices.Collect(maps.Keys(groups))
	slices.SortFunc(out, func(a, b coverage.Unloaded) int {
		if n := strings.Compare(a.Language, b.Language); n != 0 {
			return n
		}
		return strings.Compare(a.Dir, b.Dir)
	})
	return out
}
func (m *Measurer) disk() aggregate.Files {
	return aggregate.Files{Dir: m.Aggregate.Dir, Regular: slices.Collect(maps.Keys(m.Aggregate.Files))}
}

// MeasureFreshTests reuses actual per-ID artifact collection. Unlike ordinary
// measurement, a failed declared collector/conversion is retained as failure.
func (m *Measurer) MeasureFreshTests(ctx context.Context, req port.Request, tests *coverage.PerTest) (*coverage.TestCoverage, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	tc, missing, err := m.listed(ctx, m.groups(req.Sources), tests)
	if err != nil {
		return tc, err
	}
	if tc == nil {
		return nil, errors.New("listed collection has no declared collector")
	}
	if len(missing) > 0 {
		return tc, fmt.Errorf("listed test %s produced no per-ID coverage data", missing[0])
	}
	return tc, nil
}
func (m *Measurer) listed(ctx context.Context, groups map[coverage.Unloaded][]string, tests *coverage.PerTest) (*coverage.TestCoverage, []string, error) {
	var listedErrors []error
	byID := map[string][]*coverage.Report{}
	failed, declared, split := false, false, false
	for _, group := range orderedGroups(groups) {
		d := m.Declarations[group]
		declared = declared || d.Listed != nil
		split = split || d.Split
		failed = failed || d.ListedFailed
	}
	if declared {
		m.Commands = append(m.Commands, tests.All)
		m.log("coverage %s$ %s\n", tests.Root, tests.All)
		if !failed && !split {
			for _, test := range tests.Tests {
				if tests.Select == nil {
					listedErrors = append(listedErrors, fmt.Errorf("listed selection is unavailable"))
					m.log("coverage: listed selection is unavailable\n")
					failed = true
					break
				}
				m.Commands = append(m.Commands, tests.Select([]string{test.ID}))
				m.log("coverage %s$ %s\n", tests.Root, tests.Select([]string{test.ID}))
			}
		}
	}
	for _, group := range orderedGroups(groups) {
		d := m.Declarations[group]
		failed = failed || d.ListedFailed
		if d.ListedFailed {
			m.log("coverage: %s: listed collection: %s\n", group.Dir, d.ListedOutput)
			listedErrors = append(listedErrors, fmt.Errorf("listed collection failed"))
			continue
		}
		for _, test := range tests.Tests {
			for _, collector := range slices.Sorted(maps.Keys(d.Listed)) {
				if collector == "go" && d.Metadata != nil {
					artifact := filepath.Join(group.Dir, ".metrics", "coverage", "virtual-listed", test.ID, "covmeta.fixture")
					if m.Aggregate.Files == nil {
						m.Aggregate.Files = map[string]string{}
					}
					m.Aggregate.Files[artifact] = string(slices.Clone(d.Metadata))
					// The external result belongs to this actual artifact read,
					// not to an opaque-byte parser or final application verdict.
					artifactBytes := m.Aggregate.Files[artifact]
					digest := sha256.Sum256([]byte(artifactBytes))
					if m.Conversions == nil {
						m.Conversions = map[string]string{}
					}
					m.Conversions[artifact] = hex.EncodeToString(digest[:])
					if d.ConversionExit != 0 {
						listedErrors = append(listedErrors, fmt.Errorf("%s Go conversion: %s (exit %d)", test.ID, d.ConversionOutput, d.ConversionExit))
						m.log("coverage: %s: Go conversion: %s (exit %d)\n", test.ID, d.ConversionOutput, d.ConversionExit)
						continue
					}
				}
				if report := d.Listed[collector][test.ID]; report != nil {
					byID[test.ID] = append(byID[test.ID], report)
				}
			}
		}
	}
	var missing []string
	var tc *coverage.TestCoverage
	if declared && !failed {
		tc = &coverage.TestCoverage{}
		for _, test := range tests.Tests {
			if reports := byID[test.ID]; len(reports) > 0 {
				tc.Add(test.ID, coverage.Merge(reports...))
			} else {
				missing = append(missing, test.ID)
			}
		}
		m.ended(split, len(tests.Tests)-len(missing), len(listedErrors) == 0, m.limited(ctx, groups))
	}
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	return tc, missing, errors.Join(listedErrors...)
}

// ended says on Log, as the real measurer does, that the listed tests'
// coverage run ended, the coverage of n of them, those with data,
// converted next, and, when converted, that the conversions ended, Go's
// limited to packages when above 0. Their durations are zero: the memory
// takes no time.
func (m *Measurer) ended(split bool, n int, converted bool, packages int) {
	ran := "ran"
	if !split {
		ran = "ran one at a time"
	}
	m.log("coverage: the listed tests %s in 0.0s; converting the coverage of %d of them\n", ran, n)
	if !converted {
		return
	}
	limit := ""
	if packages == 1 {
		limit = ", Go's limited to 1 package,"
	} else if packages > 1 {
		limit = fmt.Sprintf(", Go's limited to %d packages,", packages)
	}
	m.log("coverage: converted the coverage of %d listed tests%s in 0.0s\n", n, limit)
}

// limited is how many packages the real measurer limits the listed tests'
// Go conversions to for the sources of groups, as the memory's go.mod
// files name them: 0 when it does not limit them.
func (m *Measurer) limited(ctx context.Context, groups map[coverage.Unloaded][]string) int {
	var sources []string
	for _, group := range orderedGroups(groups) {
		sources = append(sources, groups[group]...)
	}
	packages, _ := lang.GoPackages(ctx, langtest.Surroundings{Files: m.Aggregate.Files, Dir: m.Aggregate.Dir}, sources)
	return len(packages)
}

// CollectFresh reads the same tool formats/build populations as ordinary
// measurement, retaining useful failed-command artifacts without proving them.
func (m *Measurer) CollectFresh(ctx context.Context, req port.Request) (*coverage.Report, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if m.RunDirectoryFailure != nil {
		return nil, *m.RunDirectoryFailure
	}
	var reports []*coverage.Report
	var failures []error
	// The sources the caller serves itself no plan measures.
	groups := m.groups(slices.DeleteFunc(slices.Clone(req.Sources), func(s string) bool { return slices.Contains(req.Served, s) }))
	admitted := map[string]bool{}
	for _, sources := range groups {
		for _, source := range sources {
			admitted[filepath.Clean(source)] = true
		}
	}
	for _, source := range req.Sources {
		if !admitted[filepath.Clean(source)] && !slices.Contains(req.Served, source) {
			failures = append(failures, fmt.Errorf("supervised coverage has admitted source without a measurement plan: %s", source))
		}
	}

	for _, group := range orderedGroups(groups) {
		d, declared := m.Declarations[group]
		planFailed := d.CommandExit != 0
		if !declared {
			failures = append(failures, fmt.Errorf("no collector declared for %s", group.Dir))
			continue
		}
		// A Python or Kotlin build root whose own sources no test reaches
		// runs nothing: its language is measured and none of its sources
		// loaded, as the collector's unreached plan reports them.
		if unreached(req, group, groups[group]) {
			m.log("coverage %s: no test reaches %s, so none runs\n", group.Dir, strings.Join(relative(group.Dir, groups[group]), " "))
			r := &coverage.Report{}
			r.MarkMeasured(group.Language)
			r.Prove(group.Language, group.Dir, groups[group])
			reports = append(reports, r)
			continue
		}
		m.Entered = append(m.Entered, group)
		if d.CommandExit == 0 {
			m.output(d.Output)
		}
		if d.CommandExit != 0 {
			m.log("coverage: %s: %s (exit %d)\n", group.Dir, d.CommandOutput, d.CommandExit)
			failures = append(failures, fmt.Errorf("%s coverage command: %s (exit %d)", group.Language, d.CommandOutput, d.CommandExit))
		}
		if d.Report == "" {
			failures = append(failures, fmt.Errorf("%s: coverage measured none of its admitted sources", group.Language))
			continue
		}
		entries, err := coverage.Parse(strings.NewReader(d.Report))
		if err != nil {
			failures = append(failures, err)
			continue
		}
		r := coverage.Build(ctx, m.disk(), groups[group], group.Dir, entries)
		if d.IntegrationReport != "" {
			entries, err := coverage.Parse(strings.NewReader(d.IntegrationReport))
			if err != nil {
				planFailed = true
				failures = append(failures, err)
			} else {
				entries = r.Integrated(ctx, m.disk(), groups[group], group.Dir, entries)
				r = coverage.Merge(r, coverage.Build(ctx, m.disk(), groups[group], group.Dir, entries))
			}
		}
		if d.WithoutC8 {
			r.SetIntegrationMissing(coverage.Unmeasured{Dir: group.Dir, Language: group.Language, Cause: port.ToolMissing{}, Reason: "c8 is not installed"})
		}
		for _, missing := range r.Missing() {
			failures = append(failures, fmt.Errorf("coverage report: %s", missing.String()))
		}
		if r.Empty() && len(groups[group]) > 0 {
			failures = append(failures, fmt.Errorf("%s: coverage measured none of its admitted sources", group.Language))
		}
		if !planFailed && len(r.Missing()) == 0 && !r.Empty() {
			r.MarkMeasured(group.Language)
			r.Prove(group.Language, group.Dir, groups[group])
		}
		reports = append(reports, r)
	}
	if ctx.Err() != nil {
		failures = append(failures, context.Cause(ctx))
	}
	return coverage.Merge(reports...), errors.Join(failures...)
}
