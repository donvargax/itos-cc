package cli

import (
	"strings"

	"github.com/donvargax/kong"
)

// Written is the flags a validated command line gave, by name, each with
// its values as written, which Provided reads. cmd binds it once for every
// command, so a Run that quotes a value back as the person wrote it asks
// for it, and converts it to its own.
type Written map[string][]string

// Provided returns only the flags on the validated Kong path, retaining
// their original value spellings. Defaults are deliberately absent: an
// explicit empty string or zero is different from an omitted flag. This
// reads provenance after Parse; Kong alone validates and decodes options.
// It supports the long flags held by Flags, whose only short form is help.
func Provided(ctx *kong.Context) Written {
	flags := map[string]*kong.Flag{}
	for _, part := range ctx.Path {
		if part.Flag != nil && !part.Resolved {
			flags[part.Flag.Name] = part.Flag
		}
	}
	values := Written{}
	for i := 0; i < len(ctx.Args); i++ {
		word := ctx.Args[i]
		if word == "--" {
			break
		}
		name, value, equals := strings.Cut(strings.TrimPrefix(word, "--"), "=")
		flag := flags[name]
		if !strings.HasPrefix(word, "--") || flag == nil {
			continue
		}
		if !equals {
			if flag.IsBool() {
				value = "true"
			} else {
				i++
				value = ctx.Args[i]
			}
		}
		values[name] = append(values[name], value)
	}
	return values
}
