package main

import (
	"fmt"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

var unitsCommand = &command{
	name:     "units",
	summary:  "list the functions and methods every tool measures",
	synopsis: "[options] [path ...]",
	about: `
Lists the functions and methods of production code, or of test code with
--tests: the units crap, dry, and mutate measure.`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		sw("tests", "list test code instead of production code")),
	json: `"units": [{"file", "language", "namespace", "name", "kind", "private",
   "start_line", "end_line"}]`,
	exits: []exitDoc{
		{0, "success"},
		{2, "a usage error: a bad flag or path"},
		{3, "--changed outside a git repository"},
	},
	examples: []string{
		"itos-cc units src/billing",
		"itos-cc units --tests --json",
	},
	run: runUnits,
}

type unitsResult struct {
	Units []lang.Unit `json:"units"`
}

func runUnits(in *invocation) (any, error) {
	result := unitsResult{Units: []lang.Unit{}}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	list := files.Sources
	if in.set("tests") {
		list = files.Tests
	}
	for _, path := range list {
		found, err := lang.UnitsInFile(path)
		if err != nil {
			return result, err
		}
		for i := range found {
			found[i].File = project.Rel(found[i].File)
		}
		result.Units = append(result.Units, found...)
	}
	if !in.json {
		for _, u := range result.Units {
			fmt.Printf("%s:%d-%d  %s#%s  %s\n", u.File, u.StartLine, u.EndLine, u.Namespace, u.Name, u.Kind)
		}
	}
	return result, nil
}
