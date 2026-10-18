// A comment-only change asks no coverage evidence of its function
// (slice-11, issue #22), though strict Go evidence is bound to the file.
package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) commentSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go function with fresh results and fresh strict coverage evidence, committed at "base"$`, func() error {
		if err := q.unchangedEvidence(); err != nil {
			return err
		}
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "strict evidence"}, {"tag", "base"}} {
			if err := q.m.w.git(args...); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^a commit after "base" only added a comment at the end of one of its executable lines$`, func() error {
		line := "func Positive(n int) bool { return n > 0 }"
		text := q.source()
		if !strings.Contains(text, line) {
			return fmt.Errorf("%s lacks %q", q.file, line)
		}
		if err := q.m.w.write(q.file, strings.Replace(text, line, line+" // above zero", 1)); err != nil {
			return err
		}
		for _, args := range [][]string{{"add", q.file}, {"commit", "-qm", "only a comment of Positive"}} {
			if err := q.m.w.git(args...); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^it reports no problem and the exit code is 0$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if ps := checkProblems(a); len(ps) != 0 {
			return fmt.Errorf("problems %v", ps)
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^"itos-cc mutation check --since base --fail-uncovered" exits 0$`, func() error {
		if err := q.run("mutation check --since base --fail-uncovered"); err != nil {
			return err
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^"itos-cc mutation check --fail-uncovered --json" of its file reports "mutation\.coverage-stale" for it, naming that file$`, func() error {
		q.input = q.file
		return q.staleNaming()
	})
}
