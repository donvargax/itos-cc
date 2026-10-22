package domain

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	fileselection "github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// AdmitFresh inventories admitted units and ranks sites from actual frozen input
// observations. Both physical and memory plans use this one selection policy.
// A Go file only another system builds, as tools reports the platform in
// its folder (BuiltOn), admits no unit and no site, and the plan names it
// in NotBuilt.
func AdmitFresh(ctx context.Context, plan *FreshPlan, tree selport.Tree, around langport.Surroundings, tools Toolchain, sources, tracked []string, exclude fileselection.Exclusion, changed map[string][]LineRange, count int) error {
	committed := make(map[string]bool, len(tracked))
	for _, rel := range tracked {
		committed[rel] = true
	}
	var candidates []FreshCandidate
	var units []FreshUnit
	platforms := map[string]Platform{}
	plan.NotBuilt = nil
	for _, discoveredPath := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if exclude.Excludes(discoveredPath) {
			continue
		}
		rel, err := filepath.Rel(plan.FrozenRoot, discoveredPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !committed[rel] {
			return fmt.Errorf("discovery escaped committed tree: %q", rel)
		}
		ranges, isChanged := changed[rel]
		if changed != nil && !isChanged {
			continue
		}
		frozenPath := filepath.Join(plan.FrozenRoot, filepath.FromSlash(rel))
		source, err := tree.Read(ctx, frozenPath)
		if err != nil {
			return fmt.Errorf("read committed %s: %w", rel, err)
		}
		built, platform, err := builtHere(ctx, tools, platforms, frozenPath, source)
		if err != nil {
			return err
		}
		if !built {
			plan.NotBuilt = append(plan.NotBuilt, NotBuilt{Path: rel, Platform: platform})
			continue
		}
		file, err := lang.Parse(ctx, around, lang.Detect(rel), frozenPath, source)
		if err != nil {
			return fmt.Errorf("parse committed %s: %w", rel, err)
		}
		for _, unit := range file.Units {
			if changed != nil && !OverlapsRanges(ranges, unit.StartLine, unit.EndLine) {
				continue
			}
			function := stableFunction(file, unit, rel, committed)
			identity := fmt.Sprintf("%s@%d:%d:%s", function, unit.StartLine, unit.EndLine, UnitHash(file, unit))
			units = append(units, FreshUnit{Path: rel, Function: function, Identity: identity,
				StartLine: unit.StartLine, EndLine: unit.EndLine, Hash: UnitHash(file, unit)})
		}
		sites := Sites(file)
		for _, site := range sites {
			unit := file.Units[site.Unit]
			if changed != nil && !OverlapsRanges(ranges, unit.StartLine, unit.EndLine) {
				continue
			}
			function := stableFunction(file, unit, rel, committed)
			unitIdentity := fmt.Sprintf("%s@%d:%d:%s", function, unit.StartLine, unit.EndLine, UnitHash(file, unit))
			identity := fmt.Sprintf("%s:%s:%s", rel, unitIdentity, site.Key())
			rank := FreshRank(plan.Seed, rel, identity)
			candidates = append(candidates, FreshCandidate{Identity: identity, Path: rel, Function: function,
				UnitIdentity: unitIdentity, Site: site, Rank: rank})
		}
		file.Close()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	SortFreshCandidates(candidates)
	plan.Eligible = slices.Clone(candidates)
	sort.Slice(units, func(i, j int) bool {
		if units[i].Path != units[j].Path {
			return units[i].Path < units[j].Path
		}
		if units[i].StartLine != units[j].StartLine {
			return units[i].StartLine < units[j].StartLine
		}
		return units[i].Identity < units[j].Identity
	})
	plan.Units = slices.Clone(units)
	limit := min(count, len(candidates))
	plan.Selected = slices.Clone(candidates[:limit])
	plan.Omitted = slices.Clone(candidates[limit:])
	return nil
}

func stableFunction(file *lang.File, unit lang.Unit, rel string, committed map[string]bool) string {
	namespace := unit.Namespace
	switch file.Spec.Name {
	case "python":
		namespace = stableModuleNamespace(rel, committed, "pyproject.toml", "setup.py", "setup.cfg")
		namespace = strings.TrimSuffix(namespace, ".__init__")
	case "typescript":
		namespace = stableModuleNamespace(rel, committed, "package.json", "tsconfig.json")
	}
	return namespace + "#" + unit.Name
}

func stableModuleNamespace(rel string, committed map[string]bool, markers ...string) string {
	base := rel
	for dir := path.Dir(rel); ; dir = path.Dir(dir) {
		found := false
		for _, marker := range markers {
			if committed[path.Join(dir, marker)] {
				base = strings.TrimPrefix(rel, dir+"/")
				found = true
				break
			}
		}
		if found {
			break
		}
		if dir == "." {
			break
		}
	}
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.TrimPrefix(base, "src/")
	base = strings.TrimPrefix(base, "lib/")
	return strings.ReplaceAll(base, "/", ".")
}

// builtHere says whether this platform builds the Go file at path holding
// source, and the platform, asked of tools once a folder in platforms; any
// other file is built.
func builtHere(ctx context.Context, tools Toolchain, platforms map[string]Platform, path string, source []byte) (bool, Platform, error) {
	spec := lang.Detect(path)
	name := filepath.Base(path)
	if tools == nil || spec == nil || spec.Name != "go" || !platformBound(name, string(source)) {
		return true, Platform{}, nil
	}
	dir := filepath.Dir(path)
	p, seen := platforms[dir]
	if !seen {
		var failure InputFailure
		if p, failure = tools.GoPlatform(ctx, dir); failure != nil {
			return false, p, failure
		}
		platforms[dir] = p
	}
	return BuiltOn(name, string(source), p), p, nil
}
