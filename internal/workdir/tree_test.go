package workdir

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// tree is a temporary working directory holding a Go source and its test,
// the test's working directory for its duration, git above it out of
// reach.
func tree(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"pkg/sum.go": "package pkg\n", "pkg/sum_test.go": "package pkg\n"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	return dir
}

func cancelled(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func TestDiscoverSplitsTheFilesUnderItsRoots(t *testing.T) {
	dir := tree(t)
	sources, tests, failure := (Tree{}).Discover(t.Context(), []string{"."})
	if failure != nil {
		t.Fatal(failure)
	}
	wantSources := []string{filepath.Join(dir, "pkg", "sum.go")}
	wantTests := []string{filepath.Join(dir, "pkg", "sum_test.go")}
	if !slices.Equal(sources, wantSources) || !slices.Equal(tests, wantTests) {
		t.Errorf("Discover(.) = %q, %q; want %q, %q", sources, tests, wantSources, wantTests)
	}
}

func TestDiscoverOfARootThatIsNotThereIsUnwalkable(t *testing.T) {
	absent := filepath.Join(tree(t), "absent")
	_, _, failure := (Tree{}).Discover(t.Context(), []string{absent})
	var unwalkable *port.Unwalkable
	if !errors.As(failure, &unwalkable) {
		t.Fatalf("Discover(absent) = %T %v, want Unwalkable", failure, failure)
	}
	// The walk's own message, as the command line has always printed it.
	if _, statErr := os.Stat(absent); unwalkable.Reason != statErr.Error() {
		t.Errorf("Reason = %q, want %q", unwalkable.Reason, statErr.Error())
	}
}

func TestChangedOutsideGitIsNoGit(t *testing.T) {
	tree(t)
	_, failure := (Tree{}).Changed(t.Context())
	var noGit *port.NoGit
	if !errors.As(failure, &noGit) || noGit.Reason == "" {
		t.Fatalf("Changed() = %T %v, want NoGit with git's reason", failure, failure)
	}
}

func TestChangedListsWhatGitReports(t *testing.T) {
	dir := tree(t)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("git init: %v\n%s", err, out)
	}
	changed, failure := (Tree{}).Changed(t.Context())
	if failure != nil {
		t.Fatal(failure)
	}
	if want := []string{"pkg/sum.go", "pkg/sum_test.go"}; !slices.Equal(changed, want) {
		t.Errorf("Changed() = %q, want the untracked %q", changed, want)
	}
}

func TestADoneContextFindsNothing(t *testing.T) {
	tree(t)
	sources, tests, failure := (Tree{}).Discover(cancelled(t), []string{"."})
	var stopped *port.Cancelled
	if !errors.As(failure, &stopped) || sources != nil || tests != nil {
		t.Errorf("Discover() = %q, %q, %T; want Cancelled and nothing", sources, tests, failure)
	}
	changed, failure := (Tree{}).Changed(cancelled(t))
	if !errors.As(failure, &stopped) || changed != nil {
		t.Errorf("Changed() = %q, %T; want Cancelled and nothing", changed, failure)
	}
}

func TestExistsSaysWhetherAPathIsThere(t *testing.T) {
	tree(t)
	for path, want := range map[string]bool{"pkg": true, "pkg/sum.go": true, ".": true, "absent": false, "pkg/sum.go/x": false} {
		if got := (Tree{}).Exists(t.Context(), path); got != want {
			t.Errorf("Exists(%q) = %t, want %t", path, got, want)
		}
	}
}
