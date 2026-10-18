package features

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	gherkin "github.com/cucumber/gherkin/go/v42"
)

// Selecting the scenarios to run by tag, and the tags each scenario of a
// feature file carries.

// Selection is what a -scenarios expression selects for one driver.
type Selection struct {
	// Paths are the driver's active feature files, with dir joined to each.
	Paths []string
	// Skipped are the feature filenames ITOS_CC_SKIP_FEATURES leaves out.
	Skipped []string
	// Filter is godog's tag filter over the driver's files, "" when the
	// expression selects none of their scenarios.
	Filter string
	// Unconverted are the tags it matches only in feature files with no
	// steps yet (q-52), held by cmd/itos-cc's Go tests.
	Unconverted []string
	// Unmoved are the tags it matches only in converted files the driver
	// does not run: the binary driver's alone until their command moves.
	Unmoved []string
	// Unstepped are the tags it matches only on scenarios Unstepped
	// lists, whose steps are not written yet, held by cmd/itos-cc's Go
	// tests.
	Unstepped []string
}

// TagFilter is what a selection selects of the feature files in dir for a
// driver that runs files of them and cannot run the scenarios tagged skip:
// every live scenario of files without one, else the live scenarios of
// files carrying a tag the expression matches, with the tags it matches
// only in other files, which the driver does not run. An expression that
// matches no tag of a converted file is an error, so a selection never
// runs nothing; one that matches only tags of converted files the driver
// does not run selects nothing for it, the binary driver running those. A
// scenario only one driver can run carries @binary-only or
// @in-process-only, never a copy, and the other driver passes it by. The
// scenarios Unstepped lists are left out of every run: a tag only they
// carry is logged, as one of a file the driver does not run is, and one
// that others carry too selects those others alone.
// ITOS_CC_SKIP_FEATURES leaves its comma-separated filenames out of Paths;
// their tags still match valid selections, and Skipped names them for the
// runners to log. Carrier validation still reads every converted file.
//
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so a selection is that expression,
// turned here into godog's filter. A scenario tagged @wip never runs.
func TagFilter(selection, dir string, files []string, skip string) (Selection, error) {
	in := func(names []string) []string {
		var paths []string
		for _, name := range names {
			paths = append(paths, filepath.Join(dir, name))
		}
		return paths
	}
	omitted := map[string]bool{}
	for _, name := range strings.Split(os.Getenv("ITOS_CC_SKIP_FEATURES"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			omitted[name] = true
		}
	}
	var sel Selection
	for _, name := range files {
		if !omitted[name] {
			sel.Paths = append(sel.Paths, filepath.Join(dir, name))
		}
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return Selection{}, err
	}
	var leftOut []string
	for _, path := range all {
		if name := filepath.Base(path); omitted[name] {
			sel.Skipped = append(sel.Skipped, name)
			leftOut = append(leftOut, path)
		}
	}
	own, err := featureTags(sel.Paths)
	if err != nil {
		return Selection{}, err
	}
	carried, err := carriers(in(Converted))
	if err != nil {
		return Selection{}, err
	}
	// leaveOut is the filter's terms leaving out the Unstepped scenarios of
	// the driver's files among ids.
	leaveOut := func(ids []string) string {
		var terms strings.Builder
		for _, id := range ids {
			if slices.Contains(own, id) {
				terms.WriteString("&&~" + id)
			}
		}
		return terms.String()
	}
	if selection == "" {
		if len(sel.Paths) > 0 {
			sel.Filter = "~@wip&&~" + skip + leaveOut(Unstepped)
		}
		return sel, nil
	}
	pattern, err := regexp.Compile(selection)
	if err != nil {
		return Selection{}, err
	}
	converted, err := featureTags(in(Converted))
	if err != nil {
		return Selection{}, err
	}
	skipped, err := featureTags(leftOut)
	if err != nil {
		return Selection{}, err
	}
	everyTag, err := featureTags(all)
	if err != nil {
		return Selection{}, err
	}
	var each []string
	matched := false
	for _, tag := range everyTag {
		if !pattern.MatchString(tag) {
			continue
		}
		c := carried[tag]
		switch {
		case c.unstepped != nil && !c.stepped:
			sel.Unstepped = append(sel.Unstepped, tag)
		case slices.Contains(own, tag):
			each = append(each, tag+"&&~@wip&&~"+skip+leaveOut(c.unstepped))
		case slices.Contains(skipped, tag):
			// A real tag whose file was deliberately left out is a valid
			// selection, even when this driver has nothing left to run.
		case slices.Contains(converted, tag):
			sel.Unmoved = append(sel.Unmoved, tag)
		default:
			sel.Unconverted = append(sel.Unconverted, tag)
			continue
		}
		matched = true
	}
	if !matched {
		return sel, &noMatch{selection, sel.Unconverted}
	}
	sel.Filter = strings.Join(each, ",")
	return sel, nil
}

// noMatch is a selection matching no tag of a converted file, with the
// tags it matches in files with no steps yet.
type noMatch struct {
	selection   string
	unconverted []string
}

func (e *noMatch) Error() string {
	if len(e.unconverted) > 0 {
		return "no scenario this harness runs has a tag that " + e.selection + " matches: " +
			strings.Join(e.unconverted, " ") + " are in feature files with no steps yet (q-52), held by cmd/itos-cc's Go tests"
	}
	return "no scenario has a tag that " + e.selection + " matches"
}

// carrier is which scenarios carry a tag: the Unstepped ones, by their ID
// tag, and whether any other does.
type carrier struct {
	unstepped []string
	stepped   bool
}

// carriers is, for every tag the scenarios of files carry, theirs or one
// their feature, rule or examples carry, as godog reads them, which carry
// it. It fails when an ID Unstepped lists is no scenario's of files.
func carriers(files []string) (map[string]carrier, error) {
	carried := map[string]carrier{}
	for _, file := range files {
		data, err := os.ReadFile(file) //nolint:gosec // a file of the scenario's own temporary folder, as its step names it
		if err != nil {
			return nil, err
		}
		n := 0
		newID := func() string { n++; return strconv.Itoa(n) }
		doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), newID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for _, pickle := range gherkin.Pickles(*doc, file, newID) {
			id := ""
			for _, tag := range pickle.Tags {
				if slices.Contains(Unstepped, tag.Name) {
					id = tag.Name
				}
			}
			for _, tag := range pickle.Tags {
				c := carried[tag.Name]
				if id == "" {
					c.stepped = true
				} else if !slices.Contains(c.unstepped, id) {
					c.unstepped = append(c.unstepped, id)
				}
				carried[tag.Name] = c
			}
		}
	}
	for _, id := range Unstepped {
		if carried[id].unstepped == nil {
			return nil, fmt.Errorf("features.Unstepped lists %s, which no scenario of a converted file carries", id)
		}
	}
	return carried, nil
}

// Every tag written in files, once each, sorted.
func featureTags(files []string) ([]string, error) {
	seen := map[string]bool{}
	for _, file := range files {
		f, err := os.Open(file) //nolint:gosec // a file of the scenario's own temporary folder, as its step names it
		if err != nil {
			return nil, err
		}
		lines := bufio.NewScanner(f)
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if !strings.HasPrefix(line, "@") {
				continue
			}
			for _, word := range strings.Fields(line) {
				if strings.HasPrefix(word, "#") {
					break
				}
				if strings.HasPrefix(word, "@") {
					seen[word] = true
				}
			}
		}
		_ = f.Close()
		if err := lines.Err(); err != nil {
			return nil, err
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}
