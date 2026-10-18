// Package config reads and writes itos-cc.yaml, the project's settings,
// kept at the project root under version control and reviewed like the
// code. It holds exclude, globs of the sources no command measures or
// mutates, mutation.exceptions, the equivalent mutants a person excepted,
// each with a reason, and mutation.tests, how to list the project's tests
// and run a selection of them.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
)

// File is the settings file's name, at the project root.
const File = "itos-cc.yaml"

// Path is where the settings file is: at the project root (project.Root),
// wherever a command runs.
func Path() string {
	return filepath.Join(project.Root(), File)
}

// Config is the mutation domain's genuine settings value. This package owns
// its representation codec, not a second semantic model.
type Config = mutants.Config

// InvalidError is an itos-cc.yaml that cannot be read: not YAML, a key of
// the wrong type, an exclude glob that is no pattern, or an exception that
// lacks a key. Key is the path from the top of the key at fault, such as
// exclude[1], where one key is; empty otherwise.
type InvalidError struct {
	Reason string
	Key    string
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
// the working directory's.
func LoadFrom(root string) (*Config, error) {
	data, err := Read(root)
	if err != nil {
		return nil, err
	}
	return Parse(data)
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
// *InvalidError.
func Parse(data []byte) (*Config, error) {
	var r raw
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, &InvalidError{Reason: strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	c := &Config{}
	for i, glob := range r.Exclude {
		if err := checkExclude(i, glob); err != nil {
			return nil, err
		}
		c.Exclude = append(c.Exclude, glob)
	}
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

// checkExclude refuses exclude[i], glob, unless it is a doublestar pattern
// from the project root, slash-separated: not empty, not absolute, never
// leaving the root through .., and holding no backslash, which doublestar
// reads as an escape where a path on Windows means a separator. A glob
// that matches nothing is fine.
func checkExclude(i int, glob string) error {
	key := fmt.Sprintf("exclude[%d]", i)
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

// Renewal aliases the genuine domain value; there is no Old/New wire twin.
type Renewal = mutants.Renewal

// Except retains the original direct-library entry point.
func Except(e Exception) (bool, error) {
	data, err := os.ReadFile(Path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	updated, replaced, err := ExceptDocument(data, e)
	if err != nil {
		return false, err
	}
	return replaced, writeDocument(Path(), updated)
}

// Renew retains the original direct-library batch entry point.
func Renew(renewals []Renewal) error {
	if len(renewals) == 0 {
		return nil
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		return err
	}
	updated, err := RenewDocument(data, renewals)
	if err != nil {
		return err
	}
	return writeDocument(Path(), updated)
}
func writeDocument(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode) //nolint:gosec // path is itos-cc.yaml at the project root, rewritten keeping its own mode
}

// Exception and Tests are domain values; YAML stays at this boundary.
type Exception = mutants.Exception
type Tests = mutants.Tests
