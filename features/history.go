// The steps of history.feature: what units, dry and scrap print for a past
// commit (--at), judged from what they print, from the files they leave in
// the project and from what they keep under the git common directory, as
// every step here, from the outside: a project's folder, git and the
// binary's output, never the code.
package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// historyState is one history.feature scenario's state beside the world's.
type historyState struct {
	w *world
	// commits is how many commits the scenario made.
	commits int
	// linked is the folder of the linked worktree a Given made, "" for none.
	linked string
	// kept is what the git common directory held of a command's kept
	// result when the scenario last looked, by slash-separated path from
	// it.
	kept map[string][]byte
}

// historyFeature declares history.feature's steps, historySteps, which
// tools/bin/assemble lists in steps_gen.go.
var historyFeature = featureFile{Name: "history.feature", Steps: historySteps}

func historySteps(sc *godog.ScenarioContext, w *world) {
	h := &historyState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*h = historyState{w: w}
		return ctx, nil
	})
	sc.Step(`^the project is a git repository$`, func() error { return w.git("init", "-q") })
	sc.Step(`^"([^"]*)" holds:$`, func(name string, text *godog.DocString) error {
		return w.write(name, text.Content+"\n")
	})
	sc.Step(`^everything is committed$`, h.commit)
	sc.Step(`^the project holds no "([^"]*)"$`, func(name string) error {
		if w.d.Exists(filepath.Join(w.dir, filepath.FromSlash(name))) {
			return fmt.Errorf("the project holds %s\n%s", name, w.report())
		}
		return nil
	})
	sc.Step(`^the git common directory holds one kept result of "([^"]*)"$`, func(command string) error {
		_, err := h.keptOf(command)
		return err
	})
	sc.Step(`^the kept result of "([^"]*)" is overwritten with "([^"]*)"$`, h.overwrite)
	sc.Step(`^the kept result of "([^"]*)" is valid JSON$`, h.validJSON)
	sc.Step(`^a linked worktree of the project$`, h.linkedWorktree)
	sc.Step(`^itos-cc runs in the linked worktree with "([^"]*)"$`, func(args string) error {
		fields, err := commandWords(args)
		if err != nil {
			return err
		}
		return w.runEnv(h.linked, w.env(), fields...)
	})
	sc.Step(`^the git common directory still holds the one kept result of "([^"]*)", as it was$`, h.unchanged)
	sc.Step(`^a folder in no git repository holding "([^"]*)"$`, func(name string) error {
		dir, err := w.d.Folder()
		if err != nil {
			return err
		}
		w.folders = append(w.folders, dir)
		if err := w.writeIn(dir, name, "export function alpha(): number {\n  return 1;\n}\n"); err != nil {
			return err
		}
		w.runIn = dir
		return nil
	})
}

// commit commits every file of the project, one commit after another.
func (h *historyState) commit() error {
	h.commits++
	if err := h.w.git("add", "-A"); err != nil {
		return err
	}
	return h.w.git("commit", "-q", "-m", fmt.Sprintf("commit %d", h.commits))
}

// cacheDir is the folder the kept results are under: itos-cc's, in the git
// common directory of the project.
func (h *historyState) cacheDir() string {
	return filepath.Join(h.w.dir, ".git", "itos-cc", "history")
}

// keptOf is the kept results of command in the git common directory, by
// slash-separated path from it and with their bytes, an error unless it
// holds exactly one.
func (h *historyState) keptOf(command string) (map[string][]byte, error) {
	files, err := h.w.d.Files(context.Background(), h.cacheDir())
	if err != nil {
		return nil, fmt.Errorf("the git common directory holds no itos-cc/history: %w\n%s", err, h.w.report())
	}
	found := map[string][]byte{}
	for _, f := range files {
		if base := filepath.Base(f); strings.HasPrefix(base, command+"-") && strings.HasSuffix(base, ".json") {
			text, err := h.w.d.Read(filepath.Join(h.cacheDir(), filepath.FromSlash(f)))
			if err != nil {
				return nil, err
			}
			found[f] = text
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("the git common directory holds %d kept results of %s, not one: %v\n%s", len(found), command, files, h.w.report())
	}
	h.kept = found
	return found, nil
}

// overwrite replaces the one kept result of command with text.
func (h *historyState) overwrite(command, text string) error {
	found, err := h.keptOf(command)
	if err != nil {
		return err
	}
	for f := range found {
		return h.w.d.Write(filepath.Join(h.cacheDir(), filepath.FromSlash(f)), []byte(text))
	}
	return nil
}

// validJSON is whether the one kept result of command is JSON again.
func (h *historyState) validJSON(command string) error {
	found, err := h.keptOf(command)
	if err != nil {
		return err
	}
	for f, text := range found {
		if !json.Valid(text) {
			return fmt.Errorf("the kept result %s is not JSON: %q", f, text)
		}
	}
	return nil
}

// linkedWorktree makes a linked worktree of the project, in a folder of
// its own, at the project's HEAD.
func (h *historyState) linkedWorktree() error {
	// The first run's kept result is what the second is held to.
	if _, err := h.keptOf("units"); err != nil {
		return err
	}
	dir, err := h.w.d.Folder()
	if err != nil {
		return err
	}
	h.w.folders = append(h.w.folders, dir)
	h.linked = dir
	return h.w.git("worktree", "add", "-q", "--detach", dir, "HEAD")
}

// unchanged is whether the git common directory still holds the one kept
// result it held, byte for byte.
func (h *historyState) unchanged(command string) error {
	before := h.kept
	after, err := h.keptOf(command)
	if err != nil {
		return err
	}
	for f, text := range after {
		if old, ok := before[f]; !ok || !bytes.Equal(old, text) {
			return fmt.Errorf("the kept result %s changed:\n%s\nwas:\n%s", f, text, before[f])
		}
	}
	return nil
}
