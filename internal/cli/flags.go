package cli

// The flags past what kong refuses by itself: a flag given no value, a
// switch given a value and a once-only flag given twice are usage errors,
// each naming the flag in a person's words, never kong's ("expected string
// value but got \"EOL\""). kong refuses an unknown flag and an unknown
// command by itself, and reads a value of a flag as that flag's alone.
//
// It is done through kong's own hooks, never around its parser: mappers for
// the kinds the command line's flags are of, each judging what follows the
// flag before kong's own mapper reads it, and a hook before kong resets the
// values, where the flags given are known, in kong's path of the command
// line. Every flag is a switch (a bool, its --no- pair beside it) or takes
// one value (a string), and is given once; a flag of another kind is added
// here first. A cumulative flag (a slice, as kong reads one) may be given
// again, though no command has one yet.
//
// kong decodes a flag twice over: from the command line while it traces it,
// then from its environment variable when it resets the values. A switch
// takes its environment variable's value (ITOS_CC_JSON=1) and
// refuses one on the command line, so its mapper knows which it is reading
// by the hook, which runs between the two.
//
// kong (since #618) reads a --no- pair's own value itself and hands the
// mapper a bool token for the pair, the same for --no-json and
// --no-json=true. So the mapper refuses only a value the command line wrote
// (a string), and the hook refuses a --no- pair written with a value.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/donvargax/kong"
)

// Flags are the kong options that hold the command line's flags to the rules
// above, each parser given its own.
func Flags() []kong.Option {
	kongs := kong.NewRegistry().RegisterDefaults()
	traced := &tracing{}
	return []kong.Option{
		kong.KindMapper(reflect.Bool, switchMapper{traced, kongs.ForType(reflect.TypeFor[bool]())}),
		kong.KindMapper(reflect.String, valueMapper{kongs.ForType(reflect.TypeFor[string]())}),
		kong.WithBeforeReset(traced.done),
	}
}

// tracing is whether kong has traced the command line yet, so what a
// mapper reads is the command line's, not an environment variable's.
type tracing struct{ over bool }

// done ends the tracing, and refuses a flag the command line gives twice
// that is not cumulative, a switch's --no- pair counting as the switch. kong
// runs it once for each part of the path, each time judging it whole, before
// it resets the values to their defaults and reads the environment, so an
// environment variable beside its flag is never the flag given twice.
func (t *tracing) done(ctx *kong.Context) error {
	t.over = true
	if err := pairGivenAValue(ctx); err != nil {
		return err
	}
	given := map[*kong.Flag]bool{}
	for _, p := range ctx.Path {
		f := p.Flag
		if f == nil || f.IsCumulative() {
			continue
		}
		if given[f] {
			return fmt.Errorf("--%s%s: given more than once; give it once", f.Name, either(f))
		}
		given[f] = true
	}
	return nil
}

// pairGivenAValue refuses a switch's --no- pair written with a value
// (--no-json=true) before a bare --, in the command line as given, since kong
// hands the mapper the pair's value as its own bool.
func pairGivenAValue(ctx *kong.Context) error {
	for _, arg := range ctx.Args {
		if arg == "--" {
			return nil
		}
		for _, f := range ctx.Flags() {
			if f.Tag.Negatable != "" && f.IsBool() && strings.HasPrefix(arg, "--no-"+f.Name+"=") {
				return fmt.Errorf("--%s: %s", f.Name, switchTakesNoValue(f))
			}
		}
	}
	return nil
}

// switchMapper reads a switch: alone on the command line, its value from its
// environment variable as kong reads it. kong knows a switch by its bool
// kind, the only kind this mapper reads, so the mapper need not say so.
type switchMapper struct {
	traced *tracing
	kong   kong.Mapper
}

func (m switchMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if next := ctx.Scan.Peek(); !m.traced.over && next.Type == kong.FlagValueToken {
		if _, written := next.Value.(string); written {
			return errors.New(switchTakesNoValue(ctx.Value.Flag))
		}
	}
	return m.kong.Decode(ctx, target)
}

// switchTakesNoValue is the refusal of a switch given a value.
func switchTakesNoValue(f *kong.Flag) string {
	return fmt.Sprintf("a switch takes no value; give --%s%s alone", f.Name, either(f))
}

// valueMapper reads a flag's value as kong does, once there is one: never
// the end of the command line, nor a flag.
type valueMapper struct{ kong kong.Mapper }

func (m valueMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if next := ctx.Scan.Peek(); ctx.Value.Flag != nil && !next.IsValue() {
		if next.IsEOL() {
			return fmt.Errorf("needs a value, as --%s %s", ctx.Value.Name, ctx.Value.Flag.FormatPlaceHolder())
		}
		return fmt.Errorf("needs a value, and %s is a flag, never its value; to give it as the value, write --%s=%s", next, ctx.Value.Name, next)
	}
	return m.kong.Decode(ctx, target)
}

// either is a switch's --no- pair, as a message names it beside the switch.
func either(f *kong.Flag) string {
	if f.Tag.Negatable == "" {
		return ""
	}
	return " or --no-" + f.Name
}
