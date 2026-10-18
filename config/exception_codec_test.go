package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

// This literal follows the pre-extraction Exception field order/tags, including
// the required empty replacement and the optional line text.
func TestExceptionCodecPreservesYAMLRepresentation(t *testing.T) {
	e := Exception{File: "src/a.go", Function: "main#f", Hash: "hash", LineInFunction: 2, Column: 3, Original: "!", Reason: "reviewed"}
	want := "file: src/a.go\nfunction: main#f\nhash: hash\nline_in_function: 2\ncolumn: 3\noriginal: '!'\nreplacement: \"\"\nreason: reviewed\n"
	data, err := yaml.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("exception YAML %q, want %q", data, want)
	}
	e.LineText = "return !x"
	data, err = yaml.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want+"line_text: return !x\n" {
		t.Fatalf("exception YAML %q", data)
	}
}
