package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (q *countedMutation) budgetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^several committed files and functions have eligible sites but no mutation cache$`, q.budgetProject)
	sc.Step(`^the worker count exceeds the count-one budget$`, func() error { q.workers = 4; return nil })
	sc.Step(`^a counted run successfully judges its selected covered mutant$`, q.budgetRun)
	sc.Step(`^exactly one distinct mutation trial executes across the entire selection$`, q.oneGlobalTrial)
	sc.Step(`^omitted sites and subjects are reported rather than executed$`, q.omittedPopulation)
	sc.Step(`^no full mutation prepass prepares the selection$`, q.noPrepass)
	sc.Step(`^the same fresh discovery admits changed functions with missing or stale cache entries$`, q.changedDiscovery)
	sc.Step(`^a matching prior cached kill never substitutes for the fresh trial$`, q.freshDespiteCache)
	sc.Step(`^a counted selection omits some sites and may stop before all selected work completes$`, q.budgetProject)
	sc.Step(`^its report is emitted$`, q.budgetRun)
	sc.Step(`^JSON is one object with budget, eligible, selected, executed and omitted counts$`, q.outputCounts)
	sc.Step(`^it lists selected identities and actual outcomes with commit, seed and algorithm provenance$`, q.outputIdentities)
	sc.Step(`^judged and omitted subjects and real preparation or baseline stages are explicit$`, q.outputStages)
	sc.Step(`^cancelled, blocked and unattempted selected work has no fabricated mutation outcome$`, q.outputUnfinished)
	sc.Step(`^a sampled pass is described as assurance about sampled judgments only$`, q.sampledAssurance)
	sc.Step(`^count does not claim to bound discovery, baseline commands or total latency$`, q.countBounds)
}
func (q *countedMutation) budgetProject() error {
	if err := q.multiProject(); err != nil {
		return err
	}
	if q.e.m.w.d.Exists(filepath.Join(q.e.m.w.dir, ".metrics", "mutate")) {
		return fmt.Errorf("budget fixture has mutation cache")
	}
	sites, err := q.observe("mutation", "list", "--json")
	if err != nil {
		return err
	}
	if len(objectList(sites.object, "sites")) != 4 {
		return fmt.Errorf("budget public site population: %v", sites.object)
	}
	q.saved["budget-sites"] = sites
	// Public bounded setup queries choose a main.go site, whose actual source
	// digest the fixture independently observes. No ranking internals are used.
	for i := 0; i < 32; i++ {
		seed := fmt.Sprintf("T81-budget-%d", i)
		o, err := q.count("--count", "1", "--workers", "1", "--seed", seed)
		if err != nil {
			return err
		}
		s, err := countedSelected(o)
		if err != nil {
			return err
		}
		if o.result.Exit != 0 {
			return fmt.Errorf("budget public setup query: %v", o.object)
		}
		if s["file"] == "main.go" {
			q.seed = seed
			return nil
		}
	}
	return fmt.Errorf("public budget queries found no main source selection")
}
func (q *countedMutation) budgetRun() error {
	var err error
	workers := q.workers
	if workers == 0 {
		workers = 4
	}
	q.primary, err = q.count("--count", "1", "--seed", q.seed, "--workers", strconv.Itoa(workers))
	return err
}
func (q *countedMutation) oneGlobalTrial() error {
	o := q.primary
	s, err := countedSelected(o)
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || s["state"] != "judged" || s["outcome"] != "killed" || s["file"] != "main.go" || countedSampling(o)["executed"] != float64(1) || strings.Count(o.result.Stderr, "itos-cc: [") != 1 {
		return fmt.Errorf("one global fresh judgment: %v\n%s", o.object, o.result.Stderr)
	}
	clean := countedSHA(countedMultiSource)
	mutantSHA := ""
	source := countedMultiSource
	column, ok := s["column"].(float64)
	if !ok {
		return fmt.Errorf("selected column missing: %v", s)
	}
	lines := strings.Split(source, "\n")
	number, ok := s["line"].(float64)
	if !ok || number < 1 || int(number) > len(lines) {
		return fmt.Errorf("selected source line missing: %v", s)
	}
	line := lines[int(number)-1]
	at := int(column) - 1
	old := fmt.Sprint(s["original"])
	if at < 0 || at+len(old) > len(line) || line[at:at+len(old)] != old {
		return fmt.Errorf("selected public site not in fixture: %v", s)
	}
	lines[int(number)-1] = line[:at] + fmt.Sprint(s["replacement"]) + line[at+len(old):]
	mutantSHA = countedSHA(strings.Join(lines, "\n"))
	baseline, trials := 0, 0
	for _, r := range o.traces {
		if r["coverage"] == true {
			if r["source_sha256"] != clean {
				return fmt.Errorf("budget measured mutated source: %v", r)
			}
			continue
		}
		switch r["source_sha256"] {
		case clean:
			baseline++
		case mutantSHA:
			trials++
		default:
			return fmt.Errorf("unselected actual source executed: %v", r)
		}
	}
	if baseline != 1 || trials != 1 {
		return fmt.Errorf("global actual baseline/trial population %d/%d: %v", baseline, trials, o.traces)
	}
	return nil
}
func (q *countedMutation) omittedPopulation() error {
	s := countedSampling(q.primary)
	subjects := executionMap(q.primary.object, "subjects")
	judged := objectList(subjects, "judged")
	omitted := objectList(subjects, "omitted")
	if s["eligible"] != float64(4) || s["selected"] != float64(1) || s["omitted"] != float64(3) || len(judged) != 1 || len(omitted) != 1 || judged[0]["file"] != "main.go" || omitted[0]["file"] != "other.go" {
		return fmt.Errorf("global omitted sites/subjects: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) noPrepass() error {
	if len(q.primary.traces) != 3 {
		return fmt.Errorf("unexpected full mutation prepass execution population: %v", q.primary.traces)
	}
	coverage := 0
	for _, record := range q.primary.traces {
		if record["coverage"] == true {
			coverage++
		}
	}
	if coverage != 1 {
		return fmt.Errorf("fresh preparation coverage command population: %d", coverage)
	}
	return nil
}
func (q *countedMutation) changedDiscovery() error {
	w := q.e.m.w
	complete, err := q.observe("mutation", "run", "--json", "--workers", "1")
	if err != nil {
		return err
	}
	if complete.result.Exit != 0 {
		return fmt.Errorf("prior complete cache setup: %v", complete.object)
	}
	// A changed existing function and a newly committed function both enter the
	// actual fresh population. Keep the recorded cache to exercise stale/missing.
	if err = w.write("main.go", countedMultiSource+"\nfunc New(n int) bool { return n > 2 }\n"); err != nil {
		return err
	}
	if err = w.write("other.go", "package main\n\nfunc Other(n int) bool { /* changed */ return n > 5 }\n"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "add", "main.go", "other.go"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "new and changed counted functions"); err != nil {
		return err
	}
	report := countedMultiProfile + "example.com/ownership/main.go:5.24,5.38 1 0\nexample.com/ownership/other.go:3.40,3.54 1 1\n"
	if err = q.measureProfile(report); err != nil {
		return err
	}
	o, err := q.count("--count", "5", "--seed", q.seed, "--workers", "1")
	if err != nil {
		return err
	}
	foundNew, foundChanged := false, false
	for _, s := range objectList(o.object, "selected") {
		if strictNamed(fmt.Sprint(s["function"]), "New") && s["outcome"] == "uncovered" {
			foundNew = true
		}
		if strictNamed(fmt.Sprint(s["function"]), "Other") && s["outcome"] == "killed" {
			foundChanged = true
		}
	}
	if o.result.Exit != 0 || countedSampling(o)["eligible"] != float64(5) || !foundNew || !foundChanged {
		return fmt.Errorf("missing/stale cache fresh discovery: %v", o.object)
	}
	q.saved["changed"] = o
	return nil
}
func (q *countedMutation) freshDespiteCache() error {
	// Compare's hash and tests did not change. The actual old complete cache
	// remains present, yet a new count-one command must execute its selected site.
	w := q.e.m.w
	b, err := w.d.Read(filepath.Join(w.dir, ".metrics", "mutate", "main.go.json"))
	if err != nil {
		return err
	}
	var snapshot map[string]any
	if err = json.Unmarshal(b, &snapshot); err != nil {
		return err
	}
	cached := []map[string]any{}
	for _, unit := range objectList(snapshot, "units") {
		if unit["name"] == "Compare" {
			cached = append(cached, unit)
		}
	}
	if len(cached) != 1 || len(objectList(cached[0], "mutants")) != 3 || cached[0]["hash"] == "" || cached[0]["hash"] == nil {
		return fmt.Errorf("matching complete Compare cache absent: %v", snapshot)
	}
	for _, mutant := range objectList(cached[0], "mutants") {
		if mutant["outcome"] != "killed" {
			return fmt.Errorf("matching prior cached outcome was not killed: %v", mutant)
		}
	}
	check, err := q.observe("mutation", "check", "--json", "main.go")
	if err != nil {
		return err
	}
	file, err := executionFile(check.object, "main.go")
	if err != nil {
		return err
	}
	fresh := 0
	for _, fn := range objectList(file, "functions") {
		if strictNamed(fmt.Sprint(fn["function"]), "Compare") && fn["state"] == "fresh" && fn["killed"] == float64(3) {
			fresh++
		}
	}
	if fresh != 1 || len(check.traces) != 0 {
		return fmt.Errorf("prior cached Compare does not match current source/tests: %v", check.object)
	}
	if err = strictNoCommands(check.result.Stderr); err != nil {
		return err
	}

	o, err := q.count("--count", "5", "--seed", q.seed, "--workers", "1")
	if err != nil {
		return err
	}
	// The whole current public population includes all three unchanged Compare sites.
	if o.result.Exit != 0 {
		return fmt.Errorf("cached kill fresh run: %v", o.object)
	}
	if countedSampling(o)["executed"] != float64(4) {
		return fmt.Errorf("matching cache substituted for fresh trials: %v", o.object)
	}
	original := countedMultiSource + "\nfunc New(n int) bool { return n > 2 }\n"
	wanted := map[string]bool{}
	for _, site := range objectList(o.object, "selected") {
		if !strictNamed(fmt.Sprint(site["function"]), "Compare") {
			continue
		}
		lines := strings.Split(original, "\n")
		line := int(site["line"].(float64)) - 1
		column := int(site["column"].(float64)) - 1
		old := fmt.Sprint(site["original"])
		row := lines[line]
		if row[column:column+len(old)] != old {
			return fmt.Errorf("cached Compare site differs from public fixture: %v", site)
		}
		lines[line] = row[:column] + fmt.Sprint(site["replacement"]) + row[column+len(old):]
		wanted[countedSHA(strings.Join(lines, "\n"))] = false
	}
	for _, record := range o.traces {
		if record["coverage"] == true {
			continue
		}
		sha := fmt.Sprint(record["source_sha256"])
		if _, ok := wanted[sha]; ok {
			if wanted[sha] {
				return fmt.Errorf("cached Compare trial repeated: %v", record)
			}
			wanted[sha] = true
		}
	}
	if len(wanted) != 3 {
		return fmt.Errorf("cached Compare fresh site population: %v", wanted)
	}
	for sha, seen := range wanted {
		if !seen {
			return fmt.Errorf("cached Compare trial was not actually executed: %s", sha)
		}
	}
	return nil
}
func (q *countedMutation) outputCounts() error {
	s := countedSampling(q.primary)
	if s["budget"] != float64(1) || s["eligible"] != float64(4) || s["selected"] != float64(1) || s["executed"] != float64(1) || s["omitted"] != float64(3) {
		return fmt.Errorf("machine count population: %v", s)
	}
	return nil
}
func (q *countedMutation) outputIdentities(ctx context.Context) error {
	head, err := q.e.m.w.d.Head(ctx, q.e.m.w.dir)
	if err != nil {
		return err
	}
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	p := countedSampling(q.primary)
	matches := 0
	for _, site := range objectList(q.saved["budget-sites"].object, "sites") {
		same := true
		for _, key := range []string{"file", "function", "line", "column", "original", "replacement"} {
			same = same && site[key] == s[key]
		}
		if same {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("machine selected site lacks exact public identity: %v", s)
	}
	if p["commit"] != head || p["seed"] != q.seed || p["algorithm"] != "fresh-plan-v2" || s["identity"] == "" || s["identity"] == nil || s["outcome"] != "killed" {
		return fmt.Errorf("machine selected provenance: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) outputStages() error {
	if err := q.omittedPopulation(); err != nil {
		return err
	}
	for _, name := range []string{"coverage", "baseline", "preparation", "trials"} {
		s, err := countedStage(q.primary, name)
		if err != nil {
			return err
		}
		if s["state"] != "complete" {
			return fmt.Errorf("machine actual stage %s: %v", name, s)
		}
	}
	return nil
}
func (q *countedMutation) outputUnfinished(ctx context.Context) error {
	other, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	if err = other.setMode("baseline-fail"); err != nil {
		return err
	}
	o, err := other.count("--count", "1")
	if err != nil {
		return err
	}
	selected := objectList(o.object, "selected")
	if o.result.Exit != 1 || len(selected) != 1 || selected[0]["outcome"] != nil || selected[0]["state"] != "unattempted" || countedSampling(o)["executed"] != float64(0) {
		return fmt.Errorf("unfinished selected outcome fabricated: %v", o.object)
	}
	q.saved["unfinished"] = o
	listed, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	if err = listed.listedProject(); err != nil {
		return err
	}
	if err = listed.listedBaselineFails(); err != nil {
		return err
	}
	if err = listed.blockedNoOutcome(); err != nil {
		return err
	}
	if err = listed.baselineProblem(); err != nil {
		return err
	}
	q.saved["blocked"] = listed.primary
	interrupted, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	q.children = append(q.children, interrupted)
	if err = interrupted.interruptProject(ctx); err != nil {
		return err
	}
	if err = interrupted.interruptRun(ctx); err != nil {
		return err
	}
	if err = interrupted.e.cancelOutcome(); err != nil {
		return err
	}
	q.saved["cancelled"] = interrupted.primary
	return nil
}
func (q *countedMutation) sampledAssurance() error {
	if countedSampling(q.primary)["assurance"] != "sampled" {
		return fmt.Errorf("machine sampled assurance: %v", q.primary.object)
	}
	w := q.e.m.w
	code, out, log, err := w.capture(w.dir, w.env(), "mutation", "run", "--count", "1", "--seed", q.seed, "--workers", "4")
	if err != nil {
		return err
	}
	if code != 0 || !strings.Contains(out, "sampled") || !strings.Contains(out, "not a complete") || !strings.Contains(out, "bounds mutant trials, not discovery") {
		return fmt.Errorf("plain sampled assurance: %s%s", out, log)
	}
	q.texts["sampled"] = Result{Exit: code, Stdout: out, Stderr: log}
	return nil
}
func (q *countedMutation) countBounds() error {
	bounds := fmt.Sprint(countedSampling(q.primary)["bounds"])
	for _, term := range []string{"mutant trials", "not discovery", "coverage", "listing", "baseline commands", "total latency"} {
		if !strings.Contains(bounds, term) {
			return fmt.Errorf("count execution bounds omit %q: %s", term, bounds)
		}
	}
	return nil
}
