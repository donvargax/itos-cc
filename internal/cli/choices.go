package cli

// A switch that may take one of its choices after =, as mutation run's
// --fail-uncovered[=functions|lines] does, is a *Choice whose enum tag names
// the choices, the first of them the one the switch alone takes:
//
//	FailUncovered *cli.Choice `enum:"functions,lines" help:"…"`
//
// It reads as the hand parser read it: nil when not given, its first choice
// when given alone, and the choice written after = otherwise. Its value is
// only ever written after =, so the word after it is never its value: in
// --fail-uncovered lines, lines is an argument. Any other value, an empty
// one too, is refused in the hand parser's words, as a ChoiceError, which
// ParseProblem tells by its type: --fail-uncovered takes =functions or
// =lines, and was given "x". Given twice, it is refused as given twice
// before its value is judged, as the hand parser refused it.
//
// The choices are kong's model's, the flag's enum, so help lists them
// ([=functions|lines]) and completion offers them after the =; kong's own
// check of the enum then holds what the mapper took.

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/donvargax/kong"
)

// Choice is the choice a switch of choices took, a *Choice field nil when
// the switch is not given.
type Choice string

// choiceMapper reads a switch of choices. It is a bool to kong, so kong
// and Parse never take the next word as its value; traced tells the command
// line's values from an environment variable's, which kong decodes after it.
type choiceMapper struct{ traced *tracing }

func (choiceMapper) IsBool() bool { return true }

func (m choiceMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if !m.traced.over && repeated(ctx.Value) {
		return &RepeatedFlagError{Flag: ctx.Value.Flag}
	}
	choices := ctx.Value.EnumSlice()
	value := choices[0]
	if ctx.Scan.Peek().Type == kong.FlagValueToken {
		written, ok := ctx.Scan.Pop().Value.(string)
		if !ok || !slices.Contains(choices, written) {
			return &ChoiceError{Flag: ctx.Value.Flag, Value: written}
		}
		value = written
	}
	target.Elem().SetString(value)
	return nil
}

// choicesNamed refuses a model with a switch of choices that names none,
// so the mapper always has a choice for the switch alone.
func choicesNamed(k *kong.Kong) error {
	return kong.Visit(k.Model, func(node kong.Visitable, next kong.Next) error {
		if v, ok := node.(*kong.Value); ok && v.Target.Type() == reflect.TypeFor[*Choice]() && v.Enum == "" {
			return errors.New(v.Name + " is a switch of choices and names none in its enum tag")
		}
		return next(nil)
	})
}

// ChoiceError is a switch of choices given a value, Value as it was written
// after the =, that is none of its choices.
type ChoiceError struct {
	Flag  *kong.Flag
	Value string
}

// Choices names the switch's choices as the hand parser named them: =a or =b.
func (e *ChoiceError) Choices() string {
	return "=" + strings.Join(e.Flag.EnumSlice(), " or =")
}

func (e *ChoiceError) Error() string {
	return fmt.Sprintf("takes %s, and was given %q", e.Choices(), e.Value)
}
