package cli

// A flag that takes a number reads its value as the hand parser read it,
// never as kong's own mappers do: strconv.ParseFloat for a number (a float64
// field), strconv.Atoi for a whole number (an int field), so 1e3, -1 and
// 010 mean what they always meant and 0x10 and 1.5 are no whole numbers.
// A value that is none is refused in the hand parser's words, as a
// NumberError, which ParseProblem tells by its type: --threshold needs a
// number, and was given "abc". A number flag given no value, or given
// twice, is refused as any flag that takes a value is (valueGiven).

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/donvargax/kong"
)

// numberMapper reads a number flag's value: a whole number with whole, any
// number otherwise. A flag the command line gives again is refused as given
// twice before its second value is judged, as the hand parser refused it;
// traced tells the command line's values from a default or an environment
// variable, which kong decodes after it.
type numberMapper struct {
	traced *tracing
	whole  bool
}

func (m numberMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	if !m.traced.over && repeated(ctx.Value) {
		return &RepeatedFlagError{Flag: ctx.Value.Flag}
	}
	if err := valueGiven(ctx); err != nil {
		return err
	}
	token, err := ctx.Scan.PopValue(m.what())
	if err != nil {
		return err
	}
	value, ok := token.Value.(string)
	if !ok {
		value = fmt.Sprint(token.Value)
	}
	if m.whole {
		n, err := strconv.Atoi(value)
		if err != nil {
			return &NumberError{Flag: ctx.Value.Flag, Value: value, Whole: true}
		}
		target.SetInt(int64(n))
		return nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return &NumberError{Flag: ctx.Value.Flag, Value: value}
	}
	target.SetFloat(f)
	return nil
}

func (m numberMapper) what() string { return NumberError{Whole: m.whole}.What() }

// NumberError is a number flag given a value, Value as it was written, that
// is not a number, or with Whole not a whole number.
type NumberError struct {
	Flag  *kong.Flag
	Value string
	Whole bool
}

// What is the kind of number the flag needs, as a person reads it.
func (e NumberError) What() string {
	if e.Whole {
		return "a whole number"
	}
	return "a number"
}

func (e *NumberError) Error() string {
	return fmt.Sprintf("needs %s, and was given %q", e.What(), e.Value)
}
