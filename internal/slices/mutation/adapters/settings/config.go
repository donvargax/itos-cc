// Package settings reads itos-cc.yaml, the project's settings, kept at the
// project root under version control and reviewed like the code. It holds
// exclude, globs of the sources no command measures or mutates,
// mutation.exceptions, the equivalent mutants a person excepted, each with
// a reason, mutation.tests, how to list the project's tests and run a
// selection of them, and mutation.copy, globs of what a worker's copy
// leaves out that it brings back. It is the mutation slice's settings adapter, its
// Reader implementing mutation's SettingsReader and selection's, which cmd
// gives every slice (ADR-0061), and the one reading codec: the slice's
// exceptions adapter writes mutation.exceptions through its Parse.
package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// File is the settings file's name, at the project root.
const File = "itos-cc.yaml"

// Path is where the settings file is: at the project root (project.Root),
// wherever a command runs.
func Path() string {
	return filepath.Join(project.Root(), File)
}

// Config is the mutation domain's genuine settings value, holding the
// selection engine's settings. This package owns its representation codec,
// not a second semantic model.
type Config = mutants.Config

// InvalidError is an itos-cc.yaml that cannot be read: not YAML, a key of
// the wrong type, an anchor, an alias or a merge key, a tag JSON does not
// have, a key that is not a string, a second document, an exclude or
// mutation.copy glob that is no pattern, or an exception that lacks a key. Key is the path from the
// top of the key at fault, such as exclude[1], where one key is; empty
// otherwise. Fix is how to mend a refusal that has its own mending; empty
// where saying what each key holds mends it.
type InvalidError struct {
	Reason string
	Key    string
	Fix    string
}

func (e *InvalidError) Error() string { return File + ": " + e.Reason }

// raw is the file as decoded, with pointers to tell a missing key from an
// empty value. Keys itos-cc does not read are left alone.
type raw struct {
	Exclude  []string `yaml:"exclude"`
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
			LineText       *string `yaml:"line_text"`
		} `yaml:"exceptions"`
		Tests *rawTests `yaml:"tests"`
		Copy  []string  `yaml:"copy"`
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
	Whole   *string  `yaml:"whole"`
	Support []string `yaml:"support"`
}

// Load reads itos-cc.yaml at the project root. A missing file is an
// empty config; one that cannot be read as a config is an *InvalidError.
func Load() (*Config, error) {
	return LoadFrom(project.Root())
}

// LoadFrom is Load for the project whose root is root, which need not be
// the working directory's: its selection settings' root.
func LoadFrom(root string) (*Config, error) {
	data, err := Read(root)
	if err != nil {
		return nil, err
	}
	c, err := Parse(data)
	if err != nil {
		return nil, err
	}
	c.Selection.Root = root
	return c, nil
}

// Read is the text of itos-cc.yaml at the project root root, nil when there
// is no file there.
func Read(root string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(root, File)) //nolint:gosec // itos-cc.yaml at the project root the caller found
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Parse reads data, the text of an itos-cc.yaml, as a config: no text at
// all is an empty one, and text that cannot be read as a config is an
// *InvalidError, as is text holding what JSON cannot express
// (docs/CONFIG.md rule 1), before anything else is read: an anchor, an
// alias or a merge key, a tag written in the file other than ! and JSON's,
// a key that is not a string, or a second document. The text names no
// root, so its selection settings have none: the caller that found the
// file gives it.
func Parse(data []byte) (*Config, error) {
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	c := &Config{}
	if err := decoder.Decode(&doc); errors.Is(err, io.EOF) {
		return c, nil
	} else if err != nil {
		return nil, invalidYAML(err)
	}
	if err := spelled(doc.Content[0], ""); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, &InvalidError{Reason: "a second document follows ---, and " + File + " holds one document alone", Fix: documentFix}
	}
	var r raw
	if err := doc.Decode(&r); err != nil {
		return nil, invalidYAML(err)
	}
	for i, glob := range r.Exclude {
		if err := checkGlob(fmt.Sprintf("exclude[%d]", i), glob); err != nil {
			return nil, err
		}
		c.Selection.Exclude = append(c.Selection.Exclude, glob)
	}
	if r.Mutation == nil {
		return c, nil
	}
	for i, glob := range r.Mutation.Copy {
		if err := checkGlob(fmt.Sprintf("mutation.copy[%d]", i), glob); err != nil {
			return nil, err
		}
		c.Copy = append(c.Copy, glob)
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
			return nil, &InvalidError{Reason: fmt.Sprintf("%s has no %s", at, strings.Join(missing, ", no "))}
		}
		if x.LineText != nil {
			e.LineText = *x.LineText
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

// invalidYAML is err, yaml.v3's, as an *InvalidError in its own words.
func invalidYAML(err error) error {
	return &InvalidError{Reason: strings.TrimPrefix(err.Error(), "yaml: ")}
}

// spellingFix is how to mend a file spelled with an anchor, an alias or a
// merge key.
const spellingFix = "Write the value in place: give each key its own copy of the value, with no &anchor, *alias or << merge key."

// tagFix is how to mend a file holding a tag JSON does not have.
const tagFix = "Write the value without the tag: " + File + " takes no tag but ! and JSON's own, !!str, !!int, !!float, !!bool, !!null, !!seq and !!map."

// keyFix is how to mend a mapping holding a key that is not a string.
const keyFix = "Write each key as a string: a list or a mapping is a value, never a key."

// documentFix is how to mend a file holding a second document.
const documentFix = "Keep one document: remove the --- after the first document and what follows it, moving any key you need into the first."

// jsonTags are the tags a file may write: JSON's types, each building
// what the value would be read as without it.
var jsonTags = []string{"!!str", "!!int", "!!float", "!!bool", "!!null", "!!seq", "!!map"}

// spelled refuses the first of what JSON cannot express in n and the nodes
// below it, in document order: an anchor or a tag other than JSON's,
// naming the key whose value carries it, a merge key, naming itself, and a
// key that is not a string, naming the mapping holding it; at is n's key,
// "" for the top level. An alias always follows its anchor, so a file
// holding one is refused at its anchor's key, and an alias can expand
// without bound (CVE-2019-11253); a tag can build an arbitrary object
// (CVE-2022-1471). Only a tag written in the file counts: the
// non-specific ! writes none, and a plain 2026-10-24, which YAML resolves
// as a timestamp, is read as its key's string.
func spelled(n *yaml.Node, at string) error {
	what := at
	if what == "" {
		what = "the top level"
	}
	if n.Anchor != "" {
		return spellingError(at, fmt.Sprintf("%s carries the anchor &%s", what, n.Anchor))
	}
	if n.Style&yaml.TaggedStyle != 0 && !slices.Contains(jsonTags, n.Tag) {
		return &InvalidError{Reason: fmt.Sprintf("%s carries the tag %s, and %s takes no tags but ! and JSON's", what, n.Tag, File), Key: at, Fix: tagFix}
	}
	switch n.Kind {
	case yaml.SequenceNode:
		for i, item := range n.Content {
			if err := spelled(item, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if at != "" {
				key = at + "." + key
			}
			if n.Content[i].Tag == "!!merge" {
				return spellingError(key, key+" is a merge key")
			}
			if n.Content[i].Kind != yaml.ScalarNode {
				return &InvalidError{Reason: fmt.Sprintf("%s holds a key that is not a string, and %s takes string keys alone", what, File), Key: at, Fix: keyFix}
			}
			if err := spelled(n.Content[i], key); err != nil {
				return err
			}
			if err := spelled(n.Content[i+1], key); err != nil {
				return err
			}
		}
	}
	return nil
}

// spellingError is the *InvalidError of key, found as found says.
func spellingError(key, found string) error {
	return &InvalidError{Reason: found + ", and " + File + " takes no anchors, aliases or merge keys", Key: key, Fix: spellingFix}
}

// checkGlob refuses glob, the value of key, such as exclude[1] or
// mutation.copy[0], unless it is a doublestar pattern from the project
// root, slash-separated: not empty, not absolute, never leaving the root
// through .., and holding no backslash, which doublestar reads as an
// escape where a path on Windows means a separator. A glob that matches
// nothing is fine.
func checkGlob(key, glob string) error {
	var why string
	switch {
	case strings.TrimSpace(glob) == "":
		why = "is empty"
	case strings.Contains(glob, `\`):
		why = "holds a backslash; separate its folders with /"
	case strings.HasPrefix(glob, "/") || filepath.VolumeName(glob) != "" || hasDrive(glob):
		why = "is absolute; write it from the project root"
	case slices.Contains(strings.Split(glob, "/"), ".."):
		why = "leaves the project root through .."
	case !doublestar.ValidatePattern(glob):
		why = "is no pattern"
	default:
		return nil
	}
	return &InvalidError{Reason: fmt.Sprintf("%s %q %s", key, glob, why), Key: key}
}

// hasDrive is whether glob begins with a Windows drive, such as C:, on
// every system, so a file is read the same wherever it is.
func hasDrive(glob string) bool {
	return len(glob) > 1 && glob[1] == ':' && ('a' <= glob[0] && glob[0] <= 'z' || 'A' <= glob[0] && glob[0] <= 'Z')
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
	for _, glob := range r.Support {
		if _, err := filepath.Match(glob, ""); err != nil || strings.TrimSpace(glob) == "" {
			wrong = append(wrong, fmt.Sprintf("has a support glob %q that is no pattern", glob))
			continue
		}
		t.Support = append(t.Support, glob)
	}
	if len(wrong) > 0 {
		return nil, &InvalidError{Reason: "mutation.tests " + strings.Join(wrong, ", ")}
	}
	return t, nil
}

// Exception and Tests are domain values; YAML stays at this boundary.
type Exception = mutants.Exception
type Tests = mutants.Tests
