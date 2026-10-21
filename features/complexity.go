// The steps of complexity.feature: a function's cyclomatic complexity, as
// itos-cc crap --no-coverage --json reports it of the one file a scenario
// wrote, no test or coverage tool run. Like every step here, they read the
// binary's output alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// complexityState is one complexity.feature scenario's state beside the
// world's.
type complexityState struct {
	w *world
	// file is the source the Given wrote, slash-separated from the scratch
	// folder, and source its text.
	file, source string
}

// straightLine is a function of each language with no branch, written in a
// file of the language's extension (extensions, dry.go).
var straightLine = map[string]string{
	"TypeScript": "export function total(a: number, b: number): number {\n  const sum = a + b;\n  return sum * 2;\n}\n",
	"Python":     "def total(a, b):\n    sum = a + b\n    return sum * 2\n",
	"Go":         "package board\n\nfunc total(a, b int) int {\n\tsum := a + b\n\treturn sum * 2\n}\n",
	"Kotlin":     "fun total(a: Int, b: Int): Int {\n    val sum = a + b\n    return sum * 2\n}\n",
}

// complexityFeature declares complexity.feature's steps, complexitySteps, which
// tools/bin/assemble lists in steps_gen.go.
var complexityFeature = featureFile{Name: "complexity.feature", Steps: complexitySteps}

func complexitySteps(sc *godog.ScenarioContext, w *world) {
	c := &complexityState{w: w}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*c = complexityState{w: w}
		return ctx, nil
	})

	sc.Step(`^a (TypeScript|Python|Go|Kotlin) function with no branches$`, func(language string) error {
		return c.writeSource("total"+extensions[language], straightLine[language])
	})
	// A Go function is written in a file of its package.
	sc.Step(`^the (TypeScript|Python|Go|Kotlin) function:$`, func(language string, text *godog.DocString) error {
		source := text.Content + "\n"
		if language == "Go" && !strings.HasPrefix(source, "package ") {
			source = "package board\n\n" + source
		}
		return c.writeSource("branches"+extensions[language], source)
	})
	sc.Step(`^its complexity is measured$`, func() error {
		if c.file == "" {
			return fmt.Errorf("no function was written to measure")
		}
		return w.runWith(w.env(), "crap --no-coverage --json "+c.file)
	})
	sc.Step(`^it is (\d+)$`, c.itIs)
	sc.Step(`^it is (\d+) in a (\.\w+) file too$`, c.inAnotherFile)
	sc.Step(`^a Kotlin safe call a\?\.b counts like the elvis \?:$`, c.safeCallCountsLikeElvis)
}

// writeSource writes source to name in the scratch folder, the file the
// scenario measures.
func (c *complexityState) writeSource(name, source string) error {
	c.file, c.source, c.w.wrote = name, source, name
	return c.w.write(name, source)
}

// complexityOf is the complexity of the one function of output, the object
// crap --json printed.
func complexityOf(output string) (int, error) {
	var object struct {
		Entries []struct {
			Name       string `json:"name"`
			Complexity int    `json:"complexity"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(output), &object); err != nil {
		return 0, fmt.Errorf("standard output is not crap's object: %v", err)
	}
	if len(object.Entries) != 1 {
		return 0, fmt.Errorf("%d functions are scored, not the one written", len(object.Entries))
	}
	return object.Entries[0].Complexity, nil
}

// itIs is whether the run measured the one function written at complexity
// want.
func (c *complexityState) itIs(want int) error {
	w := c.w
	if w.exit != 0 {
		return fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	got, err := complexityOf(w.stdout)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, w.report())
	}
	if got != want {
		return fmt.Errorf("its complexity is %d, not %d\n%s", got, want, w.report())
	}
	return nil
}

// measure is the complexity of the one function of the file name, written
// with source in the scratch folder.
func (c *complexityState) measure(name, source string) (int, error) {
	w := c.w
	if err := w.write(name, source); err != nil {
		return 0, err
	}
	code, stdout, stderr, err := w.capture(w.dir, w.env(), "crap", "--no-coverage", "--json", name)
	if err != nil {
		return 0, err
	}
	report := fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, stdout, stderr)
	if code != 0 {
		return 0, fmt.Errorf("itos-cc exited %d over %s\n%s", code, name, report)
	}
	got, err := complexityOf(stdout)
	if err != nil {
		return 0, fmt.Errorf("%v over %s\n%s", err, name, report)
	}
	return got, nil
}

// inAnotherFile is whether the function written, in a file of extension
// ext beside it, is measured at complexity want too.
func (c *complexityState) inAnotherFile(want int, ext string) error {
	if c.file == "" || filepath.Ext(c.file) == ext {
		return fmt.Errorf("no function of another extension than %s was written", ext)
	}
	name := strings.TrimSuffix(c.file, filepath.Ext(c.file)) + ext
	got, err := c.measure(name, c.source)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("in %s its complexity is %d, not %d", name, got, want)
	}
	return nil
}

// safeCallCountsLikeElvis is whether a Kotlin function returning a?.b and
// one returning a ?: b each count one decision more than one returning a.
func (c *complexityState) safeCallCountsLikeElvis() error {
	plain, err := c.measure("plain.kt", "fun plain(a: String?): String? {\n    return a\n}\n")
	if err != nil {
		return err
	}
	safe, err := c.measure("safe.kt", "fun safe(a: String?): Int? {\n    return a?.length\n}\n")
	if err != nil {
		return err
	}
	elvis, err := c.measure("elvis.kt", "fun elvis(a: String?): String {\n    return a ?: \"x\"\n}\n")
	if err != nil {
		return err
	}
	if elvis != plain+1 || safe != elvis {
		return fmt.Errorf("a?.b is %d and a ?: b is %d, where a alone is %d: each should count one decision", safe, elvis, plain)
	}
	return nil
}
