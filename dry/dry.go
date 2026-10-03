// Package dry finds functions whose structure is close enough to be
// duplicates worth reviewing.
//
// Each function's syntax tree is normalized: local names, field names, and
// literal values drop out; called function names, operators, and the shape of
// the tree stay. Every node then contributes fingerprints of the subtree
// beneath it, one and two levels deep. Two functions score the Jaccard
// similarity of their fingerprint sets: shared fingerprints over all
// fingerprints seen in either.
package dry

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
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

// Form is one function ready for comparison.
type Form struct {
	Side
	Language string
	prints   []uint64 // sorted, unique
	path     string   // absolute
}

// Forms fingerprints every unit in files that is large enough to compare.
func Forms(files []string, opt Options) ([]Form, error) {
	var forms []Form
	for _, path := range files {
		f, err := lang.ParseFile(path)
		if err != nil {
			return nil, err
		}
		for _, u := range f.Units {
			if u.EndLine-u.StartLine+1 < opt.MinLines {
				continue
			}
			prints, nodes := Fingerprint(f, u.Node)
			if nodes < opt.MinNodes {
				continue
			}
			forms = append(forms, Form{
				Side: Side{
					File: project.Rel(path), StartLine: u.StartLine, EndLine: u.EndLine,
					Namespace: u.Namespace, Name: u.Name, Nodes: nodes,
				},
				Language: u.Language,
				prints:   prints,
				path:     path,
			})
		}
		f.Close()
	}
	return forms, nil
}

// Compare scores every pair of forms in the same language and returns those
// at or above the threshold, best first. When focus is non-empty, only pairs
// with at least one form in a focus file are reported, so a change can be
// checked against the whole project.
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
				if s := Jaccard(a.prints, b.prints); s >= opt.Threshold {
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

// Jaccard is |a∩b| / |a∪b| for sorted, unique slices.
func Jaccard(a, b []uint64) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	shared := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			shared++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

// depth is how many levels below a node its deepest fingerprint reaches.
// Shallow fingerprints tolerate an added statement: it changes the depth-2
// print of its parent block, but the deep prints of every ancestor would all
// change with it.
const depth = 2

// Fingerprint normalizes the tree under n and returns its fingerprint set
// and the number of nodes that survived normalization.
func Fingerprint(f *lang.File, n *sitter.Node) ([]uint64, int) {
	fp := fingerprinter{syntax: f.Spec.Syntax, src: f.Src, seen: map[uint64]bool{}}
	fp.visit(n)
	prints := make([]uint64, 0, len(fp.seen))
	for h := range fp.seen {
		prints = append(prints, h)
	}
	sort.Slice(prints, func(i, j int) bool { return prints[i] < prints[j] })
	return prints, fp.nodes
}

type fingerprinter struct {
	syntax lang.Syntax
	src    []byte
	seen   map[uint64]bool
	nodes  int
}

// punctuation carries no structure the tree does not already show.
var punctuation = map[string]bool{
	"(": true, ")": true, "{": true, "}": true, "[": true, "]": true,
	",": true, ";": true, ":": true, ".": true, "\"": true, "'": true, "`": true,
}

// visit returns the node's hashes at each depth from 0 to depth, and false
// when normalization drops the node.
func (fp *fingerprinter) visit(n *sitter.Node) ([depth + 1]uint64, bool) {
	var h [depth + 1]uint64
	label, leaf, keep := fp.label(n)
	if !keep {
		return h, false
	}
	var kids [][depth + 1]uint64
	if !leaf {
		for i := uint(0); i < n.ChildCount(); i++ {
			if kh, ok := fp.visit(n.Child(i)); ok {
				kids = append(kids, kh)
			}
		}
	}
	h[0] = hashOf(label, nil, 0)
	for d := 1; d <= depth; d++ {
		h[d] = hashOf(label, kids, d-1)
	}
	for d := 1; d <= depth; d++ {
		fp.seen[h[d]] = true
	}
	fp.nodes++
	return h, true
}

// label is what survives of n: its kind, "lit" for a literal, "id" for a
// name, or the callee's own name for a called function.
func (fp *fingerprinter) label(n *sitter.Node) (label string, leaf, keep bool) {
	kind := n.Kind()
	switch {
	case kind == "comment" || n.IsExtra():
		return "", false, false
	case fp.syntax.Literals[kind]:
		return "lit", true, true
	case !n.IsNamed():
		return kind, true, !punctuation[kind]
	case fp.syntax.Identifiers[kind]:
		if fp.syntax.IsCallee(n) {
			return "call:" + n.Utf8Text(fp.src), true, true
		}
		return "id", true, true
	}
	return kind, n.ChildCount() == 0, true
}

func hashOf(label string, kids [][depth + 1]uint64, d int) uint64 {
	h := fnv.New64a()
	h.Write([]byte(label))
	var buf [8]byte
	for _, k := range kids {
		binary.LittleEndian.PutUint64(buf[:], k[d])
		h.Write(buf[:])
	}
	// Separate a node's depths so h1 of one node never equals h2 of another.
	buf[0] = byte(d)
	h.Write(buf[:1])
	return h.Sum64()
}
