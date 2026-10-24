// Command listed-scenarios prints the scenarios itos-cc.yaml's
// mutation.tests.list names: of the live scenarios itos tests list scenario
// --json lists, read on stdin, those the in-process TestScenarios runs, a
// line each, its ID, a tab, and its feature file from the project's top:
//
//	itos tests list scenario --json | go run ./tools/bin/listed-scenarios
//
// A counted mutation run fails closed on a listed test that produces no
// per-ID coverage data (T-187's first attempt, ci.yml run 40042092414, on
// ID-GRAPH-01), so the list names only what the run command executes. The
// in-process driver runs a scenario when its file is one features.Converted
// lists and at least one of its pickles, each row of an outline being one,
// carries none of the tags TagFilter and Defer leave out of an unselected
// run: @wip, features.BinaryOnly, an ID features.Unstepped lists, or a tag
// features.Deferred lists, each its own or its feature's, rule's or
// examples'. Those come from package features, and the tags from the
// feature files through the gherkin parser the harness reads them with, so
// the list never holds a second copy of the harness's rules. Once itos
// prints each scenario's tags (itos#117), a filter of the list replaces this
// program.
//
// No fingerprint column, as the jq filter it replaces printed none: the
// list's format takes one, but its kills are recorded without, dated by
// their feature file's hash.
//
// Exit status: 0 listed, 1 the listing or a feature file could not be read,
// 2 a usage error.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	gherkin "github.com/cucumber/gherkin/go/v42"
	messages "github.com/cucumber/messages/go/v34"
	"github.com/donvargax/itos-cc/features"
)

const self = "listed-scenarios"

// exit ends the process; a test replaces it to run main.
var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, inProcess()))
}

// run lists the scenarios of the listing in that h runs, and returns its
// exit code. A line the terminal refuses has nowhere else to be reported, so its
// write's error is discarded.
func run(args []string, in io.Reader, out, stderr io.Writer, h harness) int {
	flags := flag.NewFlagSet(self, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "features", "the folder of the feature files, from the project's top")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: itos tests list scenario --json | "+self+" [-dir <features>]")
		return 2
	}
	tests, err := listed(in, *dir, h)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", self, err)
		return 1
	}
	for _, t := range tests {
		_, _ = fmt.Fprintf(out, "%s\t%s\n", t.id, t.file)
	}
	return 0
}

// harness is what a driver leaves out of a run that selects nothing: the
// feature files it runs, and the tags whose pickles it never runs; read
// reads a feature file, os.ReadFile when nil.
type harness struct {
	converted []string
	skipped   []string
	read      func(string) ([]byte, error)
}

// inProcess is the in-process TestScenarios', as package features declares
// it.
func inProcess() harness {
	skipped := append([]string{"@wip", features.BinaryOnly}, features.Unstepped...)
	return harness{
		converted: features.Converted,
		skipped:   append(skipped, slices.Sorted(maps.Keys(features.Deferred))...),
	}
}

// test is a line of the list: a scenario's ID and its feature file.
type test struct{ id, file string }

// listing is what itos tests list scenario --json prints, the fields read
// here.
type listing struct {
	Tests []struct {
		ID   string `json:"id"`
		File string `json:"file"`
		Live bool   `json:"live"`
	} `json:"tests"`
}

// listed are the live scenarios of the listing in that h runs, in its
// order, each file named under dir.
func listed(in io.Reader, dir string, h harness) ([]test, error) {
	var l listing
	if err := json.NewDecoder(in).Decode(&l); err != nil {
		return nil, fmt.Errorf("reading itos tests list scenario --json: %w", err)
	}
	if l.Tests == nil {
		return nil, fmt.Errorf("reading itos tests list scenario --json: no tests array")
	}
	runs := map[string]map[string]bool{}
	var out []test
	for _, t := range l.Tests {
		if !t.Live || !slices.Contains(h.converted, t.File) {
			continue
		}
		path := filepath.Join(dir, t.File)
		tags, ok := runs[t.File]
		if !ok {
			var err error
			if tags, err = h.runs(path); err != nil {
				return nil, err
			}
			runs[t.File] = tags
		}
		if tags["@"+t.ID] {
			out = append(out, test{t.ID, filepath.ToSlash(path)})
		}
	}
	return out, nil
}

// runs are the tags of the pickles of the feature file at path that h runs.
func (h harness) runs(path string) (map[string]bool, error) {
	read := h.read
	if read == nil {
		read = os.ReadFile
	}
	data, err := read(path)
	if err != nil {
		return nil, err
	}
	newID := (&messages.Incrementing{}).NewId
	doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), newID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	tags := map[string]bool{}
	for _, pickle := range gherkin.Pickles(*doc, path, newID) {
		var names []string
		for _, tag := range pickle.Tags {
			names = append(names, tag.Name)
		}
		if slices.ContainsFunc(names, h.skips) {
			continue
		}
		for _, name := range names {
			tags[name] = true
		}
	}
	return tags, nil
}

// skips is whether h never runs a pickle carrying tag.
func (h harness) skips(tag string) bool {
	return slices.Contains(h.skipped, tag)
}
