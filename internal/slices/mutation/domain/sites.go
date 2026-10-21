// Package domain owns pure mutation rules: it makes small changes to functions
// one at a time and checks that the tests notice. A mutant is killed when the
// tests fail or time out, and survives when they still pass. A mutant on a
// line no test executes is uncovered and is not run.
//
// Results are cached per function in .metrics/mutate/, keyed by a hash of the
// function's code, its comments left out, so a later run only retries
// survivors and the sites of functions whose code changed.
package domain

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Site is one mutation: a byte range of the source and its replacement.
type Site struct {
	Unit        int // index into the file's units
	Start, End  uint
	Offset      int // start relative to the unit, stable while the unit's text is
	Line        int
	Column      int
	Original    string
	Replacement string
}

// Key identifies a site within its unit across runs.
func (s Site) Key() string {
	return fmt.Sprintf("%d:%s>%s", s.Offset, s.Original, s.Replacement)
}

// Apply returns src with the site's replacement in place.
func (s Site) Apply(src []byte) []byte {
	out := make([]byte, 0, len(src)+len(s.Replacement))
	out = append(out, src[:s.Start]...)
	out = append(out, s.Replacement...)
	return append(out, src[s.End:]...)
}

// Sites returns every mutation in the file's units, unit by unit, each
// unit's in source order; LineOrder puts them in the file's line order.
// Strings and comments hold no sites because only operator, boolean, and
// number tokens are ever changed.
func Sites(f *lang.File) []Site {
	m := f.Spec.Mutations
	var sites []Site
	for i, u := range f.Units {
		unitStart, _ := u.Span()
		comments := commentSpans(f, u)
		f.WalkOwn(u, func(n *sitter.Node) bool {
			if n.ChildCount() > 0 {
				return true
			}
			text := n.Utf8Text(f.Src)
			parent := n.Parent()
			var replacement string
			var ok bool
			if m.Other != nil {
				replacement, ok = m.Other(n)
			}
			switch {
			case ok:
			case m.LiteralKinds[n.Kind()]:
				replacement, ok = m.Literals[text]
			case parent == nil || n.IsNamed():
			case m.SwapParents[parent.Kind()]:
				replacement, ok = m.Swaps[text]
			case m.DeleteParents[parent.Kind()] && m.Deletions[text]:
				replacement, ok = "", true
			}
			if ok {
				p := n.StartPosition()
				sites = append(sites, Site{
					Unit:        i,
					Start:       n.StartByte(),
					End:         n.EndByte(),
					Offset:      int(n.StartByte()-unitStart) - removedBefore(comments, n.StartByte()),
					Line:        int(p.Row) + 1,
					Column:      int(p.Column) + 1,
					Original:    text,
					Replacement: replacement,
				})
			}
			return true
		})
	}
	return sites
}

// LineOrder compares sites by line, then column: the order a file's sites
// are listed in. Sites come unit by unit, which is not line order where a
// unit holds an inline one.
func LineOrder(a, b Site) int {
	if a.Line != b.Line {
		return cmp.Compare(a.Line, b.Line)
	}
	return cmp.Compare(a.Column, b.Column)
}

// UnitHash fingerprints a unit's code, without its inline units or its
// comments: the space and line break only a comment needed go with it
// (commentSpans), so a change to a unit's comments alone keeps its hash.
// Changing anything else inside the unit, whitespace included, changes it.
func UnitHash(f *lang.File, u lang.Unit) string {
	h := sha256.New()
	cut := commentSpans(f, u)
	for _, inner := range f.InnerNodes(u) {
		cut = append(cut, span{inner.StartByte(), inner.EndByte()})
	}
	slices.SortFunc(cut, func(a, b span) int { return cmp.Compare(a.start, b.start) })
	at, end := u.Span()
	for _, c := range cut {
		if c.start > at {
			h.Write(f.Src[at:c.start])
		}
		at = max(at, c.end)
	}
	if end > at {
		h.Write(f.Src[at:end])
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// span is a byte range of a file's source, its end excluded.
type span struct{ start, end uint }

// commentSpans are the byte ranges of u's text its comments take, in source
// order, those of its inline units included, each with the space and line
// break only the comment needed: a comment alone on its line takes the
// whole line, the line break before it on the unit's last line; one after
// code on its line, the space before it; one before code on its line, the
// space after it; and one between code, the space before it.
func commentSpans(f *lang.File, u lang.Unit) []span {
	kinds := f.Spec.Syntax.Comments
	start, end := u.Span()
	src := f.Src
	var out []span
	visit := func(n *sitter.Node) bool {
		if !kinds[n.Kind()] {
			return true
		}
		from, to := n.StartByte(), n.EndByte()
		// A line comment's token may hold the \r of its \r\n.
		for to > from && src[to-1] == '\r' {
			to--
		}
		before := from
		for before > start && (src[before-1] == ' ' || src[before-1] == '\t') {
			before--
		}
		after := to
		for after < end && (src[after] == ' ' || src[after] == '\t') {
			after++
		}
		lineBreak := 0
		switch {
		case after < end && src[after] == '\n':
			lineBreak = 1
		case after+1 < end && src[after] == '\r' && src[after+1] == '\n':
			lineBreak = 2
		}
		startsLine := before == start || src[before-1] == '\n'
		endsLine := after == end || lineBreak > 0
		switch {
		case startsLine && endsLine:
			from = before
			to = after + uint(lineBreak)
			// The unit's last line: the line break before it goes instead.
			if lineBreak == 0 && from > start {
				from--
				if from > start && src[from-1] == '\r' {
					from--
				}
			}
		case endsLine:
			from, to = before, after
		case startsLine:
			to = after
		default:
			from = before
		}
		if len(out) > 0 && from < out[len(out)-1].end {
			from = out[len(out)-1].end
		}
		if from < to {
			out = append(out, span{from, to})
		}
		return false
	}
	for _, n := range u.Nodes() {
		lang.Walk(n, visit)
	}
	return out
}

// removedBefore is how many bytes spans take before at.
func removedBefore(spans []span, at uint) int {
	n := 0
	for _, s := range spans {
		if s.end <= at {
			n += int(s.end - s.start)
		}
	}
	return n
}
