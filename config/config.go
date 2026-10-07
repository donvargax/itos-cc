// Package config reads and writes itos-cc.yaml, the project's settings,
// kept at the project root under version control and reviewed like the
// code. It holds mutation.exceptions, the equivalent mutants a person
// excepted, each with a reason, and mutation.tests, how to list the
// project's tests and run a selection of them.
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
	// Tests, when set, is how to list the project's tests and run a
	// selection of them.
	Tests *Tests
}

// Tests is mutation.tests: commands, run through the platform shell at the
// project root, that list the project's tests and run a selection of them,
// sharing only the tests' IDs and paths.
type Tests struct {
	// List prints one test per line: its ID and, after a tab, optionally
	// the file that defines it.
	List string
	// Run runs the tests {pattern} selects.
	Run string
	// IDsPattern is the pattern of a selection, with {ids} the selected
	// IDs, each written as Each, with {id} the ID, joined by Sep.
	IDsPattern string
	Each, Sep  string
	// Whole, optional, runs every listed test; without it, Run selects them
	// all.
	Whole string
}

// Select is the command that runs the tests ids.
func (t *Tests) Select(ids []string) string {
	each := make([]string, len(ids))
	for i, id := range ids {
		each[i] = strings.ReplaceAll(t.Each, "{id}", id)
	}
	pattern := strings.ReplaceAll(t.IDsPattern, "{ids}", strings.Join(each, t.Sep))
	return strings.ReplaceAll(t.Run, "{pattern}", pattern)
}

// All is the command that runs every test, ids.
func (t *Tests) All(ids []string) string {
	if t.Whole != "" {
		return t.Whole
	}
	return t.Select(ids)
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
		Tests *rawTests `yaml:"tests"`
	} `yaml:"mutation"`
}

// rawTests is mutation.tests as decoded.
type rawTests struct {
	List       *string `yaml:"list"`
	Run        *string `yaml:"run"`
	IDsPattern *string `yaml:"ids_pattern"`
	Join       *struct {
		Each *string `yaml:"each"`
		Sep  *string `yaml:"sep"`
	} `yaml:"join"`
	Whole *string `yaml:"whole"`
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
	if r.Mutation.Tests != nil {
		t, err := r.Mutation.Tests.parse()
		if err != nil {
			return nil, err
		}
		c.Tests = t
	}
	return c, nil
}

// parse checks mutation.tests: list, run with {pattern}, ids_pattern with
// {ids}, and join, each with {id} and sep, which may be empty; whole is
// optional.
func (r *rawTests) parse() (*Tests, error) {
	var wrong []string
	text := func(key string, v *string, placeholder string) string {
		switch {
		case v == nil || strings.TrimSpace(*v) == "":
			wrong = append(wrong, "has no "+key)
		case placeholder != "" && !strings.Contains(*v, placeholder):
			article := "a"
			if strings.HasPrefix(key, "i") {
				article = "an"
			}
			wrong = append(wrong, fmt.Sprintf("has %s %s without %s", article, key, placeholder))
		default:
			return *v
		}
		return ""
	}
	t := &Tests{List: text("list", r.List, ""), Run: text("run", r.Run, "{pattern}"),
		IDsPattern: text("ids_pattern", r.IDsPattern, "{ids}")}
	if r.Join == nil {
		wrong = append(wrong, "has no join")
	} else {
		t.Each = text("join.each", r.Join.Each, "{id}")
		if r.Join.Sep == nil {
			wrong = append(wrong, "has no join.sep")
		} else {
			t.Sep = *r.Join.Sep
		}
	}
	if r.Whole != nil {
		t.Whole = text("whole", r.Whole, "")
	}
	if len(wrong) > 0 {
		return nil, &InvalidError{"mutation.tests " + strings.Join(wrong, ", ")}
	}
	return t, nil
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
