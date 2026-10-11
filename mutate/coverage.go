package mutate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const goCoverageEvidenceVersion = 2

// lineCoverageEvidenceVersion is the version of the evidence of a
// line-precision language (CoverageEvidence.Language).
const lineCoverageEvidenceVersion = 1

// strictLanguages are the languages whose functions strict coverage
// (--fail-uncovered) proves executed: Go at cover-profile block precision,
// the others at their format's line precision (coverage.Report.Lines).
var strictLanguages = map[string]bool{"go": true, "python": true, "typescript": true, "kotlin": true}

// StrictCoverage says whether strict coverage proves the executable code of
// language's functions: whether its functions need fresh measured evidence.
func StrictCoverage(language string) bool {
	return strictLanguages[language]
}

// pythonSubprocesses is the part of Python's producer that says the
// Python processes a test starts count too: coverage.py measures them with
// the rcfile COVERAGE_PROCESS_START names, and their combined LCOV is read
// beside the in-process report (integration-coverage-python). Evidence
// recorded before, without it, reads stale once, so a line only a
// subprocess runs is measured again rather than kept uncovered.
const pythonSubprocesses = "subprocesses=COVERAGE_PROCESS_START, python -m coverage combine"

// CoverageProducer is the built-in producer of language's strict coverage
// evidence, measuring each file with the tests that reach it, or, with
// allTests, the whole suite of its build root: what evidence records, and
// what mutation check admits (validCoverageProducer).
func CoverageProducer(language string, allTests bool) string {
	switch {
	case language == "go" && allTests:
		return "go test -count=1 -covermode=set -coverpkg=./... -coverprofile=coverage.out ./...; scope=all-tests"
	case language == "go":
		return "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
	case language == "python" && allTests:
		return "python -m coverage run --branch --source=<build root> <whole suite>; python -m coverage lcov; " + pythonSubprocesses + "; scope=all-tests"
	case language == "python":
		return "python -m coverage run --branch --source=<build root> <reaching tests>; python -m coverage lcov; " + pythonSubprocesses + "; scope=own"
	}
	return ""
}

// TypeScriptCoverageProducer is the built-in producer of TypeScript's
// strict coverage evidence by runner (coverage.TypeScriptRunner): Vitest's
// v8 or Jest's LCOV over the related tests, or with allTests the whole
// suite, or c8's over the test script, which is the whole suite. A project's
// own coverage script ("script") is none: its report is unattested.
func TypeScriptCoverageProducer(runner string, allTests bool) string {
	scope := "own"
	if allTests {
		scope = "all-tests"
	}
	switch runner {
	case "vitest":
		if allTests {
			return "vitest run --coverage.enabled --coverage.reporter=lcov; scope=" + scope
		}
		return "vitest related --run <sources> --coverage.enabled --coverage.reporter=lcov; scope=" + scope
	case "jest":
		if allTests {
			return "jest --coverage --coverageReporters=lcov; scope=" + scope
		}
		return "jest --coverage --coverageReporters=lcov --findRelatedTests <sources>; scope=" + scope
	case "c8":
		return "c8 --reporter=lcov <package manager> run test; scope=all-tests"
	}
	return ""
}

// KotlinCoverageProducer is the built-in producer of Kotlin's strict
// coverage evidence by runner (coverage.KotlinRunner): Gradle's
// jacocoTestReport or Kover's koverXmlReport, or Maven's jacoco:report,
// over the test classes of the module that reach the file, or with
// allTests the module's whole suite.
func KotlinCoverageProducer(runner string, allTests bool) string {
	switch {
	case runner == "jacoco" && allTests:
		return "gradle -p <module> test jacocoTestReport; scope=all-tests"
	case runner == "jacoco":
		return "gradle -p <module> test --tests <reaching classes> jacocoTestReport; scope=own"
	case runner == "kover" && allTests:
		return "gradle -p <module> koverXmlReport; scope=all-tests"
	case runner == "kover":
		return "gradle -p <module> test --tests <reaching classes> koverXmlReport; scope=own"
	case runner == "maven" && allTests:
		return "mvn -q jacoco:prepare-agent test jacoco:report; scope=all-tests"
	case runner == "maven":
		return "mvn -q jacoco:prepare-agent test jacoco:report -Dtest=<reaching classes> -Dsurefire.failIfNoSpecifiedTests=false; scope=own"
	}
	return ""
}

// validCoverageProducer says whether producer is a built-in producer of
// language's strict coverage evidence.
func validCoverageProducer(language, producer string) bool {
	if producer == "" {
		return false
	}
	var producers func(runner string, allTests bool) string
	var runners []string
	switch language {
	case "typescript":
		producers, runners = TypeScriptCoverageProducer, []string{"vitest", "jest", "c8"}
	case "kotlin":
		producers, runners = KotlinCoverageProducer, []string{"jacoco", "kover", "maven"}
	default:
		return producer == CoverageProducer(language, false) || producer == CoverageProducer(language, true)
	}
	for _, runner := range runners {
		if producer == producers(runner, false) || producer == producers(runner, true) {
			return true
		}
	}
	return false
}

// evidenceKey is how CoverageEvidence.Language names language: "" for Go,
// whose evidence predates the field.
func evidenceKey(language string) string {
	if language == "go" {
		return ""
	}
	return language
}

// evidenceVersion is the current version of language's evidence.
func evidenceVersion(language string) int {
	if language == "go" {
		return goCoverageEvidenceVersion
	}
	return lineCoverageEvidenceVersion
}

// coverageEvidence binds report's measurement of file to unit, as its
// language's evidence: Go's blocks, or another strict language's lines.
func coverageEvidence(file *lang.File, unit lang.Unit, report *coverage.Report, producer string, inputs map[string]string) *CoverageEvidence {
	if file.Spec.Name == "go" {
		return goCoverageEvidence(file, unit, report.GoBlocks(file.Path), producer, inputs)
	}
	lines, proven := report.Lines(file.Path)
	return lineCoverageEvidence(file, unit, lines, proven, producer, inputs)
}

// lineCoverageEvidence binds the executable lines of file, of a
// line-precision format, to unit: those from its BodyLine through its
// EndLine, outside the inline units inside it, which are theirs. A def or
// signature line before BodyLine runs when the unit is defined, not when it
// is called, so it is no line of the unit's to prove. The evidence is
// complete when proven: the measurement lists every executable line of the
// file (coverage.Report.Lines), so a unit with none has nothing to prove.
// A Kotlin function's lines start at its body's first statement
// (kotlinBodyLine), and one with no statement in its body has nothing to
// prove, measured or not.
func lineCoverageEvidence(file *lang.File, unit lang.Unit, lines []coverage.Line, proven bool, producer string, inputs map[string]string) *CoverageEvidence {
	evidence := &CoverageEvidence{
		Version: lineCoverageEvidenceVersion, Language: file.Spec.Name, File: project.FromRoot(file.Path),
		Function: unitID(unit.Namespace, unit.Name), Hash: UnitHash(file, unit), Producer: producer,
		Inputs: cloneHashes(inputs), Blocks: []CoverageBlock{}, Complete: proven,
	}
	first := unit.BodyLine
	if first <= 0 {
		first = unit.StartLine
	}
	if file.Spec.Name == "kotlin" {
		line, ok := kotlinBodyLine(unit.Node)
		if !ok {
			evidence.Complete = true
			return evidence
		}
		first = line
	}
	if !proven {
		return evidence
	}
	for _, line := range lines {
		if line.Line < first || line.Line > unit.EndLine || inInner(file, unit, line.Line) {
			continue
		}
		evidence.Blocks = append(evidence.Blocks, CoverageBlock{
			Span: strconv.Itoa(line.Line), Line: line.Line, Weight: 1, Covered: line.Covered,
		})
	}
	return evidence
}

// kotlinBodyLine is the first line of the Kotlin function at n whose
// coverage is the function's: its block body's first statement, or its
// expression body's expression, and false for a function with neither, one
// whose block holds no statement or that has no body. The declaration's
// own lines are not the function's: JaCoCo puts on them the bridge that
// fills in default arguments, which runs only for a call that omits one,
// and the methods a data class or @JvmOverloads generates. A one-line
// expression body shares its line with them, which is then covered when
// any of its instructions ran.
func kotlinBodyLine(n *sitter.Node) (int, bool) {
	if n == nil {
		return 0, false
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		body := n.NamedChild(i)
		if body.Kind() != "function_body" {
			continue
		}
		for j := uint(0); j < body.NamedChildCount(); j++ {
			if c := body.NamedChild(j); c.Kind() != "line_comment" && c.Kind() != "multiline_comment" {
				return int(c.StartPosition().Row) + 1, true
			}
		}
		return 0, false
	}
	return 0, false
}

// inInner says whether line belongs to an inline unit inside unit, from
// the line after its start, as CRAP leaves inline units' lines out.
func inInner(file *lang.File, unit lang.Unit, line int) bool {
	for _, i := range unit.Inner {
		in := file.Units[i]
		if line >= max(in.BodyLine, in.StartLine+1) && line <= in.EndLine {
			return true
		}
	}
	return false
}

func goCoverageEvidence(file *lang.File, unit lang.Unit, blocks []coverage.GoBlock, producer string, inputs map[string]string) *GoCoverageEvidence {
	evidence := &GoCoverageEvidence{
		Version: goCoverageEvidenceVersion, File: project.FromRoot(file.Path),
		Function: unitID(unit.Namespace, unit.Name), Hash: UnitHash(file, unit), Producer: producer,
		Inputs: cloneHashes(inputs), Blocks: []GoCoverageBlock{},
	}
	for _, block := range blocks {
		if !goBlockInUnit(unit.Node, block) || block.Weight <= 0 {
			continue
		}
		evidence.Blocks = append(evidence.Blocks, GoCoverageBlock{
			Span: block.Span, Line: block.Line, Column: block.Column,
			Weight: block.Weight, Covered: block.Covered,
		})
	}
	slices.SortFunc(evidence.Blocks, func(a, b GoCoverageBlock) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Column != b.Column {
			return a.Column - b.Column
		}
		if a.Span < b.Span {
			return -1
		}
		if a.Span > b.Span {
			return 1
		}
		return 0
	})
	evidence.Complete = len(evidence.Blocks) > 0 || !goHasExecutableStatements(unit.Node)
	return evidence
}

func goBlockInUnit(unit *sitter.Node, block coverage.GoBlock) bool {
	if unit == nil {
		return false
	}
	start, end := unit.StartPosition(), unit.EndPosition()
	row, column := uint(block.Line-1), uint(block.Column-1)
	return (row > start.Row || row == start.Row && column >= start.Column) &&
		(row < end.Row || row == end.Row && column < end.Column)
}

func goHasExecutableStatements(unit *sitter.Node) bool {
	if unit == nil {
		return false
	}
	body := unit.ChildByFieldName("body")
	if body == nil {
		return false
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		if body.NamedChild(i).Kind() != "comment" {
			return true
		}
	}
	return false
}

// FreshGoCoverageEvidence binds measured blocks to one frozen admitted
// function without consulting persistent snapshots or the live project root.
func FreshGoCoverageEvidence(root string, admitted FreshUnit, blocks []coverage.GoBlock, producer string, inputs map[string]string) (*GoCoverageEvidence, error) {
	source := filepath.Join(root, filepath.FromSlash(admitted.Path))
	file, err := lang.ParseFile(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	for _, unit := range file.Units {
		if unit.StartLine != admitted.StartLine || unit.EndLine != admitted.EndLine || UnitHash(file, unit) != admitted.Hash {
			continue
		}
		evidence := &GoCoverageEvidence{
			Version: goCoverageEvidenceVersion, File: admitted.Path,
			Function: unitID(unit.Namespace, unit.Name), Hash: admitted.Hash,
			Producer: producer, Inputs: cloneHashes(inputs), Blocks: []GoCoverageBlock{},
		}
		for _, block := range blocks {
			if goBlockInUnit(unit.Node, block) && block.Weight > 0 {
				evidence.Blocks = append(evidence.Blocks, GoCoverageBlock{
					Span: block.Span, Line: block.Line, Column: block.Column,
					Weight: block.Weight, Covered: block.Covered,
				})
			}
		}
		slices.SortFunc(evidence.Blocks, func(a, b GoCoverageBlock) int {
			if a.Line != b.Line {
				return a.Line - b.Line
			}
			if a.Column != b.Column {
				return a.Column - b.Column
			}
			return strings.Compare(a.Span, b.Span)
		})
		evidence.Complete = len(evidence.Blocks) > 0 || !goHasExecutableStatements(unit.Node)
		return evidence, nil
	}
	return nil, fmt.Errorf("frozen function %s at %s:%d no longer matches its admitted identity", admitted.Function, admitted.Path, admitted.StartLine)
}
