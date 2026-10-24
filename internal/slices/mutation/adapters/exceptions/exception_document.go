package exceptions

import (
	"bytes"
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// ExceptDocument adds e to the current document's mutation.exceptions, or
// replaces the entry for its site, writing only that entry's lines
// (writeList). It returns no bytes on failure.
func ExceptDocument(data []byte, e mutants.Exception) ([]byte, bool, error) {
	c, doc, err := read(data)
	if err != nil {
		return nil, false, err
	}
	edit := listEdit{n: len(c.Exceptions)}
	i := slices.IndexFunc(c.Exceptions, e.SameSite)
	if i >= 0 {
		edit.replace = map[int]mutants.Exception{i: e}
	} else {
		edit.add = []mutants.Exception{e}
	}
	data, err = writeList(data, doc, c.Exceptions, edit)
	return data, i >= 0, err
}

// RenewDocument replaces each renewal's complete Old entry, each entry once,
// and writes only a full batch, changing only those entries' lines.
func RenewDocument(data []byte, renewals []mutants.Renewal) ([]byte, error) {
	c, doc, err := read(data)
	if err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, &settings.InvalidError{Reason: "its top level is not a mapping"}
	}
	edit := listEdit{n: len(c.Exceptions), replace: map[int]mutants.Exception{}}
	done := make([]bool, len(c.Exceptions))
	for _, r := range renewals {
		i := entryOf(c.Exceptions, done, r.Old)
		if i < 0 {
			return nil, fmt.Errorf("%s: no entry for %s in %s to renew", settings.File, r.Old.Function, r.Old.File)
		}
		edit.replace[i] = r.New
	}
	return writeList(data, doc, c.Exceptions, edit)
}

// PruneDocument removes each of entries, a complete entry of the current
// document, each entry once, with the comment lines directly above it,
// and writes only a full batch, changing only those lines.
func PruneDocument(data []byte, entries []mutants.Exception) ([]byte, error) {
	c, doc, err := read(data)
	if err != nil {
		return nil, err
	}
	edit := listEdit{n: len(c.Exceptions), remove: map[int]bool{}}
	done := make([]bool, len(c.Exceptions))
	for _, e := range entries {
		i := entryOf(c.Exceptions, done, e)
		if i < 0 {
			return nil, fmt.Errorf("%s: no entry for %s in %s to remove", settings.File, e.Function, e.File)
		}
		edit.remove[i] = true
	}
	return writeList(data, doc, c.Exceptions, edit)
}

// entryOf is the index of the first entry of entries that is e and not
// done, which it marks done; -1 when there is none.
func entryOf(entries []mutants.Exception, done []bool, e mutants.Exception) int {
	for i, old := range entries {
		if !done[i] && old == e {
			done[i] = true
			return i
		}
	}
	return -1
}

// read is data as settings.Parse reads it, and its node tree, whose top
// level, when it has one, is a mapping holding mutation.exceptions as
// settings.Parse read it.
func read(data []byte) (*settings.Config, *yaml.Node, error) {
	c, err := settings.Parse(data)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, &settings.InvalidError{Reason: err.Error()}
	}
	if len(doc.Content) > 0 && !isNull(doc.Content[0]) {
		root := doc.Content[0]
		if root.Kind != yaml.MappingNode {
			return nil, nil, &settings.InvalidError{Reason: "its top level is not a mapping"}
		}
		if _, m := key(root, "mutation"); m != nil && m.Kind == yaml.MappingNode {
			if _, list := key(m, "exceptions"); list != nil && list.Kind == yaml.SequenceNode && len(list.Content) != len(c.Exceptions) {
				return nil, nil, fmt.Errorf("%s: mutation.exceptions changed while it was read", settings.File)
			}
		}
	}
	return c, &doc, nil
}

// valueOf is the value of key in mapping m, made a node of kind when it is
// missing or null.
func valueOf(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	tag := map[yaml.Kind]string{yaml.MappingNode: "!!map", yaml.SequenceNode: "!!seq"}[kind]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
				v.Kind, v.Tag, v.Value, v.Style = kind, tag, "", 0
			}
			return v
		}
	}
	v := &yaml.Node{Kind: kind, Tag: tag}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	return v
}

func encodeDocument(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
