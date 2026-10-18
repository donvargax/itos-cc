package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The owned-process scenarios of counted, concurrent and fail-fast runs, and
// of macOS.

func countedExecution(a map[string]any, name string) (map[string]any, error) {
	var found []map[string]any
	for _, mu := range objectList(a, "selected") {
		if mu["file"] == "main.go" && strictNamed(fmt.Sprint(mu["function"]), name) {
			found = append(found, mu)
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("counted execution %s: want one selected entry, got %v", name, found)
	}
	return found[0], nil
}

func (q *mutationExecution) concurrentOwned(ctx context.Context) error {
	if !CountedOn(q.m.w.d.Platform()) {
		return fmt.Errorf("%w: Unix process ownership", godog.ErrSkip)
	}
	keep := ""
	if q.owned != nil {
		keep = q.owned.keep
	}
	q.owned = &ownershipExecution{mode: "failfast", keep: keep}
	var err error
	q.peers, err = q.m.w.d.FixturePeers(ctx)
	if err != nil {
		return err
	}
	if err = q.installOwned("failfast", false, true); err != nil {
		return err
	}
	source := ownedTwoSource + "\nfunc Later(n int) bool { return n > 7 }\n"
	q.source["main.go"] = source
	if err = q.m.w.write("main.go", source); err != nil {
		return err
	}
	fixture := strings.ReplaceAll(ownershipTests, "CONTROL_PATH", strconv.Quote(filepath.Join(q.outside, "control.json")))
	fixture = strings.ReplaceAll(fixture, "GATE_TRIAL", `if Gate(6){name="Gate"}`)
	fixture = strings.ReplaceAll(fixture, "LATER_TRIAL", `if Later(7){name="Later"}`)
	if err = q.m.w.write("main_test.go", fixture); err != nil {
		return err
	}
	if err = q.m.w.d.Git(q.m.w.dir, "add", "main.go", "main_test.go"); err != nil {
		return err
	}
	if err = q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "parallel owned fixture"); err != nil {
		return err
	}
	if err = q.m.w.d.Tests(q.m.w.dir, Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond, TimingOut: []Site{{File: "main.go", Line: 3, Original: ">", Replacement: ">="}}, Surviving: []Site{{File: "main.go", Line: 5, Original: ">", Replacement: ">="}}}); err != nil {
		return err
	}
	if err = q.unrelated(ctx); err != nil {
		return err
	}
	args := q.ownedArgs(false)
	for i := range args {
		if args[i] == "--workers" {
			args[i+1] = "2"
		}
	}
	args = append(args, "--fail-fast", "--timeout-factor", "10000")
	q.owned.run, err = q.startOwned(ctx, args...)
	if err != nil {
		return err
	}
	if err = q.captureTree(ctx, "Compare"); err != nil {
		return err
	}
	ready, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	q.owned.trigger, err = q.peers.Capture(ready, filepath.Join(q.outside, "Gate-command.json"))
	return err
}

func (q *mutationExecution) releaseSurvivor(ctx context.Context) error {
	if q.owned.trigger == nil || len(q.owned.peers) != 3 {
		return fmt.Errorf("survivor release requires captured workers")
	}
	live, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := q.owned.trigger.Ping(live); err != nil {
		return err
	}
	for _, peer := range q.owned.peers {
		if err := peer.Ping(live); err != nil {
			return err
		}
	}
	q.owned.released = time.Now()
	if err := q.writeControl("failfast", false, true, ""); err != nil {
		return err
	}
	// A kept judgment's cleanup fails only at the shared deadline, so its
	// run returns after it.
	if q.owned.keep != "" {
		bounded, stop := context.WithDeadline(ctx, q.owned.released.Add(15*time.Second))
		defer stop()
		r, err := q.owned.run.Wait(bounded)
		return q.returned(ctx, r, err, time.Now())
	}
	bounded, stop := context.WithDeadline(ctx, q.owned.released.Add(5*time.Second))
	defer stop()
	return q.waitOwned(bounded)
}

func (q *mutationExecution) failFastCancelled() error {
	mu, err := executionMutant(q.result, "main.go", "Compare")
	if err != nil {
		return err
	}
	trigger, err := executionMutant(q.result, "main.go", "Gate")
	if err != nil {
		return err
	}
	later, err := executionMutant(q.result, "main.go", "Later")
	if err != nil {
		return err
	}
	stop := executionMap(q.result, "stop")
	subject := executionMap(stop, "subject")
	if q.m.w.exit != 1 || stop["rule"] != "mutation.survived" || stop["stopped"] != true || subject["file"] != "main.go" || subject["function"] != trigger["function"] || mu["state"] != "cancelled" || mu["outcome"] != nil || trigger["state"] != "completed" || trigger["outcome"] != "survived" || later["state"] != "unattempted" || later["outcome"] != nil {
		return fmt.Errorf("in-flight cancellation: %v", q.result)
	}
	return executionSurvivorProblem(q.result, trigger)
}

func (q *mutationExecution) sharedStop(ctx context.Context) error {
	if q.owned.joinedAt.IsZero() || q.owned.released.IsZero() || q.owned.joinedAt.Sub(q.owned.released) > 5*time.Second {
		return fmt.Errorf("owned join exceeded release-relative shared bound")
	}
	if err := q.ownedEnded(ctx); err != nil {
		return err
	}
	deadline := q.owned.released.Add(5 * time.Second)
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := q.owned.trigger.EOF(bounded); err != nil {
		return err
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("trigger EOF exceeded release-relative shared bound")
	}
	return nil
}

func (q *mutationExecution) macProject(ctx context.Context) error {
	if q.m.w.d.Platform() != "darwin" {
		return fmt.Errorf("%w: native macOS counted execution", godog.ErrSkip)
	}
	q.owned = &ownershipExecution{mode: "cancel"}
	q.command = "go test -count=1 -failfast ."
	var err error
	q.peers, err = q.m.w.d.FixturePeers(ctx)
	if err != nil {
		return err
	}
	if err = q.installOwned("characterize", false, true); err != nil {
		return err
	}
	if err = q.m.w.d.Coverage(q.m.w.dir, Coverage{Language: "go", Report: "mode: set\nexample.com/ownership/main.go:3.28,3.42 1 1\nexample.com/ownership/main.go:5.25,5.39 1 1\n"}); err != nil {
		return err
	}
	return q.characterizeOrder()
}

func (q *mutationExecution) macCountOne() error {
	args := q.ownedArgs(true)
	for i := range args {
		if args[i] == "--count" {
			args[i+1] = "1"
		}
	}
	return q.run(strings.Join(quoteWords(args), " "))
}

func (q *mutationExecution) macCounted() error {
	selected := objectList(q.result, "selected")
	sampling := executionMap(q.result, "sampling")
	if q.m.w.exit != 0 || len(checkProblems(q.result)) != 0 || len(selected) != 1 || selected[0]["state"] != "judged" || selected[0]["outcome"] != "killed" || selected[0]["scope"] != "own" || sampling["selected"] != float64(1) || sampling["executed"] != float64(1) {
		return fmt.Errorf("native macOS sampled judgment: %v", q.result)
	}
	return nil
}

func (q *mutationExecution) macInterrupted(ctx context.Context) error {
	q.owned.block = q.owned.order[1]
	if err := q.writeControl("cancel", false, false, q.owned.block); err != nil {
		return err
	}
	if err := q.declareOwned("cancel", true, q.owned.block); err != nil {
		return err
	}
	if err := q.m.w.d.Write(filepath.Join(q.outside, "trace.jsonl"), nil); err != nil {
		return err
	}
	var err error
	q.owned.run, err = q.startOwned(ctx, q.ownedArgs(true)...)
	if err != nil {
		return err
	}
	if err = q.captureTree(ctx, q.owned.block); err != nil {
		return err
	}
	if err = q.cancelParent(ctx); err != nil {
		return err
	}
	if err = q.ownedEnded(ctx); err != nil {
		return err
	}
	if err = q.ownedRemoved(ctx); err != nil {
		return err
	}
	if err = q.cancelOutcome(); err != nil {
		return err
	}
	return q.completedFirst()
}

func (q *mutationExecution) countedHelp() error {
	code, out, _, err := q.m.w.capture(q.m.w.dir, q.m.w.env(), "help", "mutation", "run")
	if err != nil {
		return err
	}
	return executionCountedHelp(out, code)
}

func executionCountedHelp(out string, code int) error {
	if code != 0 || !strings.Contains(out, "Linux and macOS") || !strings.Contains(out, "#29") || strings.Contains(out, "Linux only") || strings.Contains(out, "Linux-only") || !strings.Contains(out, "--count") {
		return fmt.Errorf("counted native help: exit%d %s", code, out)
	}
	return nil
}
