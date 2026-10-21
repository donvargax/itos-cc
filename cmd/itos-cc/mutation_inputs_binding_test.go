package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type unavailableTools struct{ mutation.Toolchain }

func (unavailableTools) Observe(context.Context, mutation.CommandRequest) (mutation.ToolFacts, mutation.InputFailure) {
	return mutation.ToolFacts{}, mutation.InputFailed{Cause: errors.New("actual tool observation unavailable")}
}

type unavailableFingerprints struct{ mutation.Fingerprints }

func (unavailableFingerprints) Tests(context.Context, string, []string) (map[string]string, mutation.InputFailure) {
	return nil, mutation.InputFailed{Cause: errors.New("actual fingerprint observation unavailable")}
}

type unavailableReach struct{ mutation.Reach }

func (unavailableReach) TestsImporting(context.Context, string, []string, []string) (map[string][]string, mutation.InputFailure) {
	return nil, mutation.InputFailed{Cause: errors.New("actual reach observation unavailable")}
}

// Run reads the disk's discovery/config through cmd's adapters here, in
// process, beside the memory driver's mutate.feature. This disposable public
// fixture reaches the exact command composition with actual memory
// observation/Workers bindings, without running a host test command.
func TestMutationCachedConsumersRequireActualInputBindings(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	m := newMemory(t)
	source := "export function flip(v: boolean): boolean { return !v; }\n"
	for name, text := range map[string]string{"package.json": `{"name":"board"}`, "board.ts": source} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := m.Write(path, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "go version", Imports: map[string][]string{"board.ts": {}}, Surviving: []features.Site{{File: "board.ts", Line: 1, Original: "!", Replacement: ""}}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	base := diskAdapters(&stderr, nil)
	inputs := m.inputs(features.Run{Dir: root})
	base.Workers = m.workers()
	base.Toolchain = inputs
	base.Fingerprints = inputs
	base.Reach = inputs
	invoke := func(a adapters, args ...string) (int, string) {
		stdout.Reset()
		stderr.Reset()
		code := 0
		out, log := captured(t, func() { code = run(t.Context(), args, strings.NewReader(""), &stdout, &stderr, a) })
		return code, out + stdout.String() + log + stderr.String()
	}
	if code, out := invoke(base, "mutation", "run", "--json", "--no-coverage", "--no-annotate", "--test-command", "go version", "board.ts"); code != 1 || !strings.Contains(out, "mutation.survived") {
		t.Fatalf("actual public setup: %d %s", code, out)
	}
	site := fmt.Sprintf("board.ts:1:%d", strings.Index(source, "!")+1)
	except := []string{"mutation", "except", site, "--reason", "reviewed equivalent", "--json"}
	if code, out := invoke(base, except...); code != 0 {
		t.Fatalf("actual exception setup: %d %s", code, out)
	}
	for _, consumer := range []struct {
		name string
		args []string
	}{
		{"check", []string{"mutation", "check", "--json", "board.ts"}}, {"except", except}, {"renew", []string{"mutation", "except", "--renew", "--json"}},
	} {
		t.Run(consumer.name, func(t *testing.T) {
			if code, out := invoke(base, consumer.args...); code != 0 {
				t.Fatalf("bound control: %d %s", code, out)
			}
			for _, binding := range []struct {
				name, token string
				change      func(*adapters)
			}{
				{"missing tools", "Toolchain binding is missing", func(a *adapters) { a.Toolchain = nil }},
				{"missing fingerprints", "Fingerprints binding is missing", func(a *adapters) { a.Fingerprints = nil }},
				{"missing reach", "Reach binding is missing", func(a *adapters) { a.Reach = nil }},
				{"wrong tools", "actual tool observation unavailable", func(a *adapters) { a.Toolchain = unavailableTools{a.Toolchain} }},
				{"wrong fingerprints", "actual fingerprint observation unavailable", func(a *adapters) { a.Fingerprints = unavailableFingerprints{a.Fingerprints} }},
				{"wrong reach", "actual reach observation unavailable", func(a *adapters) { a.Reach = unavailableReach{a.Reach} }},
			} {
				t.Run(binding.name, func(t *testing.T) {
					a := base
					binding.change(&a)
					if code, out := invoke(a, consumer.args...); code != 100 || !strings.Contains(out, binding.token) {
						t.Fatalf("binding escaped: %d %s", code, out)
					}
				})
			}
		})
	}
	// The legitimate no-work/usage returns do not demand an unused port.
	noPorts := base
	noPorts.Toolchain = nil
	noPorts.Fingerprints = nil
	noPorts.Reach = nil
	if code, out := invoke(noPorts, "mutation", "except", "--json"); code != 80 || !strings.Contains(out, "flags.value-missing") {
		t.Fatalf("usage return changed: %d %s", code, out)
	}
	if err := os.Remove(filepath.Join(root, "itos-cc.yaml")); err != nil {
		t.Fatal(err)
	}
	if code, out := invoke(noPorts, "mutation", "except", "--renew", "--json"); code != 0 {
		t.Fatalf("empty renewal required unused binding: %d %s", code, out)
	}
}
