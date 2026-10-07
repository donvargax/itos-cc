// Package config reads and writes itos-cc.yaml, the project's settings,
// kept at the project root under version control and reviewed like the
// code. Its one setting so far is mutation.exceptions: the equivalent
// mutants a person excepted, each with a reason.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/project"
)

// File is the settings file's name, at the project root.
const File = "itos-cc.yaml"

// Path is where the settings file is: at the project root (project.Root),
// wherever a command runs.
func Path() string {
	return filepath.Join(project.Root(), File)
}

// Exception is one excepted mutant: a survivor no test can kill because it
// changes no behaviour. It is found by its function, the function's hash
// when it was excepted, and the site's place within the function, so a
// function that only moved keeps it.
type Exception struct {
	File     string `yaml:"file"`     // slash-separated, from the project root
	Function string `yaml:"function"` // namespace#name
	Hash     string `yaml:"hash"`     // the function's hash, as the snapshot records it
	// LineInFunction is the site's line counted from the function's first
	// line, which is 1.
	LineInFunction int    `yaml:"line_in_function"`
	Column         int    `yaml:"column"`
	Original       string `yaml:"original"`
	Replacement    string `yaml:"replacement"` // empty for a deletion
	Reason         string `yaml:"reason"`
}

// SameSite reports whether e and o except the same site: one function, one
// place within it, one change.
func (e Exception) SameSite(o Exception) bool {
	return e.File == o.File && e.Function == o.Function && e.LineInFunction == o.LineInFunction &&
		e.Column == o.Column && e.Original == o.Original && e.Replacement == o.Replacement
}

// Config is what itos-cc reads of itos-cc.yaml.
type Config struct {
	Exceptions []Exception
}

// InvalidError is an itos-cc.yaml that cannot be read: not YAML, a key of
// the wrong type, or an exception that lacks a key.
type InvalidError struct{ Reason string }

func (e *InvalidError) Error() string { return File + ": " + e.Reason }

// raw is the file as decoded, with pointers to tell a missing key from an
// empty value. Keys itos-cc does not read are left alone.
type raw struct {
	Mutation *struct {
		Exceptions []struct {
			File           *string `yaml:"file"`
			Function       *string `yaml:"function"`
			Hash           *string `yaml:"hash"`
			LineInFunction *int    `yaml:"line_in_function"`
			Column         *int    `yaml:"column"`
			Original       *string `yaml:"original"`
			Replacement    *string `yaml:"replacement"`
			Reason         *string `yaml:"reason"`
		} `yaml:"exceptions"`
	} `yaml:"mutation"`
}

// Load reads itos-cc.yaml at the project root. A missing file is an
// empty config; one that cannot be read as a config is an *InvalidError.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*Config, error) {
	var r raw
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, &InvalidError{strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	c := &Config{}
	if r.Mutation == nil {
		return c, nil
	}
	for i, x := range r.Mutation.Exceptions {
		at := fmt.Sprintf("mutation.exceptions[%d]", i)
		var missing []string
		text := func(key string, v *string, emptyOK bool) string {
			if v == nil || (!emptyOK && strings.TrimSpace(*v) == "") {
				missing = append(missing, key)
				return ""
			}
			return *v
		}
		number := func(key string, v *int) int {
			if v == nil || *v < 1 {
				missing = append(missing, key+" of 1 or more")
				return 0
			}
			return *v
		}
		e := Exception{
			File: text("file", x.File, false), Function: text("function", x.Function, false), Hash: text("hash", x.Hash, false),
			LineInFunction: number("line_in_function", x.LineInFunction), Column: number("column", x.Column),
			Original: text("original", x.Original, false), Replacement: text("replacement", x.Replacement, true),
			Reason: text("reason", x.Reason, false),
		}
		if len(missing) > 0 {
			return nil, &InvalidError{fmt.Sprintf("%s has no %s", at, strings.Join(missing, ", no "))}
		}
		c.Exceptions = append(c.Exceptions, e)
	}
	return c, nil
}

// Except writes e into itos-cc.yaml at the project root, under
// mutation.exceptions, in place of an entry for the same site or after the
// others, and reports whether it replaced one. The file's other keys and
// its comments are kept; the file is made when there is none. It refuses,
// with an *InvalidError, to write over a file it cannot read.
func Except(e Exception) (replaced bool, err error) {
	data, err := os.ReadFile(Path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	c, err := parse(data)
	if err != nil {
		return false, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false, &InvalidError{err.Error()}
	}
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null" {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false, &InvalidError{"its top level is not a mapping"}
	}
	exceptions := valueOf(valueOf(root, "mutation", yaml.MappingNode), "exceptions", yaml.SequenceNode)

	var entry yaml.Node
	if err := entry.Encode(e); err != nil {
		return false, err
	}
	for i, old := range c.Exceptions {
		if old.SameSite(e) {
			was := exceptions.Content[i]
			entry.HeadComment, entry.LineComment, entry.FootComment = was.HeadComment, was.LineComment, was.FootComment
			exceptions.Content[i] = &entry
			return true, write(&doc)
		}
	}
	exceptions.Content = append(exceptions.Content, &entry)
	return false, write(&doc)
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

func write(doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(Path()); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(Path(), buf.Bytes(), mode)
}
