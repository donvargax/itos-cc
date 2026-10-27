package features

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// quotingSteps are slice-66's: the quote a string mutation except, --renew
// or a replaced entry must quote takes, the one itos-cc.yaml already uses
// (issue #64). They read itos-cc.yaml as text, since a quote is no part of
// the value it holds.
func (e *exceptionMutation) quotingSteps(sc *godog.ScenarioContext) {
	sc.Step("^itos-cc\\.yaml quotes its strings (with double quotes|with single quotes|nowhere), as `(exclude: .*)`$", e.quoteStrings)
	sc.Step(`^a Go project with a reviewed exception for an equivalent mutant in a function, its reason "([^"]*)"$`, e.reviewRenewWith)
	sc.Step(`^the (?:new|renewed) entry's reason is written (.+)$`, func(text string) error { return e.writtenAs("reason", text) })
	sc.Step(`^its line_text is written plain, as "([^"]*)"$`, func(text string) error { return e.writtenAs("line_text", text) })
}

// quoteStrings writes line, an exclude key quoted as style says, at the
// top of itos-cc.yaml, and gives every quoted string the file already holds
// the quote of style, so the file quotes its strings one way. When the
// file holds the entry the renewal scenarios renew, its lines are the ones
// "only the lines of the renewed entry changed" lets change.
func (e *exceptionMutation) quoteStrings(style, line string) error {
	text := e.s.m.read("itos-cc.yaml")
	if text != "" {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
			return err
		}
		quote := map[string]yaml.Style{"with double quotes": yaml.DoubleQuotedStyle, "with single quotes": yaml.SingleQuotedStyle}[style]
		for _, n := range yamlNodes(&doc) {
			if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) == 0 {
				continue
			}
			if quote == 0 {
				return fmt.Errorf("itos-cc.yaml quotes %q, so it cannot quote nowhere:\n%s", n.Value, text)
			}
			n.Style = quote
		}
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(&doc); err != nil {
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}
		text = buf.String()
	}
	text = line + "\n" + text
	lines := strings.SplitAfter(text, "\n")
	for start, l := range lines {
		if l != "    - file: label.go\n" {
			continue
		}
		end := start + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "    - ") {
			end++
		}
		e.layoutPrefix, e.layoutSuffix = strings.Join(lines[:start], ""), strings.Join(lines[end:], "")
	}
	return e.s.m.w.write("itos-cc.yaml", text)
}

// reviewRenewWith is reviewRenew, the reviewed exception's reason being
// reason.
func (e *exceptionMutation) reviewRenewWith(reason string) error {
	e.renewReason = reason
	return e.reviewRenew()
}

// writtenAs holds that the one entry of mutation.exceptions whose key
// holds the value text writes is written as text, after the key and its
// colon to the end of its line.
func (e *exceptionMutation) writtenAs(key, text string) error {
	w := e.s.m.w
	if w.exit != 0 {
		return fmt.Errorf("the command failed: %s", w.report())
	}
	var want string
	if err := yaml.Unmarshal([]byte(text), &want); err != nil {
		return err
	}
	data := e.s.m.read("itos-cc.yaml")
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(data), &doc); err != nil {
		return err
	}
	var found []*yaml.Node
	for _, n := range yamlNodes(&doc) {
		if n.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key && n.Content[i+1].Kind == yaml.ScalarNode && n.Content[i+1].Value == want {
				found = append(found, n.Content[i+1])
			}
		}
	}
	if len(found) != 1 {
		return fmt.Errorf("itos-cc.yaml holds %d entries whose %s is %q, not one:\n%s", len(found), key, want, data)
	}
	lines := strings.Split(data, "\n")
	v := found[0]
	if got := strings.TrimRight(lines[v.Line-1][v.Column-1:], " \r"); got != text {
		return fmt.Errorf("the %s is written %s, not %s:\n%s", key, got, text, data)
	}
	return nil
}

// yamlNodes is n and every node it holds, in document order.
func yamlNodes(n *yaml.Node) []*yaml.Node {
	out := []*yaml.Node{n}
	for _, c := range n.Content {
		out = append(out, yamlNodes(c)...)
	}
	return out
}
