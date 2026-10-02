// Command itos-cc measures code quality in TypeScript, Python, Kotlin, and Go.
package main

import (
	"fmt"
	"os"
)

const usage = `usage: itos-cc <command> [options] [path ...]

commands:
  crap    CRAP score per function: complexity × untested risk
  dry     functions similar enough to be duplicates
  mutate  mutation testing: do the tests notice small changes?
  units   list the functions and methods every tool measures

Paths are files, directories, or fragments of a path under the working
directory. Without paths the working directory is analyzed. Run
'itos-cc <command> -h' for a command's options.
`

var commands = map[string]func(args []string) int{
	"crap":   runCrap,
	"dry":    runDry,
	"mutate": runMutate,
	"units":  runUnits,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	switch name := os.Args[1]; name {
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		run, ok := commands[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "itos-cc: unknown command %q\n\n%s", name, usage)
			os.Exit(1)
		}
		os.Exit(run(os.Args[2:]))
	}
}
