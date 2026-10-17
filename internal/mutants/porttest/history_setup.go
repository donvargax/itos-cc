package porttest

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// Git applies the bounded fixture setup operations to independent Git state.
// It does not execute commands or supply scripted observation results.
func (r *HistoryRepository) Git(root, dir string, working map[string]string, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing fixture Git operation")
	}
	verb, rest := args[0], args[1:]
	relative := func(name string) (string, error) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if !historyWithin(root, path) {
			return "", fmt.Errorf("fixture Git path escapes root: %s", name)
		}
		rel, err := filepath.Rel(root, path)
		return filepath.ToSlash(rel), err
	}
	stage := func(all bool, names []string) error {
		var selected []string
		for _, name := range names {
			rel, err := relative(name)
			if err != nil {
				return err
			}
			selected = append(selected, rel)
		}
		matches := func(path string) bool {
			if all {
				return true
			}
			for _, name := range selected {
				if name == "." || path == name || strings.HasPrefix(path, name+"/") {
					return true
				}
			}
			return false
		}
		for path := range r.Index {
			if matches(path) {
				delete(r.Index, path)
				delete(r.IndexLinks, path)
			}
		}
		for abs, text := range working {
			if !historyWithin(root, abs) {
				continue
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return err
			}
			path := filepath.ToSlash(rel)
			if matches(path) {
				r.Index[path] = text
				if r.WorkingLinks[path] {
					if r.IndexLinks == nil {
						r.IndexLinks = map[string]bool{}
					}
					r.IndexLinks[path] = true
				}
			}
		}
		return nil
	}
	switch verb {
	case "config":
		if len(rest) != 2 || strings.HasPrefix(rest[0], "-") {
			return fmt.Errorf("unsupported fixture Git config shape %q", rest)
		}
		if r.Config == nil {
			r.Config = map[string]string{}
		}
		r.Config[rest[0]] = rest[1]
		return nil
	case "add":
		all := slices.Contains(rest, "-A") || slices.Contains(rest, "--all")
		var names []string
		for _, arg := range rest {
			if !strings.HasPrefix(arg, "-") {
				names = append(names, arg)
			}
		}
		if !all && len(names) == 0 {
			return fmt.Errorf("fixture Git add needs paths")
		}
		return stage(all, names)
	case "commit":
		message := "fixture"
		for i := 0; i < len(rest); i++ {
			arg := rest[i]
			switch arg {
			case "-q", "--quiet", "--allow-empty":
				continue
			case "-a", "--all", "-am", "-qam", "-m", "-qm":
			default:
				return fmt.Errorf("unsupported fixture Git commit argument %s", arg)
			}
			if arg == "-a" || arg == "--all" || arg == "-am" || arg == "-qam" {
				for path := range r.Index {
					abs := filepath.Join(root, filepath.FromSlash(path))
					if text, exists := working[abs]; exists {
						r.Index[path] = text
					} else {
						delete(r.Index, path)
					}
				}
			}
			if arg == "-m" || arg == "-qm" || arg == "-am" || arg == "-qam" {
				if i+1 == len(rest) {
					return fmt.Errorf("fixture Git commit message missing")
				}
				i++
				message = rest[i]
			}
		}
		r.Commit(message)
		return nil
	case "tag", "branch":
		if len(rest) == 2 && (rest[0] == "-d" || rest[0] == "--delete") {
			if _, exists := r.Refs[rest[1]]; !exists || r.Branches[rest[1]] != (verb == "branch") {
				return fmt.Errorf("fixture Git %s not found: %s", verb, rest[1])
			}
			if verb == "branch" && r.Branch == rest[1] {
				return fmt.Errorf("fixture Git cannot delete the current branch %s", rest[1])
			}
			delete(r.Refs, rest[1])
			delete(r.Branches, rest[1])
			return nil
		}
		force := false
		var names []string
		for _, arg := range rest {
			if arg == "-f" || arg == "--force" {
				force = true
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unsupported fixture Git %s flag %s", verb, arg)
			}
			names = append(names, arg)
		}
		if len(names) == 0 || len(names) > 2 {
			return fmt.Errorf("unsupported fixture Git %s shape", verb)
		}
		if _, exists := r.Refs[names[0]]; exists && !force {
			return fmt.Errorf("fixture Git %s ref already exists: %s", verb, names[0])
		}
		ref := "HEAD"
		if len(names) == 2 {
			ref = names[1]
		}
		id, failure := (History{Repository: r}).resolveSetup(ref)
		if failure != nil {
			return failure
		}
		if _, exists := r.Refs[names[0]]; exists && r.Branches[names[0]] != (verb == "branch") {
			return fmt.Errorf("ambiguous fixture branch/tag name: %s", names[0])
		}
		r.Refs[names[0]] = id
		if verb == "branch" {
			r.Branches[names[0]] = true
		}
		return nil
	case "checkout", "switch":
		create := false
		var names []string
		for _, arg := range rest {
			switch arg {
			case "-b", "-qb", "-B", "-c":
				create = true
			case "-q":
			default:
				if strings.HasPrefix(arg, "-") {
					return fmt.Errorf("unsupported fixture Git %s flag %s", verb, arg)
				}
				names = append(names, arg)
			}
		}
		if len(names) == 0 || len(names) > 2 {
			return fmt.Errorf("unsupported fixture Git %s shape", verb)
		}
		target := names[0]
		if create {
			ref := "HEAD"
			if len(names) == 2 {
				ref = names[1]
			}
			id, err := (History{Repository: r}).resolveSetup(ref)
			if err != nil {
				return err
			}
			r.Refs[target] = id
			r.Branches[target] = true
		}
		if verb == "switch" && !r.Branches[target] {
			return fmt.Errorf("fixture Git switch requires a branch: %s", target)
		}
		id, err := (History{Repository: r}).resolveSetup(target)
		if err != nil {
			return err
		}
		for path, text := range r.Index {
			if current, ok := working[filepath.Join(root, filepath.FromSlash(path))]; ok && current != text {
				return fmt.Errorf("fixture Git checkout has unstaged changes: %s", path)
			}
		}
		next := r.Commits[id].Files
		for path := range r.Index {
			if _, keep := next[path]; !keep {
				delete(working, filepath.Join(root, filepath.FromSlash(path)))
			}
		}
		for path, text := range next {
			working[filepath.Join(root, filepath.FromSlash(path))] = text
		}
		r.Index = maps.Clone(next)
		if r.Branches[target] {
			r.Branch = target
			r.Detached = ""
		} else {
			r.Branch = ""
			r.Detached = id
		}
		return nil
	case "mv":
		if len(rest) != 2 {
			return fmt.Errorf("unsupported fixture Git mv shape")
		}
		old, err := relative(rest[0])
		if err != nil {
			return err
		}
		next, err := relative(rest[1])
		if err != nil {
			return err
		}
		oldAbs, newAbs := filepath.Join(root, filepath.FromSlash(old)), filepath.Join(root, filepath.FromSlash(next))
		text, exists := working[oldAbs]
		if !exists {
			return fmt.Errorf("fixture Git mv source missing: %s", old)
		}
		if _, tracked := r.Index[old]; !tracked {
			return fmt.Errorf("fixture Git mv source untracked: %s", old)
		}
		working[newAbs] = text
		delete(working, oldAbs)
		delete(r.Index, old)
		r.Index[next] = text
		return nil
	default:
		return fmt.Errorf("unsupported fixture Git operation %s", verb)
	}
}
