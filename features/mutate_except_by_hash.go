package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of slice-76 (issue #87): excepting a survivor re-points an entry
// stale only by its function's hash. They reuse slice-41's module. A
// signature change moves the function's hash and the site's offset from its
// start, and leaves the site's line_in_function, column and line text. The
// steps carry the hashes, measured by hand with sha256sum, and read only what
// itos-cc prints and writes.

const (
	// byHashSignature is Count's signature once it changed.
	byHashSignature = "func Count(x int) (ok bool)"
	// byHashCurrent is Count's hash with that signature, and byHashIdentity
	// its mutant's identity then.
	byHashCurrent  = "9abb4792573b0b1f"
	byHashIdentity = "board/board.go:example.com/fx/board#Count@4:6:" + byHashCurrent + ":40:<><="
	// byHashTwoSites is board/board.go with Count's two sites of one change,
	// at 5:7 and 8:11, its test surviving both: it never checks Count(3) or
	// Count(5).
	byHashTwoSites = "package board\n\n// Count says whether x is under five.\nfunc Count(x int) bool {\n\tif x < 3 {\n\t\treturn true\n\t}\n\treturn x < 5\n}\n"
	byHashTwoTest  = "package board\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {\n\tif !Count(1) || !Count(4) || Count(6) {\n\t\tt.Fatal(\"Count\")\n\t}\n}\n"
	// byHashTwoProfile is go test's coverprofile of byHashTwoSites, every
	// block executed.
	byHashTwoProfile = "mode: set\nexample.com/fx/board/board.go:5.2,5.11 1 1\nexample.com/fx/board/board.go:5.11,7.3 1 1\nexample.com/fx/board/board.go:8.2,8.14 1 1\n"
	// byHashTwoHash and byHashTwoCurrent are Count's hash with both sites,
	// before and after its signature changed.
	byHashTwoHash    = "75de393fd4576f9d"
	byHashTwoCurrent = "5674a01c597318b6"
)

// byHashTwoIdentity is the identity of Count's site of offset, under hash.
func byHashTwoIdentity(hash string, offset int) string {
	return fmt.Sprintf("board/board.go:example.com/fx/board#Count@4:9:%s:%d:<><=", hash, offset)
}

// byHash is the state of the steps: itos-cc.yaml while its entries held,
// before Count's signature changed.
type byHash struct {
	i       *identityException
	entries string
}

func (i *identityException) byHashSteps(sc *godog.ScenarioContext) {
	b := &byHash{i: i}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*b = byHash{i: i}
		return ctx, nil
	})
	sc.Step(`^Count's signature became "func Count\(x int\) \(ok bool\)"$`, b.signature)
	sc.Step("^Count's signature became \"func Count\\(x int\\) \\(ok bool\\)\", and \"itos-cc mutation run board/board\\.go\" recorded its `<` → `<=` at board/board\\.go:5:11 surviving$", func() error {
		if err := b.signature(); err != nil {
			return err
		}
		return i.recorded("mutation run "+identityFile, "survived", 1)
	})
	sc.Step(`^I run "itos-cc mutation except" with the identity of that mutant as the code is now, --json and no --reason$`, func() error {
		return i.w.runWith(i.w.env(), "mutation except '"+byHashIdentity+"' --json")
	})
	sc.Step(`^it names Count's current hash, line_in_function 2, column 11, and keeps the reason "x is never 5"$`, func() error {
		_, err := i.entry(byHashCurrent)
		return err
	})
	sc.Step(`^it names Count's current hash and keeps the reason "x is never 5"$`, func() error {
		_, err := i.entry(byHashCurrent)
		return err
	})
	sc.Step(`^run again with no --reason, the entry holding now, the command is "flags\.value-missing", exit 80, leaving itos-cc\.yaml byte for byte as it was$`, b.holdingNeedsReason)
	sc.Step(`^given "--reason 'reworded'" while the entry is stale, it is re-pointed the same way, its reason "reworded"$`, b.reworded)
	sc.Step("^a committed Go module whose \"example\\.com/fx/board#Count\" holds two `<` → `<=` sites on two lines, each surviving its test$", b.twoSites)
	sc.Step(`^itos-cc\.yaml excepts both, the first with reason "first reviewed", the second with reason "second reviewed"$`, b.exceptBoth)
	sc.Step(`^Count's signature changed, leaving both sites at their places in the function$`, b.signature)
	sc.Step(`^I run "itos-cc mutation except" with the identity of the first site as the code is now, --json and no --reason$`, func() error {
		return i.w.runWith(i.w.env(), "mutation except '"+byHashTwoIdentity(byHashTwoCurrent, 36)+"' --json")
	})
	sc.Step(`^the first site's entry names Count's current hash and keeps the reason "first reviewed"$`, func() error {
		return b.twoEntry(0, map[string]any{"hash": byHashTwoCurrent, "line_in_function": 2, "column": 7, "reason": "first reviewed", "line_text": "if x < 3 {"})
	})
	sc.Step(`^the second site's entry is as it was$`, func() error {
		return b.twoEntry(1, map[string]any{"hash": byHashTwoHash, "line_in_function": 5, "column": 11, "reason": "second reviewed", "line_text": "return x < 5"})
	})
	sc.Step(`^itos-cc\.yaml has two entries under mutation\.exceptions$`, b.twoEntries)
	b.staleIdentitySteps(sc)
}

// signature changes Count's signature in board/board.go as it is, keeping
// itos-cc.yaml as it holds now.
func (b *byHash) signature() error {
	w := b.i.w
	entries, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	b.entries = string(entries)
	return b.i.edit(func(text string) string {
		return strings.Replace(text, "func Count(x int) bool", byHashSignature, 1)
	})
}

func (b *byHash) holdingNeedsReason() error {
	w := b.i.w
	before, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation except '"+byHashIdentity+"' --json"); err != nil {
		return err
	}
	object, err := w.json()
	if err != nil {
		return err
	}
	problems := checkProblems(object)
	if w.exit != 80 || len(problems) != 1 || problems[0]["rule"] != "flags.value-missing" {
		return fmt.Errorf("not flags.value-missing, exit 80\n%s", w.report())
	}
	after, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	if string(after) != string(before) {
		return fmt.Errorf("itos-cc.yaml changed\n--- before\n%s--- after\n%s", before, after)
	}
	return nil
}

// reworded puts the stale entry back and excepts its mutant with a reason,
// whole: re-pointed, and the reason the one given.
func (b *byHash) reworded() error {
	w := b.i.w
	if err := w.write("itos-cc.yaml", b.entries); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation except '"+byHashIdentity+"' --reason 'reworded' --json"); err != nil {
		return err
	}
	object, err := w.json()
	if err != nil {
		return err
	}
	if w.exit != 0 || object["replaced"] != true {
		return fmt.Errorf("given a reason, not re-pointed\n%s", w.report())
	}
	entries, err := b.i.entries()
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not one: %v", len(entries), entries)
	}
	return exceptionFields(entries[0], map[string]any{"hash": byHashCurrent, "line_in_function": 2, "column": 11, "reason": "reworded", "line_text": "return x < 5"})
}

// twoSites writes and commits the module with Count's two sites.
func (b *byHash) twoSites() error {
	w := b.i.w
	for name, text := range map[string]string{"go.mod": identityModule, identityFile: byHashTwoSites, "board/board_test.go": byHashTwoTest} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := w.git("add", "go.mod", identityFile, "board/board_test.go"); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "board"); err != nil {
		return err
	}
	surviving := []Site{{File: identityFile, Line: 5, Original: "<", Replacement: "<="}, {File: identityFile, Line: 8, Original: "<", Replacement: "<="}}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: identityCommand, Surviving: surviving}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: byHashTwoProfile})
}

// exceptBoth excepts both sites from their identities, which runs nothing
// (slice-41), and commits the itos-cc.yaml that writes.
func (b *byHash) exceptBoth() error {
	w := b.i.w
	for _, site := range []struct {
		offset int
		reason string
	}{{31, "first reviewed"}, {64, "second reviewed"}} {
		if err := w.runWith(w.env(), "mutation except '"+byHashTwoIdentity(byHashTwoHash, site.offset)+"' --reason '"+site.reason+"'"); err != nil {
			return err
		}
		if w.exit != 0 {
			return fmt.Errorf("excepting the site at offset %d exited %d\n%s", site.offset, w.exit, w.report())
		}
	}
	if err := w.git("add", "itos-cc.yaml"); err != nil {
		return err
	}
	return w.git("commit", "-qm", "except both of Count's survivors")
}

func (b *byHash) twoEntries() error {
	entries, err := b.i.entries()
	if err != nil {
		return err
	}
	if len(entries) != 2 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not two: %v\n%s", len(entries), entries, b.i.w.report())
	}
	return nil
}

// twoEntry is whether the entry at at holds want.
func (b *byHash) twoEntry(at int, want map[string]any) error {
	entries, err := b.i.entries()
	if err != nil {
		return err
	}
	if len(entries) != 2 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not two: %v\n%s", len(entries), entries, b.i.w.report())
	}
	return exceptionFields(entries[at], want)
}
