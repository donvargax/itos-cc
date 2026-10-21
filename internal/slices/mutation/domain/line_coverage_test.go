package domain

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// A function's lines are those of a proven inventory from its body to its
// end: its def line, run at import, is not its own.
func TestLineCoverageEvidenceTakesTheFunctionsBodyLines(t *testing.T) {
	f := parse(t, "calc.py", "def reached(i, strict):\n    if strict:\n        raise ValueError(\"strict\")\n    return i > 5\n\n\ndef dormant(name):\n    label = name.strip()\n    return label\n")
	defer f.Close()
	lines := []coverage.Line{{Line: 1, Covered: true}, {Line: 2, Covered: true}, {Line: 3}, {Line: 4, Covered: true}, {Line: 7, Covered: true}, {Line: 8}, {Line: 9}}
	got := map[string][]int{}
	for _, unit := range f.Units {
		e := LineCoverageEvidence(f.Path, f, unit, lines, true, "p", nil)
		if !e.Complete || e.Language != "python" || e.Version != LineCoverageEvidenceVersion {
			t.Errorf("%s evidence %+v, want complete Python line evidence", unit.Name, e)
		}
		for _, b := range e.Blocks {
			if !b.Covered {
				got[unit.Name] = append(got[unit.Name], b.Line)
			}
		}
		if n := len(e.Blocks); unit.Name == "reached" && n != 3 || unit.Name == "dormant" && n != 2 {
			t.Errorf("%s has %d lines %+v, want its body's", unit.Name, n, e.Blocks)
		}
		if unproven := LineCoverageEvidence(f.Path, f, unit, lines, false, "p", nil); unproven.Complete || len(unproven.Blocks) != 0 {
			t.Errorf("%s evidence without proof %+v, want incomplete and empty", unit.Name, unproven)
		}
	}
	if want := map[string][]int{"reached": {3}, "dormant": {8, 9}}; !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered lines %v, want %v", got, want)
	}
}

// TypeScript units take their lines from their body's first statement: an
// arrow function bound to a name runs its declaration line at import, as
// Vitest's v8 provider reports it, and a function declaration's own line is
// no line v8 names at all.
func TestTypeScriptLineEvidenceLeavesTheDeclarationLineOut(t *testing.T) {
	f := parse(t, "calc.ts", "export function dormant(name: string): string {\n  const label = name.trim();\n  return label;\n}\n\n"+
		"export const arrow = (n: number): number => {\n  return n * 2;\n};\n\nexport class Box {\n  open(x: number): number {\n    return x;\n  }\n}\n")
	defer f.Close()
	// The lines Vitest 5's v8 LCOV names for this file when a test imports
	// it and calls nothing: the arrow's declaration line ran.
	lines := []coverage.Line{{Line: 2}, {Line: 3}, {Line: 6, Covered: true}, {Line: 7}, {Line: 12}}
	got := map[string][]int{}
	for _, unit := range f.Units {
		for _, b := range LineCoverageEvidence(f.Path, f, unit, lines, true, "p", nil).Blocks {
			if b.Covered {
				t.Errorf("%s holds the covered line %d", unit.Name, b.Line)
			}
			got[unit.Name] = append(got[unit.Name], b.Line)
		}
	}
	if want := map[string][]int{"dormant": {2, 3}, "arrow": {7}, "open": {12}}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
}

// Kotlin units take their lines from their body's first statement: JaCoCo
// puts a default-argument bridge on the declaration line, which runs only
// for a call that omits the argument, and a data class's generated methods
// on its own declaration line, which belongs to no function. An empty or
// comment-only body, or none, has nothing to prove, even unmeasured; a
// one-line expression body shares its declaration's line.
func TestKotlinLineEvidenceStartsAtTheBodysFirstStatement(t *testing.T) {
	f := parse(t, "Calc.kt", "package calc\n\n"+
		"fun reached(i: Int, label: String = \"x\"): Boolean {\n    // why\n    return i > 5\n}\n\n"+ // 3-6
		"fun twice(i: Int) = i * 2\n\n"+ // 8
		"fun split(i: Int) =\n    if (i > 0) 1\n    else 2\n\n"+ // 10-12
		"fun empty() {\n}\n\n"+ // 14-15
		"fun quiet() {\n    // nothing\n}\n\n"+ // 17-19
		"data class Span(val from: Int, val to: Int)\n\n"+ // 21
		"interface Shape {\n    fun area(): Int\n}\n") // 23-25
	defer f.Close()
	// The lines JaCoCo names: the bridge on 3, missed, the data class's
	// accessors on 21, missed, and the bodies.
	lines := []coverage.Line{{Line: 3}, {Line: 5, Covered: true}, {Line: 8}, {Line: 11, Covered: true}, {Line: 12},
		{Line: 15}, {Line: 19}, {Line: 21}}
	got := map[string][]int{}
	for _, unit := range f.Units {
		evidence := LineCoverageEvidence(f.Path, f, unit, lines, true, "p", nil)
		if !evidence.Complete {
			t.Errorf("%s's proven evidence is incomplete", unit.Name)
		}
		for _, b := range evidence.Blocks {
			got[unit.Name] = append(got[unit.Name], b.Line)
		}
	}
	if want := map[string][]int{"reached": {5}, "twice": {8}, "split": {11, 12}}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
	for _, unit := range f.Units {
		evidence := LineCoverageEvidence(f.Path, f, unit, nil, false, "p", nil)
		bodiless := unit.Name == "empty" || unit.Name == "quiet" || unit.Name == "area"
		if evidence.Complete != bodiless || len(evidence.Blocks) != 0 {
			t.Errorf("%s unmeasured: complete %v with %d lines, want complete %v with none", unit.Name, evidence.Complete, len(evidence.Blocks), bodiless)
		}
	}
}

// Kotlin evidence names the build tool that measured it and its scope, and
// mutation check admits each.
func TestKotlinCoverageProducersAreAdmitted(t *testing.T) {
	seen := map[string]bool{}
	for _, runner := range []string{"jacoco", "kover", "maven"} {
		for _, all := range []bool{false, true} {
			producer := KotlinCoverageProducer(runner, all)
			if producer == "" || seen[producer] || !ValidCoverageProducer("kotlin", producer) {
				t.Errorf("%s all-tests %v: producer %q, want a distinct admitted one", runner, all, producer)
			}
			seen[producer] = true
			if ValidCoverageProducer("typescript", producer) || ValidCoverageProducer("python", producer) {
				t.Errorf("%q is admitted for another language", producer)
			}
		}
	}
	if got := KotlinCoverageProducer("maven", false); !strings.Contains(got, "jacoco:report") || !strings.Contains(got, "scope=own") {
		t.Errorf("Maven's own producer %q, want its JaCoCo report over the reaching classes", got)
	}
	if KotlinCoverageProducer("script", false) != "" || ValidCoverageProducer("kotlin", TypeScriptCoverageProducer("vitest", false)) {
		t.Error("a runner that is not Kotlin's produces Kotlin evidence")
	}
}
