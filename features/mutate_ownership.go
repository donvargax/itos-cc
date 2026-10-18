package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type ownershipExecution struct {
	mode             string
	run              Running
	peers, unrelated []FixturePeer
	records          []ownedProcessRecord
	returned         bool
	released         time.Time
	joinedAt         time.Time
	trigger          FixturePeer
	order            []string
	block            string
}
type ownedProcessRecord struct {
	Nonce     string `json:"nonce"`
	PID       int    `json:"pid"`
	ParentPID int    `json:"parent_pid"`
	Role      string `json:"role"`
	Function  string `json:"function"`
	Cwd       string `json:"cwd"`
	Original  string `json:"original_root"`
	SHA       string `json:"source_sha256"`
	Closed    bool   `json:"closed_streams"`
}

func (q *mutationExecution) ownershipSteps(sc *godog.ScenarioContext) {
	sc.Step(`^mutation commands run on (Linux|macOS|Windows)$`, func(platform string) error {
		native := map[string]string{"Linux": "linux", "macOS": "darwin", "Windows": "windows"}[platform]
		if q.m.w.d.Platform() != native {
			return fmt.Errorf("%w: native %s process ownership", godog.ErrSkip, platform)
		}
		q.owned = &ownershipExecution{}
		return nil
	})
	sc.Step(`^a test command starts ordinary owned child and grandchild processes that keep running and hold output open$`, func(ctx context.Context) error { return q.ownedProject(ctx, "timeout", false, false) })
	sc.Step(`^the mutant reaches its own timeout$`, q.timeoutReturn)
	sc.Step(`^all owned processes are terminated and output waiters are joined before the command returns$`, q.ownedEnded)
	sc.Step(`^the mutant is still counted killed by timeout$`, q.timeoutOutcome)
	sc.Step(`^cleanup is bounded and the worker copy can be restored and removed$`, q.ownedRemoved)
	sc.Step(`^unrelated processes remain untouched$`, q.unrelatedLive)
	sc.Step(`^a baseline or mutant test command starts an ordinary owned descendant and then exits successfully$`, func(ctx context.Context) error { return q.ownedProject(ctx, "success", true, false) })
	sc.Step(`^that descendant could continue work after the parent exits, even with its output streams closed$`, q.closedLive)
	sc.Step(`^the command scope returns$`, q.successReturn)
	sc.Step(`^the descendant has stopped and cannot continue fixture work after return$`, q.ownedEnded)
	sc.Step(`^the parent's actual successful exit is retained, not invented as timeout or a failed baseline$`, q.successOutcome)
	sc.Step(`^after the (?:run|stopped run) returns, its owned fixture peers have terminated and worker copies are removed$`, q.ownedRemoved)
	sc.Step(`^a supervised command and its owned child are running under a parent cancellation context$`, func(ctx context.Context) error { return q.ownedProject(ctx, "cancel", false, true) })
	sc.Step(`^the parent cancels without the mutant's own deadline expiring$`, q.cancelParent)
	sc.Step(`^command admission stops and the owned tree is terminated within the shared cleanup deadline$`, q.ownedEnded)
	sc.Step(`^process/output waiters are joined before return$`, q.ownedRemoved)
	sc.Step(`^the result identifies cancellation, not a killed or timed-out mutant$`, q.cancelOutcome)
	sc.Step(`^a judgment completed before the cancellation retains its actual result$`, q.completedFirst)
	sc.Step(`^two workers, one judging a mutant whose test command keeps owned descendants running$`, q.concurrentOwned)
	sc.Step(`^the other worker's mutant becomes an actionable survivor$`, q.releaseSurvivor)
	sc.Step(`^the running judgment is cancelled with no killed, timed-out or survived outcome$`, q.failFastCancelled)
	sc.Step(`^its owned processes are terminated and joined within one shared five-second deadline from the stop$`, q.sharedStop)
	sc.Step(`^a committed project with eligible mutation sites$`, q.macProject)
	sc.Step(`^a count-one run judges it on macOS$`, q.macCountOne)
	sc.Step(`^it runs under owned supervision and reports its sampled judgment without a platform refusal$`, q.macCounted)
	sc.Step(`^an interrupted macOS counted run reports partial work and cleans up as on Linux$`, q.macInterrupted)
	sc.Step(`^the help names Linux and macOS as the supported platforms$`, q.countedHelp)

}

const ownedOneSource = "package main\n\nfunc Compare(n int) bool { return n > 5 }\n"
const ownedTwoSource = ownedOneSource + "\nfunc Gate(n int) bool { return n > 6 }\n"

func (q *mutationExecution) installOwned(mode string, closed, two bool) error {
	source := ownedOneSource
	if two {
		source = ownedTwoSource
	}
	if err := q.project(map[string]string{"main.go": source}); err != nil {
		return err
	}
	fixture := strings.ReplaceAll(ownershipTests, "CONTROL_PATH", strconv.Quote(filepath.Join(q.outside, "control.json")))
	gate := ""
	if two {
		gate = `if Gate(6){name="Gate"}`
	}
	fixture = strings.ReplaceAll(fixture, "GATE_TRIAL", gate)
	fixture = strings.ReplaceAll(fixture, "LATER_TRIAL", "")
	if err := q.m.w.write("main_test.go", fixture); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "main_test.go"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "owned fixture"); err != nil {
		return err
	}
	return q.writeControl(mode, closed, false, "")
}
func (q *mutationExecution) writeControl(mode string, closed, release bool, block string) error {
	if release {
		return q.m.w.d.Write(filepath.Join(q.outside, "control.json.release"), []byte("release\n"))
	}
	b, err := json.Marshal(map[string]any{"Mode": mode, "Closed": closed, "Release": release, "Block": block})
	if err != nil {
		return err
	}
	return q.m.w.d.Write(filepath.Join(q.outside, "control.json"), b)
}
func (q *mutationExecution) declareOwned(mode string, two bool, block string) error {
	t := Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond}
	sites := map[string]Site{"Compare": {File: "main.go", Line: 3, Original: ">", Replacement: ">="}, "Gate": {File: "main.go", Line: 5, Original: ">", Replacement: ">="}}
	switch mode {
	case "success":
		t.Surviving = []Site{sites["Compare"]}
	case "timeout":
		t.TimingOut = []Site{sites["Compare"]}
	case "cancel":
		t.TimingOut = []Site{sites[block]}
	}
	return q.m.w.d.Tests(q.m.w.dir, t)
}
func (q *mutationExecution) startOwned(ctx context.Context, args ...string) (Running, error) {
	env := append(q.m.w.env(), q.peers.Environment()...)
	env = append(env, "T80_ORIGINAL="+q.m.w.dir)
	r := Run{Dir: q.m.w.dir, Env: env, Args: args}
	p, err := q.m.w.d.Start(ctx, r)
	if err != nil {
		return nil, err
	}
	q.handles = append(q.handles, p)
	return p, nil
}
func (q *mutationExecution) ownedArgs(counted bool) []string {
	args := []string{"mutation", "run", "--json", "--no-annotate", "--workers", "1"}
	if !counted && q.owned.mode == "timeout" {
		args = append(args, "--timeout-factor", "0.01")
	}
	if !counted {
		args = append(args, "--no-coverage", "--test-command", q.command)
	}
	if counted {
		args = append(args, "--count", "2", "--seed", "T80-owned-order")
	}
	return args
}
func (q *mutationExecution) ownedProject(ctx context.Context, mode string, closed, two bool) error {
	if q.owned == nil {
		q.owned = &ownershipExecution{}
	}
	q.owned.mode = mode
	if two {
		q.command = "go test -count=1 -failfast ."
	}
	var err error
	q.peers, err = q.m.w.d.FixturePeers(ctx)
	if err != nil {
		return err
	}
	if err = q.installOwned(mode, closed, two); err != nil {
		return err
	}
	if two {
		if err = q.m.w.d.Coverage(q.m.w.dir, Coverage{Language: "go", Report: "mode: set\nexample.com/ownership/main.go:3.28,3.42 1 1\nexample.com/ownership/main.go:5.25,5.39 1 1\n"}); err != nil {
			return err
		}
		if err = q.characterizeOrder(); err != nil {
			return err
		}
		q.owned.block = q.owned.order[1]
		if err = q.writeControl(mode, closed, false, q.owned.block); err != nil {
			return err
		}
	}
	if err = q.declareOwned(mode, two, q.owned.block); err != nil {
		return err
	}
	if mode == "timeout" {
		if err = q.unrelated(ctx); err != nil {
			return err
		}
	}
	q.owned.run, err = q.startOwned(ctx, q.ownedArgs(two)...)
	if err != nil {
		return err
	}
	name := "Compare"
	if two {
		name = q.owned.block
	}
	return q.captureTree(ctx, name)
}
func (q *mutationExecution) characterizeOrder() error {
	if err := q.writeControl("characterize", false, false, ""); err != nil {
		return err
	}
	if err := q.declareOwned("characterize", true, ""); err != nil {
		return err
	}
	args := q.ownedArgs(true)
	if err := q.run(strings.Join(quoteWords(args), " ")); err != nil {
		return err
	}
	if q.m.w.exit != 0 {
		return fmt.Errorf("counted order characterization: %s", q.m.w.report())
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	var order []string
	for _, line := range strings.Split(strings.TrimSpace(string(trace)), "\n") {
		var r map[string]string
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			return err
		}
		if r["function"] != "baseline" {
			order = append(order, r["function"])
		}
	}
	if len(order) != 2 || order[0] == order[1] || !slices.Contains(order, "Compare") || !slices.Contains(order, "Gate") {
		return fmt.Errorf("actual counted execution order %v: %s", order, trace)
	}
	for _, name := range order {
		mu, err := countedExecution(q.result, name)
		if err != nil {
			return err
		}
		if mu["outcome"] != "killed" {
			return fmt.Errorf("characterized %s not actually killed: %v", name, mu)
		}
	}
	q.owned.order = order
	return q.m.w.d.Write(filepath.Join(q.outside, "trace.jsonl"), nil)
}
func quoteWords(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return out
}
func (q *mutationExecution) captureTree(ctx context.Context, name string) error {
	ready, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, role := range []string{"command", "child", "grandchild"} {
		peer, err := q.peers.Capture(ready, filepath.Join(q.outside, name+"-"+role+".json"))
		if err != nil {
			return fmt.Errorf("capture %s/%s: %w", name, role, err)
		}
		var r ownedProcessRecord
		if err = json.Unmarshal(peer.Record(), &r); err != nil {
			return err
		}
		if r.Role != role || r.Function != name || r.PID <= 0 || r.ParentPID <= 0 || r.Nonce == "" || r.Original == "" || r.Cwd == "" || r.SHA == "" {
			return fmt.Errorf("incomplete owned record: %+v", r)
		}
		rel, err := filepath.Rel(r.Original, r.Cwd)
		if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return fmt.Errorf("owned execution is not a worker copy: %+v", r)
		}
		source := q.source["main.go"]
		expression := "n > 5"
		if name == "Gate" {
			expression = "n > 6"
		}
		source = strings.Replace(source, expression, strings.Replace(expression, ">", ">=", 1), 1)
		hash := sha256.Sum256([]byte(source))
		if r.SHA != hex.EncodeToString(hash[:]) {
			return fmt.Errorf("owned source hash: %+v expected %x", r, hash)
		}
		q.owned.peers = append(q.owned.peers, peer)
		q.owned.records = append(q.owned.records, r)
	}
	if q.owned.records[1].ParentPID != q.owned.records[0].PID || q.owned.records[2].ParentPID != q.owned.records[1].PID {
		return fmt.Errorf("captured tree identities: %+v", q.owned.records)
	}
	return nil
}
func (q *mutationExecution) waitOwned(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := q.owned.run.Wait(bounded)
	if err != nil {
		return err
	}
	q.m.w.exit = r.Exit
	q.m.w.stdout = r.Stdout
	q.m.w.stderr = r.Stderr
	if err = json.Unmarshal([]byte(r.Stdout), &q.result); err != nil {
		return fmt.Errorf("owned returned no JSON: %w: %s", err, q.m.w.report())
	}
	q.owned.returned = true
	q.owned.joinedAt = time.Now()
	return nil
}
func (q *mutationExecution) timeoutReturn(ctx context.Context) error {
	q.owned.released = time.Now()
	return q.waitOwned(ctx)
}
func (q *mutationExecution) successReturn(ctx context.Context) error {
	q.owned.released = time.Now()
	if err := q.writeControl("success", true, true, ""); err != nil {
		return err
	}
	return q.waitOwned(ctx)
}
func (q *mutationExecution) ownedEnded(ctx context.Context) error {
	if q.owned == nil || !q.owned.returned || len(q.owned.peers) != 3 {
		return fmt.Errorf("owned command has not returned with three captured peers")
	}
	deadline := time.Now().Add(5 * time.Second)
	shared := q.owned.mode == "cancel" || q.owned.mode == "failfast"
	if shared {
		deadline = q.owned.released.Add(5 * time.Second)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for _, peer := range q.owned.peers {
		if err := peer.EOF(bounded); err != nil {
			return fmt.Errorf("owned process still active after return: %w", err)
		}
	}
	if shared && time.Now().After(deadline) {
		return fmt.Errorf("owned EOF exceeded release-relative shared bound")
	}
	return nil
}
func (q *mutationExecution) ownedRemoved(ctx context.Context) error {
	if err := q.ownedEnded(ctx); err != nil {
		return err
	}
	if time.Since(q.owned.released) > 10*time.Second {
		return fmt.Errorf("owned cleanup exceeded bounded return")
	}
	for _, r := range q.owned.records {
		if q.m.w.d.Exists(r.Cwd) {
			return fmt.Errorf("worker copy remains after joined return: %s", r.Cwd)
		}
	}
	original, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "main.go"))
	if err != nil {
		return err
	}
	if string(original) != q.source["main.go"] {
		return fmt.Errorf("original source changed during owned execution")
	}
	return nil
}
func (q *mutationExecution) timeoutOutcome() error {
	mu, err := executionMutant(q.result, "main.go", "Compare")
	if err != nil {
		return err
	}
	file, err := executionFile(q.result, "main.go")
	if err != nil {
		return err
	}
	if q.m.w.exit != 0 || file["baseline"] != "passed" || mu["outcome"] != "timeout" || file["killed"] != float64(1) {
		return fmt.Errorf("owned timeout judgment: %v", q.result)
	}
	return nil
}
func (q *mutationExecution) closedLive(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if len(q.owned.records) != 3 {
		return fmt.Errorf("closed-stream tree not captured")
	}
	for i, r := range q.owned.records {
		if !r.Closed {
			return fmt.Errorf("fixture streams were not closed: %+v", r)
		}
		if err := q.owned.peers[i].Ping(bounded); err != nil {
			return err
		}
	}
	return nil
}
func (q *mutationExecution) successOutcome() error {
	f, err := executionFile(q.result, "main.go")
	if err != nil {
		return err
	}
	mu, err := executionMutant(q.result, "main.go", "Compare")
	if err != nil {
		return err
	}
	if q.m.w.exit != 1 || f["baseline"] != "passed" || mu["outcome"] != "survived" {
		return fmt.Errorf("successful owned command judgment: %v", q.result)
	}
	return nil
}
func (q *mutationExecution) cancelParent(ctx context.Context) error {
	ready, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, log := q.owned.run.Output()
		if strings.Contains(log, "[1/2]") {
			break
		}
		select {
		case <-ready.Done():
			return fmt.Errorf("first judgment public progress missing: %w", ready.Err())
		case <-ticker.C:
		}
	}
	q.owned.released = time.Now()
	if err := q.owned.run.Interrupt(ctx); err != nil {
		return err
	}
	bounded, stop := context.WithDeadline(ctx, q.owned.released.Add(5*time.Second))
	defer stop()
	return q.waitOwned(bounded)
}
func (q *mutationExecution) cancelOutcome() error {
	mu, err := countedExecution(q.result, q.owned.block)
	if err != nil {
		return err
	}
	if q.m.w.exit != 75 || executionMap(q.result, "sampling")["completion"] != "interrupted" || mu["outcome"] != nil || !slices.Contains([]any{"cancelled", "undecided"}, mu["state"]) {
		return fmt.Errorf("parent cancellation judgment: %v", q.result)
	}
	ps := checkProblems(q.result)
	if len(ps) != 1 || ps[0]["rule"] != "count.interrupted" {
		return fmt.Errorf("parent cancellation problem: %v", q.result)
	}
	sampling := executionMap(q.result, "sampling")
	if sampling["selected"] != float64(2) || sampling["executed"] != float64(1) {
		return fmt.Errorf("interrupted admitted/executed budget: %v", sampling)
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	var actual []string
	for _, line := range strings.Split(strings.TrimSpace(string(trace)), "\n") {
		var r map[string]string
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			return err
		}
		if r["function"] != "baseline" {
			actual = append(actual, r["function"])
		}
	}
	if !slices.Equal(actual, q.owned.order) {
		return fmt.Errorf("interrupted command admission/order changed: %v want%v", actual, q.owned.order)
	}
	return nil
}
func (q *mutationExecution) completedFirst() error {
	mu, err := countedExecution(q.result, q.owned.order[0])
	if err != nil {
		return err
	}
	if mu["outcome"] != "killed" || mu["state"] != "judged" {
		return fmt.Errorf("completed before cancellation: %v", mu)
	}
	return nil
}
func (q *mutationExecution) unrelated(ctx context.Context) error {
	// A second product runs the same honest Go helper in its clean baseline.
	dir, err := q.m.w.d.Folder()
	if err != nil {
		return err
	}
	q.extraDirs = append(q.extraDirs, dir)
	w := &world{d: q.m.w.d, dir: dir}
	other := &mutationExecution{m: &mutateState{w: w}, command: "go test -count=1 -failfast .", peers: q.peers}
	if err = other.installOwned("control", false, false); err != nil {
		return err
	}
	q.extraDirs = append(q.extraDirs, other.outside)
	if err = other.declareOwned("control", false, ""); err != nil {
		return err
	}
	if err = q.m.w.d.Coverage(dir, Coverage{Language: "go", Report: "mode: set\nexample.com/ownership/main.go:3.28,3.42 1 1\n"}); err != nil {
		return err
	}
	// Hold real counted preparation so SIGINT owns and joins intermediary
	// Go/shell processes as well as the fixture peers.
	args := []string{"mutation", "run", "--json", "--no-annotate", "--workers", "1", "--count", "1", "--seed", "T80-unrelated-control"}
	p, err := other.startOwned(ctx, args...)
	if err != nil {
		return err
	}
	q.handles = append(q.handles, p)
	q.control = p
	ready, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, role := range []string{"command", "child", "grandchild"} {
		peer, err := q.peers.Capture(ready, filepath.Join(other.outside, "baseline-"+role+".json"))
		if err != nil {
			return err
		}
		q.owned.unrelated = append(q.owned.unrelated, peer)
	}
	return nil
}
func (q *mutationExecution) unrelatedLive(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if len(q.owned.unrelated) != 3 {
		return fmt.Errorf("unrelated product tree not captured")
	}
	for _, p := range q.owned.unrelated {
		if err := p.Ping(bounded); err != nil {
			return fmt.Errorf("unrelated process changed: %w", err)
		}
	}
	return nil
}
