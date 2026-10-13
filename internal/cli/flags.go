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
// line. Every flag is a switch (a bool, its --no- pair beside it, or a
// switch of choices, as choices.go says) or takes one value (a string, or a
// number, read as numbers.go says), and is given once; a flag of another
// kind is added here first. A cumulative flag (a
// slice, as kong reads one) may be given again, each time with a value of its
// own, as crap's --coverage-report is: listMapper judges each value given
// before kong's slice decoder reads it, which would take a flag after it
// as its value.
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
//
// Each refusal is a type of its own, as the kong fork's are, so the UI tells
// them apart with errors.As, never by their text: SwitchValueError,
// ValueMissingError and RepeatedFlagError. A flag's second occurrence is
// refused as given twice before its value is judged, as the mapper sees it
// already set.

import (
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
		kong.KindMapper(reflect.Float64, numberMapper{traced: traced}),
		kong.KindMapper(reflect.Int, numberMapper{traced: traced, whole: true}),
		kong.KindMapper(reflect.Slice, listMapper{kongs}),
		kong.TypeMapper(reflect.TypeFor[*Choice](), choiceMapper{traced}),
		kong.PostBuild(choicesNamed),
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
			return fmt.Errorf("--%s%s: %w", f.Name, either(f), &RepeatedFlagError{Flag: f})
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
			if value, ok := strings.CutPrefix(arg, "--no-"+f.Name+"="); ok && f.Tag.Negatable != "" && f.IsBool() {
				return fmt.Errorf("--%s: %w", f.Name, &SwitchValueError{Flag: f, Value: value})
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
		if value, written := next.Value.(string); written {
			if repeated(ctx.Value) {
				return &RepeatedFlagError{Flag: ctx.Value.Flag}
			}
			return &SwitchValueError{Flag: ctx.Value.Flag, Value: value}
		}
	}
	return m.kong.Decode(ctx, target)
}

// SwitchValueError is a switch the command line gave a value, Value, as it
// was written after the =.
type SwitchValueError struct {
	Flag  *kong.Flag
	Value string
}

func (e *SwitchValueError) Error() string {
	return fmt.Sprintf("a switch takes no value; give --%s%s alone", e.Flag.Name, either(e.Flag))
}

// valueMapper reads a flag's value as kong does, once there is one: never
// the end of the command line, nor a flag.
type valueMapper struct{ kong kong.Mapper }

func (m valueMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if err := valueGiven(ctx); err != nil {
		return err
	}
	return m.kong.Decode(ctx, target)
}

// valueGiven refuses a flag that takes a value and is given none: the
// command line ends after it, or a flag comes next. A flag given before is
// refused as given twice, before its value is judged.
func valueGiven(ctx *kong.DecodeContext) error {
	if next := ctx.Scan.Peek(); ctx.Value.Flag != nil && !next.IsValue() {
		if repeated(ctx.Value) {
			return &RepeatedFlagError{Flag: ctx.Value.Flag}
		}
		missing := &ValueMissingError{Flag: ctx.Value.Flag}
		if !next.IsEOL() {
			missing.Next = next.String()
		}
		return missing
	}
	return nil
}

// listMapper reads a cumulative flag's value, or a command's arguments, as
// kong does, once a flag has one: never the end of the command line, nor a
// flag. A flag given again adds its value, as kong's slices do.
type listMapper struct{ kongs *kong.Registry }

func (m listMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if err := valueGiven(ctx); err != nil {
		return err
	}
	return m.kongs.ForType(target.Type()).Decode(ctx, target)
}

// ValueMissingError is a flag that takes a value given none: the command
// line ended after it, or Next, a flag, came next.
type ValueMissingError struct {
	Flag *kong.Flag
	Next string
}

func (e *ValueMissingError) Error() string {
	if e.Next == "" {
		return fmt.Sprintf("needs a value, as --%s %s", e.Flag.Name, e.Flag.FormatPlaceHolder())
	}
	return fmt.Sprintf("needs a value, and %s is a flag, never its value; to give it as the value, write --%s=%s", e.Next, e.Flag.Name, e.Next)
}

// RepeatedFlagError is a flag that is not cumulative given more than once.
type RepeatedFlagError struct{ Flag *kong.Flag }

func (e *RepeatedFlagError) Error() string { return "given more than once; give it once" }

// repeated is whether v, a flag being traced, was set earlier on the
// command line and is not cumulative: given again.
func repeated(v *kong.Value) bool {
	return v.Flag != nil && v.Set && !v.IsCumulative()
}

// either is a switch's --no- pair, as a message names it beside the switch.
func either(f *kong.Flag) string {
	if f.Tag.Negatable == "" {
		return ""
	}
	return " or --no-" + f.Name
}
