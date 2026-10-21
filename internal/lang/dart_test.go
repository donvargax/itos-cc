package lang

import (
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// The Dart rules UNIT-16 and CPLX-07 do not reach: a unit whose signature
// and body a comment parts, the members those scenarios declare none of,
// and the null-aware and handler forms CPLX-07's function has no room for.

// dartAround holds a package demo at /lang/app, so a file's path under
// app/lib decides its library.
var dartAround = porttest.Surroundings{Dir: "/lang", Files: map[string]string{
	"/lang/app/pubspec.yaml": "# a Flutter app\nname: 'demo' # the package\nversion: 1.0.0\n",
}}

func dartFile(t *testing.T, path, src string) *File {
	t.Helper()
	f, err := Parse(t.Context(), dartAround, Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

func TestDartUnitRunsFromItsSignatureToItsBody(t *testing.T) {
	src := `/// Doubles n.
@pragma('vm:prefer-inline')
int twice(int n) // the body follows
{
  if (n > 0) {}
  return n * 2;
}
`
	f := dartFile(t, "app/lib/src/maths.dart", src)
	assertUnits(t, describe(f.Units), []string{"function demo.src.maths#twice 3-7"})
	u := f.Units[0]
	var kinds []string
	for _, n := range u.Nodes() {
		kinds = append(kinds, n.Kind())
	}
	if want := []string{"function_signature", "comment", "function_body"}; !slices.Equal(kinds, want) {
		t.Errorf("Nodes are %v, want %v", kinds, want)
	}
	start, end := u.Span()
	if got, want := string(f.Src[start:end]), src[len("/// Doubles n.\n@pragma('vm:prefer-inline')\n"):len(src)-1]; got != want {
		t.Errorf("Span is %q, want %q", got, want)
	}
	if got := f.Complexity(u); got != 2 {
		t.Errorf("complexity %d, want 2: the body's if counts", got)
	}
}

func TestDartMembers(t *testing.T) {
	src := `int get top => 1;
set top(int v) {}
external void native();

class A {
  A.new() {}
  A._internal() {}
  factory A.of() = A.new;
  static int s() => 1;
  @override bool operator ==(Object o) => true;
  int operator [](int i) => i;
  void operator []=(int i, int v) {}
  external void ext();
  Future<void> run() async {}
  Iterable<int> gen() sync* { yield 1; }
}

abstract class B {
  void m();
  int get g;
}

class P(final int x);

enum E {
  a(1);

  const E(this.v);
  final int v;
}
`
	assertUnits(t, describe(dartFile(t, "app/test/members_test.dart", src).Units), []string{
		"function demo.test.members_test#top 1-1",
		"function demo.test.members_test#top= 2-2",
		"method demo.test.members_test.A#A.new 6-6",
		"method demo.test.members_test.A#A._internal 7-7 private",
		"method demo.test.members_test.A#s 9-9",
		"method demo.test.members_test.A#operator== 10-10",
		"method demo.test.members_test.A#operator[] 11-11",
		"method demo.test.members_test.A#operator[]= 12-12",
		"method demo.test.members_test.A#run 14-14",
		"method demo.test.members_test.A#gen 15-15",
	})
}

func TestDartLibraryWithoutAPubspec(t *testing.T) {
	assertUnits(t, describe(dartFile(t, "tool/gen.dart", "void main() {}\n").Units),
		[]string{"function tool.gen#main 1-1"})
}

func TestDartComplexity(t *testing.T) {
	src := `void handlers() {
  try {} on FormatException catch (e) {} // one handler
  catch (e) {}
}

void nullAware(List<int>? l, A? a) {
  l?[0];
  a?..run()..stop();
  final x = [...?l, ...[1]];
  final m = {?a: 1};
}

int arms(Object o) => switch (o) {
  int n when n > 0 => 1,
  _ when o == 2 => 2,
  _ => 3,
};

class A {
  void run() {}
  void stop() {}
}
`
	f := dartFile(t, "app/lib/c.dart", src)
	got := map[string]int{}
	for _, u := range f.Units {
		got[u.Name] = f.Complexity(u)
	}
	assertComplexities(t, got, map[string]int{
		// on T catch is one handler, a catch alone another.
		"handlers": 3,
		// ?[, ?.. and ...? count; the map's {?a: 1} does not.
		"nullAware": 4,
		// A guarded _ arm counts; the bare _ arm and the guards do not.
		"arms": 3,
		"run":  1, "stop": 1,
	})
}

func TestDartTestFiles(t *testing.T) {
	cases := map[string]bool{
		"lib/board.dart":                    false,
		"lib/testing.dart":                  false,
		"lib/src/board_test.dart":           true,
		"test/board_test.dart":              true,
		"test/helpers/fake_board.dart":      true,
		"packages/app/test/helpers.dart":    true,
		"integration_test/app_test.dart":    true,
		"test_driver/integration_test.dart": true,
	}
	for path, want := range cases {
		if got := Detect(path).IsTest(path); got != want {
			t.Errorf("IsTest(%s) = %v, want %v", path, got, want)
		}
	}
}
