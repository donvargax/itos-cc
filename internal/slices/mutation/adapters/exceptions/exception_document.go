package exceptions

import (
	"bytes"
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/config"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// ExceptDocument updates the CURRENT document through the single YAML codec.
// It preserves unrelated keys/comments and returns no bytes on failure.
func ExceptDocument(data []byte, e mutants.Exception) ([]byte, bool, error) {
	c, err := config.Parse(data)
	if err != nil {
		return nil, false, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, &config.InvalidError{Reason: err.Error()}
	}
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null" {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, &config.InvalidError{Reason: "its top level is not a mapping"}
	}
	exceptions := valueOf(valueOf(root, "mutation", yaml.MappingNode), "exceptions", yaml.SequenceNode)

	var entry yaml.Node
	if err := entry.Encode(e); err != nil {
		return nil, false, err
	}
	for i, old := range c.Exceptions {
		if old.SameSite(e) {
			was := exceptions.Content[i]
			entry.HeadComment, entry.LineComment, entry.FootComment = was.HeadComment, was.LineComment, was.FootComment
			exceptions.Content[i] = &entry
			data, err := encodeDocument(&doc)
			return data, true, err
		}
	}
	exceptions.Content = append(exceptions.Content, &entry)
	data, err = encodeDocument(&doc)
	return data, false, err
}

// RenewDocument consumes complete Old entries once and encodes only a full batch.
func RenewDocument(data []byte, renewals []mutants.Renewal) ([]byte, error) {
	c, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &config.InvalidError{Reason: err.Error()}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, &config.InvalidError{Reason: "its top level is not a mapping"}
	}
	exceptions := valueOf(valueOf(doc.Content[0], "mutation", yaml.MappingNode), "exceptions", yaml.SequenceNode)
	if len(exceptions.Content) != len(c.Exceptions) {
		return nil, fmt.Errorf("%s: mutation.exceptions changed while it was read", config.File)
	}
	done := make([]bool, len(c.Exceptions))
	for _, r := range renewals {
		i := slices.IndexFunc(c.Exceptions, func(e mutants.Exception) bool { return e == r.Old })
		for i >= 0 && done[i] {
			next := slices.IndexFunc(c.Exceptions[i+1:], func(e mutants.Exception) bool { return e == r.Old })
			if next < 0 {
				i = -1
				break
			}
			i += 1 + next
		}
		if i < 0 {
			return nil, fmt.Errorf("%s: no entry for %s in %s to renew", config.File, r.Old.Function, r.Old.File)
		}
		var entry yaml.Node
		if err := entry.Encode(r.New); err != nil {
			return nil, err
		}
		was := exceptions.Content[i]
		entry.HeadComment, entry.LineComment, entry.FootComment = was.HeadComment, was.LineComment, was.FootComment
		exceptions.Content[i] = &entry
		done[i] = true
	}
	return encodeDocument(&doc)
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
