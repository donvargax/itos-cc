package features

import (
	"fmt"

	"github.com/cucumber/godog"
)

// The steps of ID-MUT-428 to 431: two Go init functions share
// example.com/fx/board#init and a survivor at the same place in each, line
// 2, column 10, so only their hashes tell their entries apart.
const (
	sameNameFile   = "board/setup.go"
	sameNameSource = "package board\n\n// Low and Top say whether three is under four and five under six.\nvar Low, Top bool\n\nfunc init() {\n\tLow = 3 < 4\n}\n\nfunc init() {\n\tTop = 5 < 6\n}\n"
	// sameNameTest survives `<` → `<=` in both: it never compares equal
	// numbers.
	sameNameTest = "package board\n\nimport \"testing\"\n\nfunc TestSetup(t *testing.T) {\n\tif !Low || !Top {\n\t\tt.Fatal(\"setup\")\n\t}\n}\n"
	// sameNameProfile is go test's coverprofile of sameNameSource, both
	// init functions executed.
	sameNameProfile = "mode: set\nexample.com/fx/board/setup.go:7.2,8.1 1 1\nexample.com/fx/board/setup.go:11.2,12.1 1 1\n"
)

func (i *identityException) sameNameSteps(sc *godog.ScenarioContext) {
	w := i.w
	sc.Step("^a committed Go module whose two init functions of board/setup\\.go each survive `<` → `<=` at the same place in the function$", i.sameNameModule)
	sc.Step(`^"itos-cc mutation run board/setup\.go" recorded both surviving$`, func() error {
		if err := w.runWith(w.env(), "mutation run "+sameNameFile); err != nil {
			return err
		}
		if w.exit != 1 {
			return fmt.Errorf("mutation run exited %d, not 1 for two survivors\n%s", w.exit, w.report())
		}
		return nil
	})
	i.sameNameRunSteps(sc)
}

// sameNameModule writes and commits the module, and declares that its test
// survives `<` → `<=` in both init functions and covers both.
func (i *identityException) sameNameModule() error {
	w := i.w
	for name, text := range map[string]string{"go.mod": identityModule, sameNameFile: sameNameSource, "board/setup_test.go": sameNameTest} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := w.git("add", "go.mod", sameNameFile, "board/setup_test.go"); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "board"); err != nil {
		return err
	}
	surviving := []Site{{File: sameNameFile, Line: 7, Original: "<", Replacement: "<="}, {File: sameNameFile, Line: 11, Original: "<", Replacement: "<="}}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: identityCommand, Surviving: surviving}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: sameNameProfile})
}
