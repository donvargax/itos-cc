package graph

import "math"

// Metrics summarize a node for coloring: a module from its functions, a
// directory or repository from the worst of what it contains.
type Metrics struct {
	Functions  int      `json:"functions"`
	MaxCRAP    *float64 `json:"max_crap,omitempty"`
	Killed     int      `json:"killed"`
	Survived   int      `json:"survived"`
	Uncovered  int      `json:"uncovered"`
	Stale      int      `json:"stale"` // functions whose mutation results predate their source
	Duplicates int      `json:"duplicates"`
	// Grades run from 1 (worst) to 10 (best); nil means not measured.
	CRAPGrade     *float64 `json:"crap_grade,omitempty"`
	MutationGrade *float64 `json:"mutation_grade,omitempty"`
	Grade         *float64 `json:"grade,omitempty"`
}

// crapGrade maps the worst CRAP score onto 10 (≤5, low risk) down to 1
// (≥30, complex and under-tested).
func crapGrade(worst float64) float64 {
	switch {
	case worst <= 5:
		return 10
	case worst >= 30:
		return 1
	}
	return round1(10 - 9*(worst-5)/25)
}

// mutationGrade maps the share of killed mutants onto 1–10.
func mutationGrade(killed, survived int) float64 {
	return round1(1 + 9*float64(killed)/float64(killed+survived))
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// summarize fills Metrics bottom-up.
func (g *repoGraph) summarize() {
	g.metrics(g.nodes[g.name])
}

// metrics combines a node's own functions, if it is a module, with the worst
// of its children: a package directory can be both.
func (g *repoGraph) metrics(n *Node) *Metrics {
	m := &Metrics{}
	for _, u := range n.Units {
		m.Functions++
		if u.CRAP != nil {
			v := *u.CRAP
			m.MaxCRAP = maxPtr(m.MaxCRAP, &v)
		}
		m.Killed += u.Killed
		m.Survived += u.Survived
		m.Uncovered += u.Uncovered
		if u.Stale {
			m.Stale++
		}
		m.Duplicates += u.Duplicates
	}
	if m.MaxCRAP != nil {
		v := crapGrade(*m.MaxCRAP)
		m.CRAPGrade = &v
	}
	if m.Killed+m.Survived > 0 {
		v := mutationGrade(m.Killed, m.Survived)
		m.MutationGrade = &v
	}
	for _, c := range n.children {
		cm := g.metrics(c)
		m.Functions += cm.Functions
		m.Killed += cm.Killed
		m.Survived += cm.Survived
		m.Uncovered += cm.Uncovered
		m.Stale += cm.Stale
		m.Duplicates += cm.Duplicates
		m.MaxCRAP = maxPtr(m.MaxCRAP, cm.MaxCRAP)
		m.CRAPGrade = minPtr(m.CRAPGrade, cm.CRAPGrade)
		m.MutationGrade = minPtr(m.MutationGrade, cm.MutationGrade)
	}
	switch {
	case m.CRAPGrade != nil && m.MutationGrade != nil:
		v := round1((*m.CRAPGrade + *m.MutationGrade) / 2)
		m.Grade = &v
	case m.CRAPGrade != nil:
		m.Grade = m.CRAPGrade
	case m.MutationGrade != nil:
		m.Grade = m.MutationGrade
	}
	n.Metrics = m
	return m
}

func maxPtr(a, b *float64) *float64 {
	if a == nil || (b != nil && *b > *a) {
		return b
	}
	return a
}

func minPtr(a, b *float64) *float64 {
	if a == nil || (b != nil && *b < *a) {
		return b
	}
	return a
}
