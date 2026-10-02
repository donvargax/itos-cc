package graph

import "math"

// Metrics summarize a node for coloring. A module is as risky as its worst
// function: one untested tangle is the thing to fix. A directory or
// repository is graded by the share of its functions at each risk level, so
// one bad file does not paint a whole system red.
type Metrics struct {
	Functions int `json:"functions"`
	// Mutated counts functions with mutation results, so a mutation score
	// says how much of the code it speaks for.
	Mutated    int      `json:"mutated"`
	Bands      Bands    `json:"crap_bands"`
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

// Bands count functions by CRAP: low (≤5), medium (5–30), high (>30), and
// unknown (no coverage measured).
type Bands struct {
	Low     int `json:"low"`
	Medium  int `json:"medium"`
	High    int `json:"high"`
	Unknown int `json:"unknown"`
}

func (b *Bands) add(crap *float64) {
	switch {
	case crap == nil:
		b.Unknown++
	case *crap <= 5:
		b.Low++
	case *crap < 30:
		b.Medium++
	default:
		b.High++
	}
}

func (b *Bands) merge(o Bands) {
	b.Low += o.Low
	b.Medium += o.Medium
	b.High += o.High
	b.Unknown += o.Unknown
}

// bandGrade grades a container by how much of its measured code is risky:
// a high-risk function counts fully and a medium one partly, and half the
// measured code being risky is already the worst grade.
func bandGrade(b Bands) (float64, bool) {
	measured := b.Low + b.Medium + b.High
	if measured == 0 {
		return 0, false
	}
	risk := (float64(b.High) + 0.3*float64(b.Medium)) / float64(measured)
	return round1(10 - 9*math.Min(1, 2*risk)), true
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

// metrics summarizes a node from its own functions, if it is a module, and
// its children: a package directory can be both.
func (g *repoGraph) metrics(n *Node) *Metrics {
	m := &Metrics{}
	for _, u := range n.Units {
		m.Functions++
		m.Bands.add(u.CRAP)
		if u.CRAP != nil {
			v := *u.CRAP
			m.MaxCRAP = maxPtr(m.MaxCRAP, &v)
		}
		if u.Mutated {
			m.Mutated++
		}
		m.Killed += u.Killed
		m.Survived += u.Survived
		m.Uncovered += u.Uncovered
		if u.Stale {
			m.Stale++
		}
		m.Duplicates += u.Duplicates
	}
	for _, c := range n.children {
		cm := g.metrics(c)
		m.Functions += cm.Functions
		m.Mutated += cm.Mutated
		m.Bands.merge(cm.Bands)
		m.Killed += cm.Killed
		m.Survived += cm.Survived
		m.Uncovered += cm.Uncovered
		m.Stale += cm.Stale
		m.Duplicates += cm.Duplicates
		m.MaxCRAP = maxPtr(m.MaxCRAP, cm.MaxCRAP)
	}
	if n.Kind == "module" && m.MaxCRAP != nil && len(n.children) == 0 {
		v := crapGrade(*m.MaxCRAP)
		m.CRAPGrade = &v
	} else if v, ok := bandGrade(m.Bands); ok {
		m.CRAPGrade = &v
	}
	if m.Killed+m.Survived > 0 {
		v := mutationGrade(m.Killed, m.Survived)
		m.MutationGrade = &v
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
