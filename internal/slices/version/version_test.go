package version

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
)

// runVersion runs the version command, --json or not, over a handler that
// holds version, holds it to exit 0, and returns what it wrote on stdout
// and stderr.
func runVersion(t *testing.T, asJSON bool, version string) (stdout, stderr string) {
	t.Helper()
	var out, errs strings.Builder
	ui := &cli.UI{In: strings.NewReader(""), Stdout: &out, Stderr: &errs}
	if code := (CLI{JSON: asJSON}).Run(ui, Handler{Version: version}); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	return out.String(), errs.String()
}

// Without --json, version prints one line, itos-cc and the version, and
// nothing on stderr: no commit line, whatever the build knows.
func TestVersionPrintsOneLine(t *testing.T) {
	stdout, stderr := runVersion(t, false, "v1.2.3")
	if want := "itos-cc v1.2.3\n"; stdout != want {
		t.Errorf("standard output = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("error output = %q, want none", stderr)
	}
}

// With --json, the one object every command prints, its keys in the order
// every object has them: ok, schema and the version, no commit key.
func TestVersionJSONIsTheObjectOfTheVersion(t *testing.T) {
	stdout, stderr := runVersion(t, true, "dev")
	if want := "{\n  \"ok\": true,\n  \"schema\": 1,\n  \"version\": \"dev\"\n}\n"; stdout != want {
		t.Errorf("standard output = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("error output = %q, want none", stderr)
	}
}

// version's help page, in the layout every command's has: the text it
// prints, its --json key, its exit codes and examples.
func TestVersionHelpGivesItsContract(t *testing.T) {
	page := cli.Page{Program: "itos-cc", Command: "version", Synopsis: "[options]", Contract: CLI{}.Contract()}.String()
	for _, want := range []string{
		"usage: itos-cc version [options]\n\nPrints \"itos-cc <version>\". --version is the same.\n",
		`{"schema": 1, "ok": true|false, "version",`,
		"  0   success\n  80  a usage error: a bad flag or an argument\n  100 ",
		"  itos-cc version\n  itos-cc --version --json\n",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the help does not say %q\n%s", want, page)
		}
	}
}

// ports are the Ports cmd's adapters give the handler, held in memory: the
// version the build says it is.
type ports struct{ version string }

func (p ports) BuildVersion() string { return p.version }

// NewHandler builds the handler over its Ports: the version they say, which
// Run then prints.
func TestNewHandlerIsBuiltOverItsPorts(t *testing.T) {
	h, err := NewHandler(t.Context(), ports{version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != "1.2.3" {
		t.Errorf("handler's version = %q, want the Ports' 1.2.3", h.Version)
	}
}
