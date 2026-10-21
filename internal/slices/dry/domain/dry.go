// Package domain is the dry command's domain, finding functions whose
// structure is close enough to be duplicates worth reviewing. It is the dry
// slice's own (ADR-0051), kept apart from its CLI and application code, and
// a domain (decision 37): it takes files internal/lang has parsed, with the
// names its caller gives them, and reads nothing itself.
//
// Two functions score the similarity of their structure as internal/lang
// fingerprints it (lang.Fingerprint, lang.Jaccard): local names, field
// names, and literal values drop out; called function names, operators, and
// the shape of the tree stay.
package domain

import (
	"fmt"
	"slices"
	"sort"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Options bound which functions are compared and which pairs are reported.
type Options struct {
	Threshold float64 // minimum similarity, 0–1
	MinLines  int     // smaller functions are not candidates
	MinNodes  int     // functions with fewer normalized nodes are not candidates
}

// Defaults report pairs that are clearly the same code with names changed.
var Defaults = Options{Threshold: 0.82, MinLines: 4, MinNodes: 20}

// Side is one function of a candidate pair.
type Side struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
}

// Candidate is a pair of functions in the same language that score at least
// the threshold.
type Candidate struct {
	Score    float64 `json:"score"`
	Language string  `json:"language"`
	Left     Side    `json:"left"`
	Right    Side    `json:"right"`
}

// Group is functions linked by candidate pairs: each is similar to at least
// one other member. MinScore and MaxScore bound the pairs that link them.
type Group struct {
	Language string  `json:"language"`
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
	Members  []Side  `json:"members"`
}

// Groups joins candidates that share a function, so four copies of one
// helper read as one group of four rather than six pairs. Groups come best
// first, and members in file and line order.
func Groups(candidates []Candidate) []Group {
	parent := map[string]string{}
	var find func(k string) string
	find = func(k string) string {
		if parent[k] != k {
			parent[k] = find(parent[k])
		}
		return parent[k]
	}
	sides := map[string]Side{}
	for _, c := range candidates {
		for _, s := range []Side{c.Left, c.Right} {
			if _, ok := parent[key(s)]; !ok {
				parent[key(s)], sides[key(s)] = key(s), s
			}
		}
		parent[find(key(c.Left))] = find(key(c.Right))
	}
	byRoot := map[string]*Group{}
	var out []*Group
	for _, c := range candidates {
		root := find(key(c.Left))
		g, ok := byRoot[root]
		if !ok {
			g = &Group{Language: c.Language, MinScore: c.Score, MaxScore: c.Score}
			byRoot[root] = g
			out = append(out, g)
		}
		g.MinScore, g.MaxScore = min(g.MinScore, c.Score), max(g.MaxScore, c.Score)
	}
	keys := make([]string, 0, len(sides))
	for k := range sides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := byRoot[find(k)]
		g.Members = append(g.Members, sides[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.MaxScore != b.MaxScore {
			return a.MaxScore > b.MaxScore
		}
		if len(a.Members) != len(b.Members) {
			return len(a.Members) > len(b.Members)
		}
		return key(a.Members[0]) < key(b.Members[0])
	})
	groups := make([]Group, len(out))
	for i, g := range out {
		groups[i] = *g
	}
	return groups
}

// Form is one function ready for comparison.
type Form struct {
	Side
	Language string
	prints   []uint64 // sorted, unique
	path     string   // the file's path as it was parsed (lang.File.Path)
}

// Forms fingerprints every unit of f, a parsed file, that is large enough to
// compare, each Side naming the file name. The caller gives the name a
// person reads, since it is the key Groups, Compare and the pairs sort and
// pair by, and keeps f open until Forms returns; the forms hold nothing of
// its tree.
func Forms(f *lang.File, name string, opt Options) []Form {
	var forms []Form
	for _, u := range f.Units {
		if u.EndLine-u.StartLine+1 < opt.MinLines {
			continue
		}
		prints, nodes := fingerprint(f, u)
		if nodes < opt.MinNodes {
			continue
		}
		forms = append(forms, Form{
			Side: Side{
				File: name, StartLine: u.StartLine, EndLine: u.EndLine,
				Namespace: u.Namespace, Name: u.Name, Nodes: nodes,
			},
			Language: u.Language,
			prints:   prints,
			path:     f.Path,
		})
	}
	return forms
}

// fingerprint is the fingerprint set of u's code and the number of nodes
// that survived normalizing it: each of its Nodes', as a Dart unit is a
// signature and the body beside it, without its inline units.
func fingerprint(f *lang.File, u lang.Unit) ([]uint64, int) {
	var prints []uint64
	nodes := 0
	for _, n := range u.Nodes() {
		p, c := lang.Fingerprint(f, n, f.InnerNodes(u)...)
		prints = append(prints, p...)
		nodes += c
	}
	slices.Sort(prints)
	return slices.Compact(prints), nodes
}

// Compare scores every pair of forms in the same language and returns those
// at or above the threshold, best first. When focus is non-empty, only pairs
// with at least one form in a focus file, by the path its file was parsed
// as, are reported, so a change can be checked against the whole project.
func Compare(forms []Form, focus map[string]bool, opt Options) []Candidate {
	byLang := map[string][]Form{}
	for _, f := range forms {
		byLang[f.Language] = append(byLang[f.Language], f)
	}
	var out []Candidate
	for language, group := range byLang {
		sort.Slice(group, func(i, j int) bool { return len(group[i].prints) < len(group[j].prints) })
		for i := range group {
			a := group[i]
			for j := i + 1; j < len(group); j++ {
				b := group[j]
				// |A∩B| ≤ |A| and |A∪B| ≥ |B|, so once |A| < t·|B| no later,
				// larger form can reach the threshold.
				if float64(len(a.prints)) < opt.Threshold*float64(len(b.prints)) {
					break
				}
				if len(focus) > 0 && !focus[a.path] && !focus[b.path] {
					continue
				}
				if s := lang.Jaccard(a.prints, b.prints); s >= opt.Threshold {
					out = append(out, candidate(language, s, a, b))
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return key(out[i].Left)+key(out[i].Right) < key(out[j].Left)+key(out[j].Right)
	})
	return out
}

// candidate orders the pair by file and line, so the same pair always reads
// the same way.
func candidate(language string, score float64, a, b Form) Candidate {
	if key(b.Side) < key(a.Side) {
		a, b = b, a
	}
	return Candidate{Score: float64(int(score*1000+0.5)) / 1000, Language: language, Left: a.Side, Right: b.Side}
}

func key(s Side) string {
	return fmt.Sprintf("%s\x00%09d", s.File, s.StartLine)
}
