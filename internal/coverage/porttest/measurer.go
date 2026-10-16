// Package porttest holds the model-bearing coverage fake and shared contracts.
// No production domain code imports this package.
package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
)

// Measurement describes independently observed collector artifacts and results.
// Reports are actual tool-format bytes, not a desired measurement verdict.
type Measurement struct {
	Language, Report, IntegrationReport string
	CommandExit                         int
	CommandOutput                       string
	WithoutC8                           bool
	InventoryReport                     string
	InventoryExit                       int
	InventoryOutput                     string
	// Listed holds each test's measured report, independently of the request.
	Listed           map[string]map[string]*coverage.Report
	ListedFailed     bool
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
		return nil, port.Cancelled{}
	}
	if len(req.Reports) > 0 || req.Existing || m.Declarations == nil {
		return m.Aggregate.Measure(ctx, req)
	}
	if m.RunDirectoryFailure != nil {
		return nil, *m.RunDirectoryFailure
	}
	groups := m.groups(req.Sources)
	var reports []*coverage.Report
	for _, group := range orderedGroups(groups) {
		root := group.Dir
		d, declared := m.Declarations[group]
		sources := groups[group]
		r := &coverage.Report{}
		// A declared collector operation entered even if it failed or wrote
		// nothing. Execution observation is separate from successful evidence.
		if declared {
			m.Entered = append(m.Entered, group)
		}
		switch {
		case !declared:
			r.SetMissing(coverage.Unmeasured{Dir: root, Language: group.Language, Cause: port.ToolMissing{}, Reason: "no collector declared"})
		case d.CommandExit != 0:
			m.log("coverage: %s: %s (exit %d)\n", root, d.CommandOutput, d.CommandExit)
			r.SetMissing(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.MeasuredNothing{}, Reason: d.CommandOutput})
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
				r.SetIntegrationMissing(coverage.Unmeasured{Dir: root, Language: d.Language, Cause: port.ToolMissing{}, Reason: "c8 is not installed"})
			}
			r.MarkMeasured(d.Language)
			r.Prove(d.Language, root, sources)

		}
		reports = append(reports, r)
	}
	r := coverage.Merge(reports...)
	if tests != nil {
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
			if !failed && !split {
				for _, test := range tests.Tests {
					if tests.Select == nil {
						m.log("coverage: listed selection is unavailable\n")
						failed = true
						break
					}
					m.Commands = append(m.Commands, tests.Select([]string{test.ID}))
				}
			}
		}
		for _, group := range orderedGroups(groups) {
			d := m.Declarations[group]
			failed = failed || d.ListedFailed
			if d.ListedFailed {
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
		if declared && !failed {
			tc := &coverage.TestCoverage{}
			for _, test := range tests.Tests {
				if reports := byID[test.ID]; len(reports) > 0 {
					tc.Add(test.ID, coverage.Merge(reports...))
				}
			}
			r.SetTests(tc)
		}
	}
	if ctx.Err() != nil {
		return nil, port.Cancelled{}
	}
	return r, nil
}

func (m *Measurer) badArtifact(root string, err error) *coverage.Report {
	r := &coverage.Report{}
	r.SetMissing(coverage.Unmeasured{Dir: root, Cause: port.Unreadable{}, Reason: err.Error()})
	return r
}

func (m *Measurer) Inventory(ctx context.Context, report *coverage.Report, language string) port.Failure {
	if ctx.Err() != nil {
		return port.Cancelled{}
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
		return port.Cancelled{}
	}
	return nil
}

func (m *Measurer) log(format string, args ...any) {
	if m.Log != nil {
		_, _ = fmt.Fprintf(m.Log, format, args...)
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
