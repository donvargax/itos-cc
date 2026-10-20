package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// unrecordedSteps hold a run since a commit over a file whose snapshot holds
// no entry for its first function (slice-12, issue #40): the other two
// functions' whole-suite outcomes are recorded by a run, and the first
// one's entry is then taken out of the raw snapshot JSON, as an older
// itos-cc's snapshot left it.
func (b *broadLanguage) unrecordedSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go file with three functions, its snapshot holding whole-suite outcomes for the second and third and no entry for the first$`, b.unrecordedProject)
	sc.Step(`^a non-importing module test changed since "base"$`, func() error { return b.l.appendComment("e2e/support_test.go") })
	sc.Step(`^only the (second|third) function changed since "base"$`, func(which string) error { return b.changeNamed(ordinalFunction(which)) })
	sc.Step(`^the exit code is not 70 and the output names no panic$`, b.noPanic)
	sc.Step(`^the (second|third) function's outcomes are rerun$`, func(which string) error { return b.executed(ordinalFunction(which), "killed") })
	sc.Step(`^mutation check reports the (second|third) function stale and the (second|third) fresh$`, func(stale, fresh string) error {
		a, _, _, err := b.l.capture("mutation check --json " + b.l.file)
		if err != nil {
			return err
		}
		if err := b.stale(ordinalFunction(stale), "", a); err != nil {
			return err
		}
		return b.expectStates(map[string]string{ordinalFunction(fresh): "fresh"}, a)
	})
	sc.Step(`^the snapshot still holds no entry for the first function$`, b.firstUnrecorded)
}

func ordinalFunction(which string) string {
	return map[string]string{"first": "First", "second": "Second", "third": "Third"}[which]
}

func (b *broadLanguage) unrecordedProject() error {
	l, w := b.l, b.l.m.w
	l.file = "main.go"
	l.language = "go"
	source := "package main\n\nfunc First() bool { return true }\nfunc Second() bool { return false }\nfunc Third() bool { return true }\n"
	test := "package main\nimport \"testing\"\nfunc TestFirst(t *testing.T) { if !First() { t.Fatal(\"First\") } }\nfunc TestSecond(t *testing.T) { if Second() { t.Fatal(\"Second\") } }\nfunc TestThird(t *testing.T) { if !Third() { t.Fatal(\"Third\") } }\n"
	for file, text := range map[string]string{"go.mod": "module example.com/language\n\ngo 1.22\n", l.file: source, "main_test.go": test, "e2e/support_test.go": "package e2e\nimport \"testing\"\nfunc TestSupport(t *testing.T) {}\n"} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	// The file's own tests, its package's, beside the whole suite's.
	for _, command := range []string{"go test -count=1 -failfast ./.", "go test -count=1 -failfast ./...", "go test -count=1 ./..."} {
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: command}); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := l.s.commit("base fixture"); err != nil {
		return err
	}
	if err := w.git("tag", "base"); err != nil {
		return err
	}
	if err := b.record("--all-tests"); err != nil {
		return err
	}
	snap := b.before
	var kept []any
	for _, u := range objectList(snap, "units") {
		if u["name"] != "First" {
			kept = append(kept, u)
		}
	}
	if len(kept) != 2 {
		return fmt.Errorf("recorded snapshot lacks the three functions: %v", snap)
	}
	snap["units"] = kept
	if err := l.writeSnapshot(l.file, snap); err != nil {
		return err
	}
	var err error
	b.before, err = l.s.snapshot(l.file)
	return err
}

func (b *broadLanguage) changeNamed(name string) error {
	l := b.l
	old := l.m.read(l.file)
	next := old
	for _, value := range []string{"true", "false"} {
		next = strings.Replace(next, "func "+name+"() bool { return "+value+" }", "func "+name+"() bool { return ("+value+") }", 1)
	}
	if next == old {
		return fmt.Errorf("no function %s to change in %s", name, l.file)
	}
	if err := l.m.w.write(l.file, next); err != nil {
		return err
	}
	return l.s.commit("one function and module test changed")
}

func (b *broadLanguage) noPanic() error {
	w := b.l.m.w
	if w.last == nil {
		return fmt.Errorf("no run of itos-cc to read")
	}
	if w.exit == 70 || strings.Contains(strings.ToLower(w.stdout+w.stderr), "panic") {
		return fmt.Errorf("the run panicked, exit %d\n%s", w.exit, w.report())
	}
	return nil
}

func (b *broadLanguage) firstUnrecorded() error {
	snap, err := b.l.s.snapshot(b.l.file)
	if err != nil {
		return err
	}
	for _, u := range objectList(snap, "units") {
		if u["name"] == "First" {
			return fmt.Errorf("the snapshot gained an entry for First: %v", u)
		}
	}
	if len(objectList(snap, "units")) != 2 {
		return fmt.Errorf("the snapshot lost a recorded function: %v", snap)
	}
	return nil
}
