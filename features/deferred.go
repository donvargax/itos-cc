package features

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// serveDeferred is why the in-process driver leaves a mutation scenario that
// also observes serve to the binary driver: serve stays on its legacy
// handler, its moves deferred while it leaves for a web app of its own.
const serveDeferred = "observes serve as well as mutation; serve stays on its legacy handler, its moves (T-102 to T-108, T-112, T-113) deferred while serve leaves for its own web app, so the binary driver alone runs it until that work"

// Deferred are the scenarios of InProcess files that the in-process driver
// leaves to the binary driver for now, by ID tag, each with why, naming the
// work that owns it. The binary driver runs every one of them; the
// in-process run logs those its selection matches (Defer). Unlike
// BinaryOnly, which a scenario only the binary can run carries for good,
// each entry goes once its owner's work lets the scenario run in process.
var Deferred = map[string]string{
	"@ID-MUT-134": serveDeferred,
	"@ID-MUT-153": serveDeferred,
	"@ID-MUT-154": serveDeferred,
	"@ID-MUT-163": serveDeferred,
	"@ID-MUT-207": serveDeferred,
}

// Defer is sel, the selection's of the feature files in dir that a driver
// runs, without the scenarios deferred names, and the deferred tags it
// leaves out that the selection matches, sorted. A tag deferred names that
// no scenario of a feature file in dir carries is an error, so an entry
// cannot outlive its scenario.
func Defer(sel Selection, selection, dir string, deferred map[string]string) (Selection, []string, error) {
	if len(deferred) == 0 {
		return sel, nil, nil
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return Selection{}, nil, err
	}
	every, err := featureTags(all)
	if err != nil {
		return Selection{}, nil, err
	}
	own, err := featureTags(sel.Paths)
	if err != nil {
		return Selection{}, nil, err
	}
	var pattern *regexp.Regexp
	if selection != "" {
		if pattern, err = regexp.Compile(selection); err != nil {
			return Selection{}, nil, err
		}
	}
	var left []string
	var terms strings.Builder
	for _, tag := range slices.Sorted(maps.Keys(deferred)) {
		if !slices.Contains(every, tag) {
			return Selection{}, nil, fmt.Errorf("features.Deferred lists %s, which no scenario carries", tag)
		}
		if !slices.Contains(own, tag) {
			continue
		}
		terms.WriteString("&&~" + tag)
		if pattern == nil || pattern.MatchString(tag) {
			left = append(left, tag)
		}
	}
	if sel.Filter == "" || terms.Len() == 0 {
		return sel, left, nil
	}
	each := strings.Split(sel.Filter, ",")
	for i := range each {
		each[i] += terms.String()
	}
	sel.Filter = strings.Join(each, ",")
	return sel, left, nil
}
