package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Running mutants: what a run announces of its baselines and trials, and the
// test runs it records.

// announced is a command mutation run's stderr says it runs unmutated: the
// folder it runs in, as itos-cc names it, slash-separated, and its words.
type announced struct {
	dir, command string
	words        []string
}

var baselineLine = regexp.MustCompile(`^itos-cc: baseline (.*?)\$ (.+)$`)

// baselines are the baselines stderr, a mutation run's, announces.
func baselines(stderr string) []announced {
	var out []announced
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		if match := baselineLine.FindStringSubmatch(line); match != nil {
			out = append(out, announced{dir: filepath.ToSlash(match[1]), command: match[2], words: strings.Fields(match[2])})
		}
	}
	return out
}

// trial is a mutant a mutation run's stderr says it ran: its file,
// slash-separated, line, texts, what the run said of it and the seconds it
// took.
type trial struct {
	file                           string
	line                           int
	original, replacement, outcome string
	elapsed                        float64
}

var trialOutcome = regexp.MustCompile("^itos-cc: \\[\\d+/\\d+\\] (\\S+):(\\d+) `([^`]*)` → (?:`([^`]*)`|\\(deleted\\)) (.+?) \\(([\\d.]+)s(?:, [^)]*)?\\)$")

// trialsOf are the mutants of file stderr says the run ran.
func trialsOf(stderr, file string) []trial {
	var out []trial
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		match := trialOutcome.FindStringSubmatch(line)
		if match == nil || filepath.ToSlash(match[1]) != file {
			continue
		}
		n, _ := strconv.Atoi(match[2])
		took, _ := strconv.ParseFloat(match[6], 64)
		out = append(out, trial{file: file, line: n, original: match[3], replacement: match[4], outcome: match[5], elapsed: took})
	}
	return out
}

// uncoveredListed are the mutants of file stdout lists as uncovered, as
// --fail-uncovered lists them: "uncovered `<original>` → … in <function>,
// <file> line <line> column <column>".
func uncoveredListed(stdout, file string) []listedSite {
	var out []listedSite
	for _, line := range strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "uncovered ")
		if !ok {
			continue
		}
		match := plainSite.FindStringSubmatch(rest)
		if match == nil || filepath.ToSlash(match[4]) != file {
			continue
		}
		n, _ := strconv.Atoi(match[5])
		column, _ := strconv.Atoi(match[6])
		out = append(out, listedSite{file: file, line: n, column: column, original: match[1], replacement: match[2], function: match[3]})
	}
	return out
}

// sameSites is whether got and want hold the same sites, in any order.
func sameSites(got, want []listedSite) bool {
	key := func(s listedSite) string { return s.String() }
	a, b := make([]string, 0, len(got)), make([]string, 0, len(want))
	for _, s := range got {
		a = append(a, key(s))
	}
	for _, s := range want {
		b = append(b, key(s))
	}
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// sitesWhere are the sites of file mutation list gives on whose line keep
// holds.
func (m *mutateState) sitesWhere(file string, keep func(listedSite) bool) ([]listedSite, error) {
	sites, err := m.sitesOf(file)
	if err != nil {
		return nil, err
	}
	var out []listedSite
	for _, s := range sites {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// subsequence is whether want's words are in words, in order.
func subsequence(words, want []string) bool {
	i := 0
	for _, word := range words {
		if i < len(want) && word == want[i] {
			i++
		}
	}
	return i == len(want)
}

// contiguous is whether want's words are in words, one after another.
func contiguous(words, want []string) bool {
	for i := 0; i+len(want) <= len(words); i++ {
		if slices.Equal(words[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// testRecords makes the folder, outside the scratch folder, the project's
// tests record what they saw in, and gives its path.
func (m *mutateState) testRecords() (string, error) {
	if m.records != "" {
		return m.records, nil
	}
	dir, err := m.w.d.Folder()
	if err != nil {
		return "", err
	}
	m.records = dir
	return dir, nil
}

// testRecord is what one run of a project's tests recorded: where it ran,
// the real tree's folder or a copy, when its test began and ended, in
// milliseconds, and what else the project's test says.
type testRecord struct {
	Cwd       string   `json:"cwd"`
	Real      bool     `json:"real"`
	Start     int64    `json:"start"`
	End       int64    `json:"end"`
	Mutated   bool     `json:"mutated"`
	Make      bool     `json:"make"`
	Links     []string `json:"links"`
	Unchanged bool     `json:"unchanged"`
	Path      []string `json:"path"`
	File      string   `json:"file"`
}

// recorded are the runs of the project's tests, in the order they
// recorded them.
func (m *mutateState) runsRecorded() ([]testRecord, error) {
	if m.records == "" {
		return nil, errors.New("the project's tests record nothing")
	}
	data, err := m.w.d.Read(filepath.Join(m.records, "runs"))
	if err != nil {
		return nil, fmt.Errorf("the project's tests recorded no run: %v\n%s", err, m.w.report())
	}
	var out []testRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r testRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("the record %q is no run's: %v", line, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// goRecorder is a Go test file's code that records a run: record(entry)
// writes the entry, with where the test runs, to the records folder.
func goRecorder(pkg, records, real string, imports ...string) string {
	all := append([]string{"encoding/json", "os", "path/filepath", "testing"}, imports...)
	slices.Sort(all)
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport (\n", pkg)
	for _, name := range all {
		fmt.Fprintf(&b, "\t%q\n", name)
	}
	fmt.Fprintf(&b, ")\n\nconst records, real = %s, %s\n\n", strconv.Quote(records), strconv.Quote(real))
	b.WriteString(`// record writes entry, with the folder the test runs in and whether it
// is the real tree's, to the records folder.
func record(t *testing.T, entry map[string]any) {
	cwd, _ := os.Getwd()
	here, _ := filepath.EvalSymlinks(cwd)
	there, _ := filepath.EvalSymlinks(real)
	entry["cwd"], entry["real"] = here, here == there
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile(filepath.Join(records, "runs"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}
`)
	return b.String()
}
