package cli

import (
	"reflect"
	"testing"

	"github.com/alecthomas/kong"
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

// A cumulative flag may be given again. No command has one, so a line of
// its own declares one, as kong reads a slice.
func TestACumulativeFlagMayBeGivenAgain(t *testing.T) {
	var l struct {
		Answer []string `sep:"none"`
	}
	parser, err := kong.New(&l, append(Flags(), kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"--answer", "a", "--answer=b"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Answer, []string{"a", "b"}) {
		t.Errorf("got %q", l.Answer)
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
