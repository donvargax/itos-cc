package exceptions

import (
	"bytes"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// spliceHashes writes edit, which gives entries of mutation.exceptions, a
// block or flow list read holds, a new hash and changes nothing else of
// them, by changing only the text of each one's hash value, where yaml.v3
// places it, in the style it has; a plain one that would no longer read as
// a string takes the quote the document uses (quoteOf). False when a
// hash's text is not a plain or quoted scalar it can find there.
func spliceHashes(data []byte, doc *yaml.Node, edit listEdit) ([]byte, bool) {
	t := newText(data)
	t.quote = quoteOf(doc)
	// Parse read every entry edit names, and each has its hash.
	_, mutation := key(doc.Content[0], "mutation")
	_, list := key(mutation, "exceptions")
	var edits []change
	for i, e := range edit.replace {
		_, v := key(list.Content[i], "hash")
		was, ok := scalarText(v)
		// yaml.v3's place is in the text; the text there must be the hash's.
		at, _ := t.offset(v.Line, v.Column)
		if !ok || !bytes.HasPrefix(t.data[at:], []byte(was)) {
			return nil, false
		}
		with, ok := t.hashText(v.Style, e.Hash)
		if !ok {
			return nil, false
		}
		edits = append(edits, change{at, at + len(was), with})
	}
	return apply(data, edits), true
}

// scalarText is the text of v, a scalar written plain or quoted, as the
// document holds it when it is on one line with no escape; false for any
// other node, whose text spliceHashes then cannot find.
func scalarText(v *yaml.Node) (string, bool) {
	switch v.Style {
	case 0:
		return v.Value, true
	case yaml.SingleQuotedStyle:
		return "'" + strings.ReplaceAll(v.Value, "'", "''") + "'", true
	case yaml.DoubleQuotedStyle:
		return `"` + v.Value + `"`, true
	}
	return "", false
}

// hexDigits is what a function's hash holds.
var hexDigits = regexp.MustCompile(`^[0-9a-f]+$`)

// hashText is hash, a function's hash, which holds only hex digits, written
// in style: quoted as it was, or plain when it reads as a string plain,
// else in the quote the document uses; false for a hash with anything but
// hex digits, which no quote is then sure to hold.
func (t *text) hashText(style yaml.Style, hash string) (string, bool) {
	if !hexDigits.MatchString(hash) {
		return "", false
	}
	if style == 0 {
		var plain yaml.Node
		// Hex digits are a document yaml.v3 reads, one scalar.
		_ = yaml.Unmarshal([]byte(hash), &plain)
		if plain.Content[0].ShortTag() == "!!str" {
			return hash, true
		}
		style = t.quote
	}
	if style == yaml.DoubleQuotedStyle {
		return `"` + hash + `"`, true
	}
	return "'" + hash + "'", true
}
