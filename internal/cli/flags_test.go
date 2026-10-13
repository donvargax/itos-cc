package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// The scenarios (ID-CLI-05 to 09) hold the usage errors by their exit code
// and the flag they name. These hold what no scenario reads: a flag given no
// value, a flag's value never read as a flag, each sentence whole, a flag
// that takes a value given twice, a flag that may repeat, and a switch's
// environment variable, which one scenario sets and none reads a refusal of.

// line is a command line of every kind of flag Flags reads: a switch with
// its --no- pair and a flag that takes one value.
type line struct {
	Folder string `arg:"" optional:""`
	Stack  string `placeholder:"STACK"`
	JSON   bool   `negatable:"" env:"ITOS_CC_CLI_TEST_JSON"`
}

// parse is args parsed as line, through Flags, and the error kong gives.
func parse(t *testing.T, args ...string) (line, error) {
	t.Helper()
	var l line
	parser, err := kong.New(&l, append(Flags(), kong.Name("itos-cc"), kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parser.Parse(args)
	return l, err
}

func TestAFlagGivenNoValueIsRefusedNamingIt(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--stack"}, "--stack: needs a value, as --stack STACK"},
		{[]string{"--stack", "--json"}, "--stack: needs a value, and --json is a flag, never its value; to give it as the value, write --stack=--json"},
		{[]string{"--stack", "-h"}, "--stack: needs a value, and -h is a flag, never its value; to give it as the value, write --stack=-h"},
	}
	for _, c := range cases {
		if _, err := parse(t, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

// The kong fork types each failure of the command line, so an unknown flag
// and a stray argument tell apart by type, not by kong's text.
func TestAnUnknownFlagAndAStrayArgumentAreTypedApart(t *testing.T) {
	_, err := parse(t, "--stak")
	var unknown *kong.UnknownFlagError
	if !errors.As(err, &unknown) || unknown.Flag != "--stak" {
		t.Errorf("--stak: got %T %v, want an unknown --stak", err, err)
	}
	_, err = parse(t, "made", "more")
	var unexpected *kong.UnexpectedArgumentError
	if !errors.As(err, &unexpected) || unexpected.Arg != "more" || unexpected.ExpectedCommand {
		t.Errorf("made more: got %T %v, want a stray argument more", err, err)
	}
	if errors.As(err, &unknown) {
		t.Errorf("made more: %v is also an unknown flag", err)
	}
}

func TestAValueGivenAsAFlagsOwnIsTaken(t *testing.T) {
	l, err := parse(t, "made", "--stack=--json")
	if err != nil {
		t.Fatal(err)
	}
	if l.Folder != "made" || l.Stack != "--json" {
		t.Errorf("got %+v", l)
	}
}

func TestASwitchGivenAValueIsRefusedNamingItAndItsPair(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--json=yes"}, "--json: a switch takes no value; give --json or --no-json alone"},
		{[]string{"--no-json=false"}, "--json: a switch takes no value; give --json or --no-json alone"},
	}
	for _, c := range cases {
		if _, err := parse(t, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

// kong reads a --no- pair's value itself and hands the mapper a bool for
// the pair alone and for the pair given a value alike; the pair alone is
// taken, given a value it is refused as the switch is.
func TestANegatedSwitchGivenAValueIsRefusedAndAloneTaken(t *testing.T) {
	want := "--json: a switch takes no value; give --json or --no-json alone"
	for _, args := range [][]string{{"--no-json=true"}, {"made", "--no-json=true"}} {
		if _, err := parse(t, args...); err == nil || err.Error() != want {
			t.Errorf("%v: got %v, want %q", args, err, want)
		}
	}
	l, err := parse(t, "--no-json")
	if err != nil || l.JSON {
		t.Errorf("--no-json: got %+v, %v", l, err)
	}
	l, err = parse(t, "--", "--no-json=true")
	if err != nil || l.Folder != "--no-json=true" {
		t.Errorf("-- --no-json=true: got %+v, %v", l, err)
	}
}

func TestSwitchesGivenAloneAreTaken(t *testing.T) {
	for args, want := range map[string]bool{"--json": true, "--no-json": false} {
		l, err := parse(t, args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		if l.JSON != want {
			t.Errorf("%s: got %+v", args, l)
		}
	}
}

// Decision 28: a flag that is not cumulative is given once, a switch's --no-
// pair counting as the switch, wherever the second one stands.
func TestAOnceOnlyFlagGivenTwiceIsRefusedNamingIt(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--stack", "go", "--stack=python"}, "--stack: given more than once; give it once"},
		{[]string{"--json", "--json"}, "--json or --no-json: given more than once; give it once"},
		{[]string{"--no-json", "--json"}, "--json or --no-json: given more than once; give it once"},
		{[]string{"--json", "made", "--no-json"}, "--json or --no-json: given more than once; give it once"},
	}
	for _, c := range cases {
		if _, err := parse(t, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

// After --, a word is data, never the flag it looks like, so it is not the
// flag given twice.
func TestAFlagAfterDashDashIsNotGivenTwice(t *testing.T) {
	l, err := parse(t, "--json", "--", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !l.JSON || l.Folder != "--json" {
		t.Errorf("got %+v", l)
	}
}

// A cumulative flag may be given again, as crap's --coverage-report is,
// each time with a value of its own, a comma in it no separator under sep
// none; given none, or a flag where its value goes, it is refused as any
// flag that takes a value is.
func TestACumulativeFlagMayBeGivenAgain(t *testing.T) {
	type answers struct {
		Answer []string `sep:"none" placeholder:"A"`
		JSON   bool
	}
	parse := func(args ...string) (answers, error) {
		var l answers
		parser, err := kong.New(&l, append(Flags(), kong.Exit(func(int) { t.Fatal("kong exited") }))...)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parser.Parse(args)
		return l, err
	}
	l, err := parse("--answer", "a,b", "--answer=--json")
	if err != nil || !reflect.DeepEqual(l.Answer, []string{"a,b", "--json"}) || l.JSON {
		t.Errorf("got %+v, %v", l, err)
	}
	for args, want := range map[string]string{
		"--answer":        "--answer: needs a value, as --answer A",
		"--answer --json": "--answer: needs a value, and --json is a flag, never its value; to give it as the value, write --answer=--json",
	} {
		var missing *ValueMissingError
		if _, err := parse(strings.Fields(args)...); !errors.As(err, &missing) || err.Error() != want {
			t.Errorf("%s: got %v, want %q", args, err, want)
		}
	}
}

func TestASwitchTakesItsEnvironmentVariablesValue(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "false": false} {
		t.Setenv("ITOS_CC_CLI_TEST_JSON", value)
		l, err := parse(t)
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if l.JSON != want {
			t.Errorf("%s: got %v, want %v", value, l.JSON, want)
		}
	}
	t.Setenv("ITOS_CC_CLI_TEST_JSON", "true")
	l, err := parse(t, "--no-json")
	if err != nil || l.JSON {
		t.Errorf("--no-json over the variable: got %+v, %v", l, err)
	}
}

// A switch of no --no- pair is named alone, never with a pair that is not
// its own: the message must not offer a flag the command line cannot take.
func TestEitherNamesThePairOnlyOfANegatableSwitch(t *testing.T) {
	for _, c := range []struct {
		name string
		flag *kong.Flag
		want string
	}{
		{name: "plain", flag: flagNamed(t, "quiet", `name:"quiet"`), want: ""},
		{name: "negatable", flag: flagNamed(t, "json", `name:"json" negatable:""`), want: " or --no-json"},
	} {
		if got := either(c.flag); got != c.want {
			t.Errorf("%s: either = %q, want %q", c.name, got, c.want)
		}
	}
}

// flagNamed is the flag named name in a model declaring it with the tag, so
// the test holds either's reading of kong's own tag and not a field's place.
func flagNamed(t *testing.T, name, tag string) *kong.Flag {
	t.Helper()
	typ := reflect.StructOf([]reflect.StructField{{Name: "Switch", Type: reflect.TypeFor[bool](), Tag: reflect.StructTag(tag)}})
	parser, err := kong.New(reflect.New(typ).Interface(), Flags()...)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range parser.Model.Flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("the model declares no flag named %q", name)
	return nil
}

// Each refusal is a type of its own, so the UI tells them apart by type,
// never by their text, and names the flag and the value as written.
func TestEachRefusalIsTypedNamingItsFlag(t *testing.T) {
	_, err := parse(t, "--json=yes")
	var switchValue *SwitchValueError
	if !errors.As(err, &switchValue) || switchValue.Flag.Name != "json" || switchValue.Value != "yes" {
		t.Errorf("--json=yes: got %T %v, want a switch given yes", err, err)
	}
	_, err = parse(t, "--no-json=true")
	if !errors.As(err, &switchValue) || switchValue.Flag.Name != "json" || switchValue.Value != "true" {
		t.Errorf("--no-json=true: got %T %v, want the pair given true", err, err)
	}
	_, err = parse(t, "--stack", "--json")
	var missing *ValueMissingError
	if !errors.As(err, &missing) || missing.Flag.Name != "stack" || missing.Next != "--json" {
		t.Errorf("--stack --json: got %T %v, want --stack missing its value", err, err)
	}
	_, err = parse(t, "--json", "--json")
	var repeated *RepeatedFlagError
	if !errors.As(err, &repeated) || repeated.Flag.Name != "json" {
		t.Errorf("--json --json: got %T %v, want --json given twice", err, err)
	}
}

// A flag's second occurrence is refused as given twice before its value is
// judged: the second --json given a value, the second --stack given none.
func TestASecondOccurrenceIsRefusedAsRepeatedBeforeItsValue(t *testing.T) {
	for _, args := range [][]string{{"--json", "--json=yes"}, {"--stack", "go", "--stack"}, {"--stack", "go", "--stack", "--json"}} {
		_, err := parse(t, args...)
		var repeated *RepeatedFlagError
		if !errors.As(err, &repeated) {
			t.Errorf("%v: got %T %v, want the flag given twice", args, err, err)
		}
	}
}
