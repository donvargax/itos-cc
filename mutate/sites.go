// Package mutate runs mutation testing: it makes small changes to functions
// one at a time and checks that the tests notice. A mutant is killed when the
// tests fail or time out, and survives when they still pass. A mutant on a
// line no test executes is uncovered and is not run.
//
// Results are cached per function in .metrics/mutate/, keyed by a hash of the
// function's source, so a later run only retries survivors and the sites of
// functions whose text changed.
package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/donvargax/itos-cc/lang"
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

// Sites returns every mutation in the file's units, in source order.
// Strings and comments hold no sites because only operator, boolean, and
// number tokens are ever changed.
func Sites(f *lang.File) []Site {
	m := f.Spec.Mutations
	var sites []Site
	for i, u := range f.Units {
		unitStart := u.Node.StartByte()
		lang.Walk(u.Node, func(n *sitter.Node) bool {
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
					Offset:      int(n.StartByte() - unitStart),
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

// UnitHash fingerprints a unit's source text. Changing anything inside the
// unit, including whitespace and comments, changes it.
func UnitHash(f *lang.File, u lang.Unit) string {
	h := sha256.Sum256(f.Src[u.Node.StartByte():u.Node.EndByte()])
	return hex.EncodeToString(h[:8])
}
