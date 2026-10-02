// Command itos-cc prints the functions and methods of source files as JSON.
//
//	itos-cc units [path ...]
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"itos-cc/lang"
)

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "target": true,
	"build": true, "dist": true, ".venv": true, "venv": true, "__pycache__": true,
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "units" {
		fmt.Fprintln(os.Stderr, "usage: itos-cc units [path ...]")
		os.Exit(1)
	}
	roots := os.Args[2:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	units := []lang.Unit{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if lang.Detect(path) == nil {
				return nil
			}
			found, err := lang.UnitsInFile(path)
			if err != nil {
				return err
			}
			units = append(units, found...)
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(units); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
