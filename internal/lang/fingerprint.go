package lang

import (
	"encoding/binary"
	"hash/fnv"
	"sort"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// A unit's structure, as the commands that compare code compare it: its
// syntax tree normalized, local names, field names and literal values
// dropping out while called function names, operators and the shape of the
// tree stay, then every node contributing fingerprints of the subtree
// beneath it, one and two levels deep. Two trees score the Jaccard
// similarity of their fingerprint sets: shared fingerprints over all
// fingerprints seen in either. dry compares functions by it and scrap a
// test's examples.

// depth is how many levels below a node its deepest fingerprint reaches.
// Shallow fingerprints tolerate an added statement: it changes the depth-2
// print of its parent block, but the deep prints of every ancestor would all
// change with it.
const depth = 2

// Fingerprint normalizes the tree under n, a node of f, without the
// subtrees in skip, and returns its fingerprint set, sorted and unique, and
// the number of nodes that survived normalization.
func Fingerprint(f *File, n *sitter.Node, skip ...*sitter.Node) ([]uint64, int) {
	fp := fingerprinter{syntax: f.Spec.Syntax, src: f.Src, seen: map[uint64]bool{}, skip: map[uintptr]bool{}}
	for _, s := range skip {
		fp.skip[s.Id()] = true
	}
	fp.visit(n)
	prints := make([]uint64, 0, len(fp.seen))
	for h := range fp.seen {
		prints = append(prints, h)
	}
	sort.Slice(prints, func(i, j int) bool { return prints[i] < prints[j] })
	return prints, fp.nodes
}

// Jaccard is |a∩b| / |a∪b| for sorted, unique slices, such as two
// fingerprint sets.
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

type fingerprinter struct {
	syntax Syntax
	src    []byte
	seen   map[uint64]bool
	skip   map[uintptr]bool // node IDs left out
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
	for d := byte(1); d <= depth; d++ {
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
	case kind == "comment" || n.IsExtra() || fp.skip[n.Id()]:
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

// hashOf is the hash of label over kids' hashes at depth d, a byte since
// it is at most depth. A hash's Write never returns an error (hash.Hash),
// so none is checked.
func hashOf(label string, kids [][depth + 1]uint64, d byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(label))
	var buf [8]byte
	for _, k := range kids {
		binary.LittleEndian.PutUint64(buf[:], k[d])
		_, _ = h.Write(buf[:])
	}
	// Separate a node's depths so h1 of one node never equals h2 of another.
	buf[0] = d
	_, _ = h.Write(buf[:1])
	return h.Sum64()
}
