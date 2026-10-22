package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// uncoveredSteps are slice-23's: the refusal to except an uncovered site
// says it is on purpose and names the way out, and a killed site's does
// not.
func (e *exceptionMutation) uncoveredSteps(sc *godog.ScenarioContext) {
	sc.Step(`^its message says an exception covers survivors only, on purpose$`, func() error {
		p, err := e.uncoveredRefusal()
		if err != nil {
			return err
		}
		if message, _ := p["message"].(string); !strings.Contains(message, "covers survivors only, on purpose") {
			return fmt.Errorf("the message %q does not say an exception covers survivors only, on purpose", message)
		}
		return nil
	})
	sc.Step(`^its fix names a test that runs line 9, or routing the branch through code a test already reaches$`, func() error {
		p, err := e.uncoveredRefusal()
		if err != nil {
			return err
		}
		if fix, _ := p["fix"].(string); !strings.Contains(fix, "a test that runs line 9") || !strings.Contains(fix, "route the branch through code a test already reaches") {
			return fmt.Errorf("the fix %q names neither a test for line 9 nor the route through tested code", fix)
		}
		return nil
	})
	sc.Step(`^refusing the killed site at src/board\.ts:5:9 says neither$`, func() error {
		if err := e.run("mutation except src/board.ts:5:9 --reason 'no test can reach it' --json"); err != nil {
			return err
		}
		if e.s.m.w.exit != 80 {
			return fmt.Errorf("refusing the killed site: exit %d", e.s.m.w.exit)
		}
		p, err := e.uncoveredRefusal()
		if err != nil {
			return err
		}
		message, _ := p["message"].(string)
		fix, _ := p["fix"].(string)
		if strings.Contains(message, "on purpose") || strings.Contains(fix, "already reaches") || strings.Contains(fix, "runs line") {
			return fmt.Errorf("the killed site's refusal says the uncovered one's words: %q, %q", message, fix)
		}
		return nil
	})
}

// uncoveredRefusal is the one exception.no-survivor problem of the last --json
// object.
func (e *exceptionMutation) uncoveredRefusal() (map[string]any, error) {
	a, err := e.object()
	if err != nil {
		return nil, err
	}
	p := checkProblems(a)
	if len(p) != 1 || p[0]["rule"] != "exception.no-survivor" {
		return nil, fmt.Errorf("problems %v, want one exception.no-survivor", p)
	}
	return p[0], nil
}
