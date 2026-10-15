package features

import (
	"path/filepath"
	"testing"
)

// Portable assertion sensitivity, not a native Windows command execution. The
// native admission positives run in the dedicated three-OS scenario CI step.
func TestNativeAdmissionAssertionRejectsWrongObservedFacts(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{"ok": false, "problems": []any{map[string]any{"rule": "count.platform", "platform": "windows", "message": "counted execution is unavailable on windows"}}}
	}
	if err := executionAdmission(valid(), 3, "count.platform", 3, "windows"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		exit   int
		change func(map[string]any)
	}{
		{"wrong exit", 2, func(map[string]any) {}},
		{"passing verdict", 3, func(a map[string]any) { a["ok"] = true }},
		{"missing problem", 3, func(a map[string]any) { a["problems"] = []any{} }},
		{"wrong rule", 3, func(a map[string]any) { a["problems"].([]any)[0].(map[string]any)["rule"] = "some.other.rule" }},
		{"wrong platform", 3, func(a map[string]any) {
			a["problems"].([]any)[0].(map[string]any)["platform"] = "darwin"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := valid()
			c.change(a)
			if err := executionAdmission(a, c.exit, "count.platform", 3, "windows"); err == nil {
				t.Fatal("accepted deliberately wrong observed admission facts")
			}
		})
	}
}
func TestAdmissionNoWorkAssertionRejectsActualFixtureWrites(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	d := binary{}
	q := &mutationExecution{m: &mutateState{w: &world{d: d, dir: root}}, outside: outside}
	for _, name := range []string{"trace.jsonl", "list.log"} {
		if err := d.Write(filepath.Join(outside, name), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.nothingLaunched(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"trace.jsonl", "list.log"} {
		if err := d.Write(filepath.Join(outside, name), []byte("actual command wrote this\n")); err != nil {
			t.Fatal(err)
		}
		if err := q.nothingLaunched(t.Context()); err == nil {
			t.Fatalf("accepted actual %s command trace", name)
		}
		if err := d.Write(filepath.Join(outside, name), nil); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := filepath.Join(root, ".metrics", "mutate", "main.go.json")
	if err := d.Write(snapshot, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := q.nothingLaunched(t.Context()); err == nil {
		t.Fatal("accepted actual snapshot publication")
	}
	if err := d.Remove(snapshot); err != nil {
		t.Fatal(err)
	}
	q.m.w.stderr = "itos-cc: coverage go test\n"
	if err := q.nothingLaunched(t.Context()); err == nil {
		t.Fatal("accepted announced coverage execution")
	}
}
