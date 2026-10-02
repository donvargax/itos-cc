package main

import (
	"flag"
	"fmt"
	"os"

	"itos-cc/lang"
	"itos-cc/project"
)

func runUnits(args []string) int {
	fs := flag.NewFlagSet("units", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: itos-cc units [options] [path ...]\n\nPrints the functions and methods of production code as JSON.\n\n")
		fs.PrintDefaults()
	}
	var sel selection
	sel.register(fs)
	tests := fs.Bool("tests", false, "list test code instead of production code")
	paths, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	files, err := sel.files(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	list := files.Sources
	if *tests {
		list = files.Tests
	}
	units := []lang.Unit{}
	for _, path := range list {
		found, err := lang.UnitsInFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "itos-cc:", err)
			return exitUsage
		}
		for i := range found {
			found[i].File = project.Rel(found[i].File)
		}
		units = append(units, found...)
	}
	printJSON(units)
	return exitOK
}
