package settings

import (
	"fmt"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// Measured is what coverage is measured by in data, the text of an
// itos-cc.yaml, or nil where there is none, as a coverage input's
// fingerprint takes it (decision 64): the settings Parse reads, without
// mutation.exceptions, since an exception never changes what coverage
// measures. Every other setting counts, the version the file reads as,
// exclude, mutation.tests and mutation.copy, by the values Parse reads
// alone, so what Parse does not read counts nowhere: an x- key, at any
// depth, comments, quotes, block or flow style, indentation and the order
// of keys. A file that is not there, an empty one and one declaring
// version 1 and exceptions alone are measured alike, as each reads as
// version 1 with no setting. Text Parse refuses is its *InvalidError.
func Measured(data []byte) (string, error) {
	c, top, err := parse(data)
	if err != nil {
		return "", err
	}
	var tests Tests
	if c.Tests != nil {
		tests = *c.Tests
	}
	settings := *c
	settings.Exceptions, settings.Tests = nil, nil
	return fmt.Sprintf("version %d\n%#v\n%#v", declared(top), settings, tests), nil
}

// declared is the version a document parse accepted reads as: the one its
// top declares, else 1.
func declared(top *yaml.Node) int {
	if top != nil && top.Kind == yaml.MappingNode {
		for i := 0; i < len(top.Content); i += 2 {
			if top.Content[i].Value == "version" {
				n, _ := strconv.Atoi(top.Content[i+1].Value)
				return n
			}
		}
	}
	return 1
}
