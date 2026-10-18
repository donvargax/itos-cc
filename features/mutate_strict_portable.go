// Strict Go evidence carried between checkouts (slice-10, issues #36 and
// #37): how the same Go is chosen, and Go files no build includes.
package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) portableSteps(sc *godog.ScenarioContext) {
	sc.Step(`^fresh strict coverage evidence for an unchanged Go function, recorded with (.+)$`, func(recorded string) error {
		env, err := q.goEnvironment(recorded)
		if err != nil {
			return err
		}
		q.env = env
		return q.unchangedEvidence()
	})
	sc.Step(`^I run mutation check with --fail-uncovered --json for the function's file, with (.+)$`, func(checked string) error {
		env, err := q.goEnvironment(checked)
		if err != nil {
			return err
		}
		q.env = env
		return q.run("mutation check --fail-uncovered --json " + q.file)
	})
	sc.Step(`^no function is "mutation.coverage-stale" and the exit code is 0$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if ps := q.findings(a, "mutation.coverage-stale", ""); len(ps) != 0 {
			return fmt.Errorf("evidence read stale: %v", ps)
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^with GOFLAGS=-buildvcs=false its problem is "mutation.coverage-stale", naming "@go-env"$`, func() error {
		q.env = append(q.env, "GOFLAGS=-buildvcs=false")
		q.input = "@go-env"
		return q.staleNaming()
	})
	sc.Step(`^fresh strict coverage evidence for an unchanged Go function, its snapshot committed$`, func() error {
		if err := q.unchangedEvidence(); err != nil {
			return err
		}
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "strict evidence"}} {
			if err := q.m.w.git(args...); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^a Go file (\S+) was added, (.+)$`, q.addUnbuilt)
	sc.Step(`^a committed Go file added beside the function's own file makes its problem "mutation.coverage-stale", naming that file$`, func() error {
		q.input = "added.go"
		if err := q.m.w.write(q.input, "package main\n\nvar Added = 1\n"); err != nil {
			return err
		}
		for _, args := range [][]string{{"add", q.input}, {"commit", "-qm", "a built file"}} {
			if err := q.m.w.git(args...); err != nil {
				return err
			}
		}
		return q.staleNaming()
	})
}

// goEnvironment is the run's variables an example names: NAME=value set, or
// NAME alone, which the drivers take as the variable unset.
func (q *strictMutation) goEnvironment(example string) ([]string, error) {
	switch example {
	case "GOTOOLCHAIN unset":
		return []string{"GOTOOLCHAIN"}, nil
	case "GOTOOLCHAIN=local", "GOTOOLCHAIN=auto":
		return []string{example}, nil
	case "GOOS unset":
		return []string{"GOOS"}, nil
	case "GOOS set to the system's own":
		return []string{"GOOS=" + q.m.w.d.Platform()}, nil
	}
	return nil, fmt.Errorf("unknown Go environment %q", example)
}

// staleNaming runs the strict check and holds its one stale problem for
// Positive to naming q.input, exiting 1.
func (q *strictMutation) staleNaming() error {
	if err := q.run("mutation check --fail-uncovered --json " + q.file); err != nil {
		return err
	}
	a, err := q.object()
	if err != nil {
		return err
	}
	ps := q.findings(a, "mutation.coverage-stale", "Positive")
	if len(ps) != 1 || !strings.Contains(fmt.Sprint(ps[0]["message"]), q.input) {
		return fmt.Errorf("stale finding does not name %s: %v", q.input, ps)
	}
	return q.m.w.exitsWith(1)
}

// addUnbuilt adds a Go file no build of the checkout includes, as the
// example says how.
func (q *strictMutation) addUnbuilt(file, how string) error {
	text := "package x\n\nvar X = 1\n"
	var git [][]string
	switch how {
	case "inside the git directory", "untracked, in a directory go ignores":
	case "in a directory .gitignore ignores":
		if err := q.m.w.write(".gitignore", "dist/\n"); err != nil {
			return err
		}
		git = [][]string{{"add", ".gitignore"}, {"commit", "-qm", "ignore dist"}}
	case "committed, in a testdata directory":
		git = [][]string{{"add", file}, {"commit", "-qm", "a fixture"}}
	case "untracked, beside the function's own file":
		text = "package main\n\nvar Scratch = 1\n"
	default:
		return fmt.Errorf("unknown way to add %s: %q", file, how)
	}
	if err := q.m.w.write(file, text); err != nil {
		return err
	}
	for _, args := range git {
		if err := q.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}
