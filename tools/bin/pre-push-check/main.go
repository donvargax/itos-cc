// Command pre-push-check proves tools/hooks/pre-push through real pushes to
// a local bare remote from two clones in a temporary folder of its own, the
// hook declared in each exactly as README.md tells a clone to declare it, and
// git itself running it (T-177), as tools/bin/pre-commit-check proves
// tools/hooks/pre-commit. It writes only inside that folder, never this
// repository, and reaches no network: git talks to the bare remote on disk,
// and go builds a fixture module with no requirements, GOPROXY off and
// GOTOOLCHAIN local.
//
// The clash it reproduces is the one the hook exists for: each clone adds a
// file declaring helper in the same package, each push compiling alone, and
// the second clone's tree stops compiling once it rebases onto the first's.
// A go wrapper on PATH counts the hook's go runs, so each push is seen to run
// go vet once, the pushes accepted as much as the one refused. Run it from
// the repository, go run ./tools/bin/pre-push-check; it needs git, go and,
// as git's hooks do, sh. Exit 1 naming each case that went wrong.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Stderr))
}

// check carries what every case needs: the scratch folder, the environment
// git and the hook run in, the real go, and the cases that went wrong.
type check struct {
	tmp    string
	env    []string
	goBin  string
	stderr io.Writer
	failed bool
}

func run(stderr io.Writer) int {
	c := &check{stderr: stderr}
	if err := c.prove(); err != nil {
		say(stderr, "pre-push-check: %v\n", err)
		return 1
	}
	if c.failed {
		return 1
	}
	return 0
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func (c *check) fail(name, format string, a ...any) {
	say(c.stderr, "pre-push-check: %s: %s\n", name, fmt.Sprintf(format, a...))
	c.failed = true
}

// prove sets the remote and clones up and runs each case in turn; an error
// is a setup that went wrong, a failed case is reported through fail.
func (c *check) prove() error {
	hook, err := c.hookText()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "pre-push-check-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	c.tmp = tmp
	if err := c.setEnv(); err != nil {
		return err
	}

	remote := filepath.Join(tmp, "remote.git")
	one := filepath.Join(tmp, "one")
	two := filepath.Join(tmp, "two")
	if _, err := c.git(tmp, "init", "-q", "--bare", "-b", "main", remote); err != nil {
		return err
	}
	if _, err := c.git(tmp, "init", "-q", "-b", "main", one); err != nil {
		return err
	}
	if err := c.configure(one); err != nil {
		return err
	}
	if err := writeFiles(one, map[string]string{
		"go.mod":               "module example.invalid/fixture\n\ngo 1.22\n",
		"fixture.go":           "package fixture\n\nfunc F() int { return 1 }\n",
		"tools/hooks/pre-push": hook,
	}); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(one, "tools/hooks/pre-push"), 0o755); err != nil { //nolint:gosec // the hook git runs must be executable
		return err
	}
	if err := c.commit(one, "start the module with the hook", "go.mod", "fixture.go", "tools/hooks/pre-push"); err != nil {
		return err
	}
	if _, err := c.git(one, "remote", "add", "origin", remote); err != nil {
		return err
	}
	if err := c.declare(one); err != nil {
		return err
	}

	c.push("a first push of a tree that compiles is accepted", one, remote, true)

	if _, err := c.git(tmp, "clone", "-q", remote, two); err != nil {
		return err
	}
	if err := c.configure(two); err != nil {
		return err
	}
	if err := c.declare(two); err != nil {
		return err
	}

	if err := c.add(one, "one.go", "package fixture\n\nfunc helper() int { return 1 }\n"); err != nil {
		return err
	}
	c.push("one clone's helper, compiling, is accepted", one, remote, true)

	if err := c.add(two, "two.go", "package fixture\n\nfunc helper() int { return 2 }\n"); err != nil {
		return err
	}
	name := "a rebased tree declaring helper twice is refused with vet's message"
	if out, err := c.goVet(two); err != nil {
		c.fail(name, "the other clone's helper should compile before the rebase: %v\n%s", err, out)
	}
	if _, err := c.git(two, "fetch", "-q", "origin"); err != nil {
		return err
	}
	if _, err := c.git(two, "rebase", "-q", "origin/main"); err != nil {
		return err
	}
	out := c.push(name, two, remote, false)
	for _, want := range []string{"pre-push: go vet ./... failed", "helper redeclared"} {
		if !strings.Contains(out, want) {
			c.fail(name, "the refusal does not say %q:\n%s", want, out)
		}
	}

	if err := c.add(two, "two.go", "package fixture\n\nfunc helperTwo() int { return 2 }\n"); err != nil {
		return err
	}
	c.push("the rebased tree, the clash renamed away, is accepted", two, remote, true)
	return nil
}

// hookText reads the hook from this repository's working tree, the one the
// check runs in.
func (c *check) hookText() (string, error) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("run it from inside the repository: git rev-parse --show-toplevel: %w", err)
	}
	b, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(top)), "tools", "hooks", "pre-push"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// setEnv writes the go wrapper and builds the environment: only each
// repository's own git config, no user or system hooks, no signing, and no
// GIT_ variable of a caller, a hook's GIT_DIR among them, reaching the clones.
func (c *check) setEnv() error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go is not on PATH: %w", err)
	}
	c.goBin = goBin
	bin := filepath.Join(c.tmp, "bin")
	if err := os.Mkdir(bin, 0o750); err != nil {
		return err
	}
	wrapper := fmt.Sprintf("#!/bin/sh\necho run >>'%s'\nexec '%s' \"$@\"\n",
		filepath.Join(c.tmp, "go.log"), goBin)
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(wrapper), 0o755); err != nil { //nolint:gosec // the wrapper the hook runs must be executable
		return err
	}
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(key, "GIT_"), key == "PATH", key == "GOFLAGS", key == "GOWORK",
			key == "GOPROXY", key == "GOTOOLCHAIN":
			continue
		}
		c.env = append(c.env, kv)
	}
	c.env = append(c.env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GOFLAGS=",
		"GOWORK=off",
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
	)
	return nil
}

// git runs git -C dir with the check's environment and returns its
// combined output.
func (c *check) git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // git, with arguments this program names
	cmd.Env = c.env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("git -C %s %s: %w\n%s", dir, strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}

// goVet runs the real go vet in dir, outside the hook and its count.
func (c *check) goVet(dir string) (string, error) {
	cmd := exec.Command(c.goBin, "vet", "./...") //nolint:gosec // the go found on PATH
	cmd.Dir = dir
	cmd.Env = c.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (c *check) configure(dir string) error {
	for _, kv := range [][2]string{
		{"user.name", "pre-push-check"},
		{"user.email", "pre-push-check@example.invalid"},
		{"commit.gpgsign", "false"},
	} {
		if _, err := c.git(dir, "config", kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// declare declares the hook in dir's clone with README.md's two lines and
// checks git lists it.
func (c *check) declare(dir string) error {
	if _, err := c.git(dir, "config", "hook.itos-cc-pre-push.event", "pre-push"); err != nil {
		return err
	}
	if _, err := c.git(dir, "config", "hook.itos-cc-pre-push.command", "tools/hooks/pre-push"); err != nil {
		return err
	}
	list, err := c.git(dir, "hook", "list", "pre-push")
	if err != nil {
		return err
	}
	if !strings.Contains(list, "itos-cc-pre-push") {
		v, _ := c.git(dir, "--version")
		return fmt.Errorf("%s lists no pre-push hook from config: %q", strings.TrimSpace(v), list)
	}
	return nil
}

func writeFiles(dir string, files map[string]string) error {
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (c *check) commit(dir, message string, paths ...string) error {
	if _, err := c.git(dir, append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	_, err := c.git(dir, "commit", "-q", "-m", message)
	return err
}

// add writes one Go file and commits it.
func (c *check) add(dir, name, text string) error {
	if err := writeFiles(dir, map[string]string{name: text}); err != nil {
		return err
	}
	return c.commit(dir, "add "+name, name)
}

// push pushes dir's main to the remote and checks the outcome, that the
// hook ran go once, and where the remote's main is after it: dir's HEAD when
// accepted, where it was when refused. It returns git's output.
func (c *check) push(name, dir, remote string, accept bool) string {
	log := filepath.Join(c.tmp, "go.log")
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		c.fail(name, "%v", err)
		return ""
	}
	before := c.rev(remote, "refs/heads/main")
	out, err := c.git(dir, "push", "-q", "origin", "main")
	switch {
	case accept && err != nil:
		c.fail(name, "wanted the push accepted, it was refused: %v", err)
	case !accept && err == nil:
		c.fail(name, "wanted the push refused, it was accepted:\n%s", out)
	}
	if runs := countLines(log); runs != 1 {
		c.fail(name, "wanted the hook to run go once, it ran it %d times", runs)
	}
	after := c.rev(remote, "refs/heads/main")
	head := c.rev(dir, "HEAD")
	switch {
	case accept && after != head:
		c.fail(name, "accepted, yet the remote's main is %q, not HEAD %q", after, head)
	case !accept && after != before:
		c.fail(name, "refused, yet the remote's main moved from %q to %q", before, after)
	}
	return out
}

// rev is the commit ref names in dir, or "" when it names none.
func (c *check) rev(dir, ref string) string {
	out, _ := c.git(dir, "rev-parse", "--verify", "-q", ref)
	return strings.TrimSpace(out)
}

func countLines(path string) int {
	b, err := os.ReadFile(path) //nolint:gosec // the check's own log in its scratch folder
	if err != nil {
		return -1
	}
	return bytes.Count(b, []byte("\n"))
}
