package settings

import (
	"fmt"
	"strings"

	"github.com/donvargax/kong"
	"go.yaml.in/yaml/v3"
)

// shape is what a value of itos-cc.yaml holds, as far as its keys go: a
// mapping lists the keys it takes, in the order the docs name them; a list
// of mappings gives the shape of each entry; any other value takes no key.
// It describes nothing of a value's type, which decoding checks.
type shape struct {
	keys  []listed
	entry *shape
}

// listed is a key the format lists, and the shape of its value.
type listed struct {
	name  string
	value shape
}

// plain is listed keys, each holding a value with no keys of its own.
func plain(names ...string) []listed {
	keys := make([]listed, len(names))
	for i, name := range names {
		keys[i] = listed{name: name}
	}
	return keys
}

// format is every key itos-cc.yaml takes, from the top: the one table the
// reader walks a file against (docs/CONFIG.md rule 2), held as data so a
// JSON Schema can be generated from it.
var format = shape{keys: []listed{
	{name: "version"},
	{name: "exclude"},
	{name: "mutation", value: shape{keys: []listed{
		{name: "exceptions", value: shape{entry: &shape{keys: plain(
			"file", "function", "hash", "line_in_function", "column", "original", "replacement", "reason", "line_text")}}},
		{name: "tests", value: shape{keys: append(plain("list", "run", "ids_pattern"),
			listed{name: "join", value: shape{keys: plain("each", "sep")}},
			listed{name: "whole"}, listed{name: "support"})}},
		{name: "copy"},
	}}},
}}

// Extension is the prefix of an extension key, as Docker Compose and
// OpenAPI allow: a mapping at any level may hold keys starting with it,
// which the walk and the decoder leave unread, so other tools and people
// can keep their own data in itos-cc.yaml (docs/CONFIG.md rule 2,
// slice-54), and which the writers keep in an entry they rewrite
// (slice-55). Their values are still held to rule 1 (spelled). The prefix
// is exact: X- and x_ are keys like any other.
const Extension = "x-"

// unlisted refuses the first key in n, in document order, that s, the
// shape of n's place, does not list there, an extension key aside; at is
// n's key, "" for the top level. A value whose shape differs from s's is
// left to decoding.
func unlisted(n *yaml.Node, s shape, at string) error {
	switch {
	case n.Kind == yaml.MappingNode && s.keys != nil:
		for i := 0; i < len(n.Content); i += 2 {
			name := n.Content[i].Value
			if strings.HasPrefix(name, Extension) {
				continue
			}
			key := name
			if at != "" {
				key = at + "." + name
			}
			value, ok := s.value(name)
			if !ok {
				return unlistedError(at, key, name, s)
			}
			if err := unlisted(n.Content[i+1], value, key); err != nil {
				return err
			}
		}
	case n.Kind == yaml.SequenceNode && s.entry != nil:
		for i, item := range n.Content {
			if err := unlisted(item, *s.entry, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// value is the shape of the key name s lists, and whether it lists it.
func (s shape) value(name string) (shape, bool) {
	for _, k := range s.keys {
		if k.name == name {
			return k.value, true
		}
	}
	return shape{}, false
}

// unlistedError is the *InvalidError of key, holding name, which s, the
// shape of the mapping at at, does not list. Its fix names the listed keys
// the kong fork's Suggest finds for name, by the rule the command line's
// "Did you mean" follows (decision 62), one or several in s's order, or
// else every key s lists.
func unlistedError(at, key, name string, s shape) error {
	what := at
	if what == "" {
		what = "the top level"
	}
	names := make([]string, len(s.keys))
	for i, k := range s.keys {
		names[i] = k.name
	}
	var fix string
	switch meant := kong.Suggest(name, names); len(meant) {
	case 0:
		fix = fmt.Sprintf("Remove the key, or rename it to one %s takes: %s.", what, strings.Join(names, ", "))
	case 1:
		fix = fmt.Sprintf("Did you mean %s? Rename the key, or remove it.", meant[0])
	default:
		fix = fmt.Sprintf("Did you mean one of %s? Rename the key, or remove it.", strings.Join(meant, ", "))
	}
	return &InvalidError{Reason: fmt.Sprintf("%s holds the key %s, which %s does not list", what, name, File), Key: key, Fix: fix}
}
