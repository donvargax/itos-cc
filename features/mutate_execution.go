// Fail-fast and ownership steps observe only public commands and owned fixtures.
package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type mutationExecution struct {
	m              *mutateState
	s              *sinceMutation
	id             string
	outside        string
	result         map[string]any
	before         map[string]string
	source         map[string]string
	command        string
	args           string
	handles        []Running
	control        Running
	peers          FixturePeers
	owned          *ownershipExecution
	extraDirs      []string
	observations   map[string]executionObservation
	twin           *mutationExecution
	listedSelected []string
}

func executionMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	q := &mutationExecution{m: m, s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*q = mutationExecution{m: m, s: s, command: "go test -count=1 ."}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				q.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if slices.Contains([]string{"165", "166", "167", "187", "188", "189", "191", "192", "193", "194", "195", "196", "197", "198", "199", "200", "201", "202", "211", "248"}, q.id) {
			m.w.beforeRemove = q.cleanup
		}
		return ctx, nil
	})

	q.ownershipSteps(sc)
	q.policySteps(sc)
	q.partialSteps(sc)
	q.planningSteps(sc)
	sc.Step(`^a run with several selected mutants and one worker$`, q.three)
	sc.Step(`^the first judged mutant survives its own tests and has no applicable listed tests or valid exception$`, func() error { return q.declare("Weak") })
	sc.Step(`^mutation run --fail-fast judges them$`, q.failFast)
	sc.Step(`^it fails with mutation\.survived for that mutant$`, q.firstSurvivor)
	sc.Step(`^no later mutant is started, and each is reported unattempted with no outcome$`, q.laterUnattempted)
	sc.Step(`^without --fail-fast every mutant is judged and the survivor is reported among them$`, q.aggregate)
	sc.Step(`^the complete and counted mutation run commands$`, q.admissionProject)
	sc.Step(`^--fail-fast is combined with --count$`, func() error { return q.run("mutation run --count 1 --fail-fast --json") })
	sc.Step(`^--count with --fail-fast is a flags\.conflict usage error$`, func(ctx context.Context) error {
		if err := q.admission("flags.conflict", 2, "--fail-fast"); err != nil {
			return err
		}
		return q.nothingLaunched(ctx)
	})
	sc.Step(`^the help describes --fail-fast, its rule and its platforms$`, q.help)
	sc.Step(`^a committed eligible mutation fixture on Windows$`, func() error {
		if m.w.d.Platform() != "windows" {
			return fmt.Errorf("%w: native Windows admission", godog.ErrSkip)
		}
		return q.admissionProject()
	})
	sc.Step(`^I run "itos-cc ([^"]*)" in that fixture$`, q.run)
	sc.Step(`^the problem is "([^"]*)" for Windows and the exit code is 3$`, func(rule string) error { return q.admission(rule, 3, "windows") })
	sc.Step(`^no test, list or coverage command runs and no snapshot is written$`, q.nothingLaunched)
}

func joinExecutionErrors(errs []error) error { return errors.Join(errs...) }

func (q *mutationExecution) project(files map[string]string) error {
	w := q.m.w
	if q.outside == "" {
		dir, err := w.d.Folder()
		if err != nil {
			return err
		}
		q.outside = dir
	}
	q.source = files
	for name, text := range files {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	if err := w.write("go.mod", "module example.com/ownership\n\ngo 1.22\n"); err != nil {
		return err
	}
	if err := w.write("main_test.go", executionTests); err != nil {
		return err
	}
	if err := w.d.Write(filepath.Join(q.outside, "control.json"), []byte(`{}`)); err != nil {
		return err
	}
	w.variables = []string{"T80_TRACE=" + filepath.Join(q.outside, "trace.jsonl"), "T80_ORIGINAL=" + w.dir}
	if err := w.d.Write(filepath.Join(q.outside, "trace.jsonl"), nil); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "init", "-q"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "go.mod", "main_test.go"); err != nil {
		return err
	}
	for file := range files {
		if err := w.d.Git(w.dir, "add", file); err != nil {
			return err
		}
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "fixture"); err != nil {
		return err
	}
	return q.declare("Weak")
}
func (q *mutationExecution) declare(survive ...string) error {
	t := Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.outside, "trace.jsonl"), Source: "main.go"}}
	for file, source := range q.source {
		for i, line := range strings.Split(source, "\n") {
			for _, name := range survive {
				if strings.Contains(line, "func "+name+"(") {
					t.Surviving = append(t.Surviving, Site{File: file, Line: i + 1, Original: ">", Replacement: ">="})
				}
			}
		}
	}
	return q.m.w.d.Tests(q.m.w.dir, t)
}
func (q *mutationExecution) three() error {
	if !CountedOn(q.m.w.d.Platform()) {
		return fmt.Errorf("%w: fail-fast uses Unix ownership", godog.ErrSkip)
	}
	return q.project(map[string]string{"main.go": executionThreeSource})
}
func (q *mutationExecution) run(args string) error {
	if err := q.m.w.runWith(q.m.w.env(), args); err != nil {
		return err
	}
	var a map[string]any
	if err := json.Unmarshal([]byte(q.m.w.stdout), &a); err != nil {
		return fmt.Errorf("execution JSON: %w: %s", err, q.m.w.report())
	}
	q.result = a
	return nil
}
func (q *mutationExecution) failFast() error {
	if q.id == "194" {
		return q.run("mutation run --fail-fast --json --no-annotate --workers 1 --timeout-factor 1 --since base")
	}
	return q.run("mutation run --fail-fast --json --no-coverage --no-annotate --workers 1 --test-command '" + q.command + "' " + q.args)
}
func executionFile(a map[string]any, file string) (map[string]any, error) {
	var matches []map[string]any
	for _, f := range objectList(a, "files") {
		if f["file"] == file {
			matches = append(matches, f)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("execution file %s: expected one, got %v", file, matches)
	}
	return matches[0], nil
}
func executionMutant(a map[string]any, file, name string) (map[string]any, error) {
	f, err := executionFile(a, file)
	if err != nil {
		return nil, err
	}
	var matches []map[string]any
	for _, mu := range objectList(f, "mutants") {
		if strictNamed(fmt.Sprint(mu["function"]), name) {
			matches = append(matches, mu)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("execution mutant %s#%s: expected one, got %v", file, name, matches)
	}
	return matches[0], nil
}
func (q *mutationExecution) firstSurvivor() error {
	mu, err := executionMutant(q.result, "main.go", "Weak")
	if err != nil {
		return err
	}
	stop := executionMap(q.result, "stop")
	subject := executionMap(stop, "subject")
	if q.m.w.exit != 1 || q.result["ok"] != false || stop["stopped"] != true || stop["rule"] != "mutation.survived" || subject["file"] != "main.go" || subject["function"] != mu["function"] || mu["state"] != "completed" || mu["outcome"] != "survived" || mu["excepted"] != nil {
		return fmt.Errorf("first actionable survivor: %v", q.result)
	}
	return executionSurvivorProblem(q.result, mu)
}
func (q *mutationExecution) laterUnattempted() error {
	for _, name := range []string{"Compare", "Other"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != "unattempted" || mu["outcome"] != nil {
			return fmt.Errorf("later unattempted %s: %v", name, mu)
		}
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(trace)), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("later admission: expected baseline and Weak trial, got %s", trace)
	}
	var original string
	for i, line := range lines {
		var r map[string]string
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			return err
		}
		source := q.source["main.go"]
		if i == 1 {
			source = strings.Replace(source, "n > 5", "n >= 5", 1)
		}
		hash := sha256.Sum256([]byte(source))
		rel, err := filepath.Rel(r["original_root"], r["cwd"])
		if r["original_root"] == "" || r["cwd"] == "" || err != nil || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return fmt.Errorf("admission trace not in worker: %v", r)
		}
		if i == 0 {
			original = r["original_root"]
		}
		if r["source_sha256"] != hex.EncodeToString(hash[:]) || r["original_root"] != original {
			return fmt.Errorf("admission trace differs from actual baseline/Weak trial: %v", r)
		}
	}

	return nil
}
func (q *mutationExecution) aggregate() error {
	if err := q.run("mutation run --json --no-coverage --no-annotate --workers 1 --mutate-all --test-command '" + q.command + "'"); err != nil {
		return err
	}
	if q.m.w.exit != 1 || q.result["stop"] != nil || q.result["work"] != nil {
		return fmt.Errorf("aggregate shape: %v", q.result)
	}
	ps := checkProblems(q.result)
	if len(ps) != 1 || ps[0]["rule"] != "mutation.survived" || !strictNamed(fmt.Sprint(ps[0]["function"]), "Weak") || ps[0]["file"] != "main.go" {
		return fmt.Errorf("aggregate survivor problem: %v", ps)
	}
	for _, name := range []string{"Weak", "Compare", "Other"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		want := "killed"
		if name == "Weak" {
			want = "survived"
		}
		if mu["outcome"] != want || mu["state"] != nil {
			return fmt.Errorf("aggregate judgment %s: %v", name, mu)
		}
	}
	return nil
}
func (q *mutationExecution) admission(rule string, code int, subject string) error {
	if err := executionAdmission(q.result, q.m.w.exit, rule, code, subject); err != nil {
		return err
	}
	return nil
}
func executionAdmission(a map[string]any, exit int, rule string, code int, subject string) error {
	ps := checkProblems(a)
	if exit != code || a["ok"] != false || len(ps) != 1 || ps[0]["rule"] != rule {
		return fmt.Errorf("admission result: exit=%d %v", exit, a)
	}
	key := "flag"
	if subject == "windows" {
		key = "platform"
	}
	if ps[0][key] != subject {
		return fmt.Errorf("admission subject %s=%s absent: %v", key, subject, ps[0])
	}

	return nil
}
func (q *mutationExecution) nothingLaunched(ctx context.Context) error {
	if err := strictNoCommands(q.m.w.stderr); err != nil {
		return err
	}
	files, err := q.m.w.d.Files(ctx, q.m.w.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasPrefix(file, ".metrics/mutate/") {
			return fmt.Errorf("admission wrote snapshot %s", file)
		}
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	if len(trace) != 0 {
		return fmt.Errorf("admission launched tests: %s", trace)
	}
	return q.noListedCommand()
}
func (q *mutationExecution) help() error {
	code, out, _, err := q.m.w.capture(q.m.w.dir, q.m.w.env(), "help", "mutation", "run")
	if err != nil {
		return err
	}
	for _, word := range []string{"--fail-fast", "fail-fast.platform", "Linux and macOS", "unattempted"} {
		if code != 0 || !strings.Contains(out, word) {
			return fmt.Errorf("admission help missing %q: exit=%d %s", word, code, out)
		}
	}
	if !strings.Contains(out, `"stop"`) && !strings.Contains(out, "`stop`") {
		return fmt.Errorf("admission help missing stop key: %s", out)
	}
	return executionCountedHelp(out, code)
}

func executionMap(a map[string]any, key string) map[string]any {
	m, _ := a[key].(map[string]any)
	return m
}

func (q *mutationExecution) cleanup(ctx context.Context) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	// Stop new fixture work before cooperatively draining remaining exact peers.
	var failures []error
	for _, p := range q.handles {
		failures = append(failures, p.Interrupt(cleanup))
	}
	// Counted cancellation joins its command tree. Observe those control peers
	// without sending a competing cooperative-stop message into closing sockets.
	if q.control != nil {
		_, err := q.control.Wait(cleanup)
		failures = append(failures, err)
		if err == nil && q.owned != nil {
			for _, peer := range q.owned.unrelated {
				failures = append(failures, peer.EOF(cleanup))
			}
		}
	}
	// A completed owned command has already joined its helpers. Consume their
	// strict EOF before cooperative Close can send STOP into closing sockets.
	// Logical counted controls need this cleanup even without a public EOF step.
	if q.owned != nil && q.owned.returned {
		for _, peer := range q.owned.peers {
			failures = append(failures, peer.EOF(cleanup))
		}
	}
	if q.peers != nil {
		failures = append(failures, q.peers.Close(cleanup))
	}
	for _, p := range q.handles {
		failures = append(failures, p.Close(cleanup))
	}
	if err := joinExecutionErrors(failures); err != nil {
		return err
	}
	for _, dir := range q.extraDirs {
		if err := q.m.w.d.Remove(dir); err != nil {
			return err
		}
	}
	if q.outside != "" {
		return q.m.w.d.Remove(q.outside)
	}
	return nil
}

func executionSurvivorProblem(a, mu map[string]any) error {
	if err := executionStop(a, "mutation.survived"); err != nil {
		return err
	}
	var found []map[string]any
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.survived" {
			found = append(found, p)
		}
	}
	if len(found) != 1 {
		return fmt.Errorf("survivor problem population: %v", found)
	}
	p := found[0]
	for _, key := range []string{"function", "line", "column", "original", "replacement"} {
		if p[key] != mu[key] {
			return fmt.Errorf("survivor problem %s mismatch: %v versus%v", key, p, mu)
		}
	}
	if p["file"] != "main.go" {
		return fmt.Errorf("survivor problem file mismatch: %v", p)
	}
	return nil
}
