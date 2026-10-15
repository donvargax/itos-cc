package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

func TestMemoryIntegrationDeclarations(t *testing.T) {
	m := newMemory(t)
	// The logical declarations are retained now; actual request-specific report
	// parsing and collector bindings are T90/T100, not a simulated collector pass.
	features.IntegrationDeclarationContract(t, m)
	dir, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []features.Coverage{
		{Language: "typescript", Report: "SF:src/cli.ts\nDA:4,0\nend_of_record\n", IntegrationReport: "SF:src/cli.ts\nDA:4,1\nend_of_record\n"},
		{Language: "typescript", Report: "SF:src/cli.ts\nDA:4,0\nend_of_record\n", WithoutC8: true},
	} {
		if err := m.Coverage(dir, c); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(m.coverage[filepath.Clean(dir)], c) {
			t.Fatalf("declaration not retained: %+v", m.coverage)
		}
	}
}
