package scrap

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/dry"
	"github.com/donvargax/itos-cc/lang"
)

// Actions tell an assistant how far it may go on its own.
const (
	LeaveAlone     = "LEAVE_ALONE"      // fine as it is; change it only if asked
	AutoTableDrive = "AUTO_TABLE_DRIVE" // safe to fold repeated examples into a table
	AutoRefactor   = "AUTO_REFACTOR"    // safe to clean up examples in place
	ManualSplit    = "MANUAL_SPLIT"     // split the file by responsibility before anything else
	ReviewFirst    = "REVIEW_FIRST"     // not stable, but not clear enough to change unattended
)

// ExampleReport is one example's measurements and score. A clean example
// scores 1; everything above is pressure to change it.
type ExampleReport struct {
	Name      string   `json:"name"`
	Group     string   `json:"group,omitempty"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Table     bool     `json:"table,omitempty"`
	Score     float64  `json:"score"`
	Smells    []string `json:"smells,omitempty"`
	Metrics
}

// Cluster is a set of examples whose bodies have the same structure.
type Cluster struct {
	// Kind is "table" for small, logic-free repeats that read as a coverage
	// matrix, and "duplication" for repeated scaffolding worth a helper.
	Kind       string    `json:"kind"`
	Similarity float64   `json:"similarity"`
	Examples   []Located `json:"examples"`
}

// Located names an example and its lines.
type Located struct {
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Recommendation is one concrete move, most confident first.
type Recommendation struct {
	Confidence string `json:"confidence"` // HIGH, MEDIUM, or LOW
	Text       string `json:"text"`
}

// FileReport is the verdict on one test file.
type FileReport struct {
	File            string           `json:"file"`
	Language        string           `json:"language"`
	Action          string           `json:"action"`
	Pressure        float64          `json:"pressure"` // total excess score; lower is better
	Examples        int              `json:"examples"`
	SetupLines      int              `json:"setup_lines"`
	AverageScore    float64          `json:"average_score"`
	MaxScore        float64          `json:"max_score"`
	Recommendations []Recommendation `json:"recommendations"`
	Clusters        []Cluster        `json:"clusters"`
	Details         []ExampleReport  `json:"details"`
	Compare         *Comparison      `json:"compare,omitempty"`
}

// Comparison is how a file moved since the previous snapshot.
type Comparison struct {
	Verdict          string  `json:"verdict"` // improved, worse, or unchanged
	PreviousPressure float64 `json:"previous_pressure"`
	Delta            float64 `json:"delta"`
}

// Thresholds behind the smells. They are deliberately round numbers; a
// smell is a prompt to look, not a rule.
const (
	largeLines      = 30
	mockHeavy       = 3
	assertionHeavy  = 10
	hotScore        = 6.0
	similar         = 0.7  // bodies this alike are related
	tableSimilar    = 0.85 // small examples this alike differ only in data
	tableMaxLines   = 12
	duplicationSize = 40 // normalized nodes before shared scaffolding is worth a helper
)

// Score is the excess structure of one example: size beyond a screenful,
// every branch or loop, mocks beyond a couple, and missing or piled-up
// assertions.
func Score(m Metrics) float64 {
	s := 1.0
	s += math.Max(0, float64(m.Lines-15)) / 5
	s += 2.5 * float64(m.Decisions)
	s += 1.5 * math.Max(0, float64(m.Mocks-2))
	if m.Assertions == 0 {
		s += 6
	}
	s += math.Max(0, float64(m.Assertions-assertionHeavy)) / 2
	return round(s)
}

func smells(m Metrics) []string {
	var out []string
	if m.Assertions == 0 {
		out = append(out, "no-assertions")
	}
	if m.Lines > largeLines {
		out = append(out, "large")
	}
	if m.Decisions > 0 {
		out = append(out, "logic")
	}
	if m.Mocks >= mockHeavy {
		out = append(out, "mock-heavy")
	}
	if m.Assertions > assertionHeavy {
		out = append(out, "assertion-heavy")
	}
	return out
}

// Analyze measures one parsed test file. rel names it in the report.
func Analyze(f *lang.File, rel string) FileReport {
	examples, setup := Extract(f)
	r := FileReport{File: rel, Language: f.Spec.Name, Examples: len(examples), SetupLines: setup,
		Recommendations: []Recommendation{}, Clusters: []Cluster{}, Details: []ExampleReport{}}
	var total float64
	measurer := newMeasurer(f)
	for _, ex := range examples {
		m, loop := measurer.measure(ex)
		er := ExampleReport{Name: ex.Name, Group: ex.Group, StartLine: ex.StartLine, EndLine: ex.EndLine,
			Table: ex.Table || loop, Metrics: m, Score: Score(m), Smells: smells(m)}
		r.Details = append(r.Details, er)
		total += er.Score
		r.MaxScore = math.Max(r.MaxScore, er.Score)
		r.Pressure += er.Score - 1
	}
	if len(examples) > 0 {
		r.AverageScore = round(total / float64(len(examples)))
	}
	r.Clusters = clusters(f, examples, r.Details)
	for _, c := range r.Clusters {
		r.Pressure += clusterWeight(c) * float64(len(c.Examples)-1)
	}
	r.Pressure = round(r.Pressure)
	r.Action = action(r)
	r.Recommendations = recommend(r)
	return r
}

// clusters links examples whose bodies are structurally similar and keeps
// the groups that mean something: coverage matrices and repeated
// scaffolding.
func clusters(f *lang.File, examples []Example, reports []ExampleReport) []Cluster {
	prints := make([][]uint64, len(examples))
	nodes := make([]int, len(examples))
	for i, ex := range examples {
		if ex.Body != nil {
			prints[i], nodes[i] = dry.Fingerprint(f, ex.Body)
		}
	}
	parent := make([]int, len(examples))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range examples {
		for j := i + 1; j < len(examples); j++ {
			if len(prints[i]) == 0 || len(prints[j]) == 0 {
				continue
			}
			if dry.Jaccard(prints[i], prints[j]) >= similar {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]int{}
	for i := range examples {
		groups[find(i)] = append(groups[find(i)], i)
	}
	var out []Cluster
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		var sum float64
		var pairs int
		for a := 0; a < len(members); a++ {
			for b := a + 1; b < len(members); b++ {
				sum += dry.Jaccard(prints[members[a]], prints[members[b]])
				pairs++
			}
		}
		similarity := sum / float64(pairs)
		kind := classify(members, reports, nodes, similarity)
		if kind == "" {
			continue
		}
		c := Cluster{Kind: kind, Similarity: round(similarity)}
		for _, i := range members {
			c.Examples = append(c.Examples, Located{reports[i].Name, reports[i].StartLine, reports[i].EndLine})
		}
		sort.Slice(c.Examples, func(a, b int) bool { return c.Examples[a].StartLine < c.Examples[b].StartLine })
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Examples[0].StartLine < out[b].Examples[0].StartLine })
	return out
}

func classify(members []int, reports []ExampleReport, nodes []int, similarity float64) string {
	small := len(members) >= 3 && similarity >= tableSimilar
	totalNodes := 0
	for _, i := range members {
		r := reports[i]
		if r.Lines > tableMaxLines || r.Decisions > 0 || r.Table {
			small = false
		}
		totalNodes += nodes[i]
	}
	switch {
	case small:
		return "table"
	case totalNodes/len(members) >= duplicationSize:
		return "duplication"
	}
	return ""
}

// clusterWeight is the pressure each repeat adds. Moderately similar
// examples are normal arrange-act-assert tests with different data: they are
// worth a hint, not a refactor.
func clusterWeight(c Cluster) float64 {
	switch {
	case c.Kind == "table":
		return 0.5
	case c.Similarity >= tableSimilar:
		return 2
	}
	return 0
}

func action(r FileReport) string {
	var hot, mocked int
	groups := map[string]bool{}
	for _, e := range r.Details {
		if e.Score >= hotScore || e.Assertions == 0 {
			hot++
			groups[e.Group] = true
		}
		if e.Mocks >= mockHeavy {
			mocked++
		}
	}
	var tables, dups int
	for _, c := range r.Clusters {
		switch {
		case c.Kind == "table":
			tables++
		case clusterWeight(c) > 0:
			dups++
		}
	}
	switch {
	case r.Examples >= 25 && len(groups) >= 3 && hot >= 5:
		return ManualSplit
	case r.Examples >= 4 && mocked*2 > r.Examples:
		return ReviewFirst
	case hot > 0 || dups > 0:
		return AutoRefactor
	case tables > 0:
		return AutoTableDrive
	}
	return LeaveAlone
}

var tableIdiom = map[string]string{
	"typescript": "it.each / test.each",
	"python":     "@pytest.mark.parametrize",
	"go":         "a table of cases run with t.Run",
	"kotlin":     "@ParameterizedTest",
}

const maxRecommendations = 10

func recommend(r FileReport) []Recommendation {
	var high, medium, low []Recommendation
	add := func(list *[]Recommendation, level, format string, args ...any) {
		*list = append(*list, Recommendation{level, fmt.Sprintf(format, args...)})
	}
	if r.Action == ManualSplit {
		add(&low, "LOW", "split this file by responsibility before polishing examples, then rerun scrap on each part")
	}
	for _, e := range r.Details {
		where := fmt.Sprintf("%q (lines %d-%d)", e.Name, e.StartLine, e.EndLine)
		if e.Assertions == 0 {
			add(&high, "HIGH", "%s asserts nothing: it passes whatever the code does; add an assertion", where)
		}
		if e.Lines > largeLines {
			add(&high, "HIGH", "%s is %d lines: split it into examples that each check one behavior", where, e.Lines)
		}
		if e.Decisions > 0 {
			add(&medium, "MEDIUM", "%s takes %s: a test should not need logic; split the cases",
				where, plural(e.Decisions, "branch or loop", "branches or loops"))
		}
		if e.Mocks >= mockHeavy {
			add(&medium, "MEDIUM", "%s sets up %d mocks: test through a fake or a real collaborator", where, e.Mocks)
		}
		if e.Assertions > assertionHeavy {
			add(&medium, "MEDIUM", "%s makes %d assertions: it checks several behaviors; split it", where, e.Assertions)
		}
	}
	for _, c := range r.Clusters {
		names := located(c.Examples)
		if c.Kind == "table" {
			add(&high, "HIGH", "%d examples differ only in their data (%.0f%% alike): fold them into %s: %s",
				len(c.Examples), c.Similarity*100, tableIdiom[r.Language], names)
		} else if clusterWeight(c) > 0 {
			add(&medium, "MEDIUM", "%d examples repeat the same scaffolding (%.0f%% alike): extract a helper or fixture: %s",
				len(c.Examples), c.Similarity*100, names)
		} else {
			add(&low, "LOW", "%d examples share most of their structure (%.0f%% alike); a shared helper may read better: %s",
				len(c.Examples), c.Similarity*100, names)
		}
	}
	all := append(append(high, medium...), low...)
	if len(all) > maxRecommendations {
		all = all[:maxRecommendations]
	}
	if all == nil {
		all = []Recommendation{}
	}
	return all
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func located(examples []Located) string {
	var parts []string
	for _, e := range examples {
		parts = append(parts, fmt.Sprintf("%q (%d-%d)", e.Name, e.StartLine, e.EndLine))
	}
	return strings.Join(parts, ", ")
}

// CompareTo sets r.Compare from the previous report of the same file.
func (r *FileReport) CompareTo(prev FileReport) {
	delta := round(r.Pressure - prev.Pressure)
	verdict := "unchanged"
	switch {
	case delta <= -0.5:
		verdict = "improved"
	case delta >= 0.5:
		verdict = "worse"
	}
	r.Compare = &Comparison{Verdict: verdict, PreviousPressure: prev.Pressure, Delta: delta}
}

func round(v float64) float64 { return math.Round(v*10) / 10 }
