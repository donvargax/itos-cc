Feature: Mutation testing
  "Would the tests notice if this code were wrong?" itos-cc changes one
  operator, boolean, or 0/1 at a time inside each function and runs the tests
  that cover the file. A mutant is killed when the tests fail or time out,
  survives when they pass, and is uncovered when no test executes its line.

  Rule: Mutation sites

    @ID-MUT-01
    Scenario: TypeScript sites
      Given the file "x.ts":
        """
        const LIMIT = 1 + 2;
        function f(xs: Array<number>, a: number): boolean {
          // a < b in a comment
          const s = "x > y";
          if (a > 0 && !done) return a === 1;
          return -a <= 10 || true;
        }
        """
      When I run "itos-cc mutation list x.ts"
      Then the sites are:
        | line | original | replacement |
        | 5    | >        | >=          |
        | 5    | 0        | 1           |
        | 5    | &&       | \|\|        |
        | 5    | !        | (deleted)   |
        | 5    | ===      | !==         |
        | 5    | 1        | 0           |
        | 6    | -        | (deleted)   |
        | 6    | <=       | <           |
        | 6    | \|\|     | &&          |
        | 6    | true     | false       |
      And code outside functions, comments, strings, and type arguments hold no sites

    @ID-MUT-02
    Scenario: Nullish coalescing and optional chaining
      Given the file "x.ts":
        """
        export function place(a, b) {
          return a ?? b?.c ?? b?.[0];
        }
        """
      When I run "itos-cc mutation list x.ts"
      Then the sites are:
        | line | original | replacement |
        | 2    | ??       | \|\|        |
        | 2    | ?.       | .           |
        | 2    | ??       | \|\|        |
        | 2    | ?.       | (deleted)   |
        | 2    | 0        | 1           |
      And a?.() becomes a(), as a?.[0] becomes a[0]
      And .js, .jsx, .mjs, and .cjs files have the same sites

    @ID-MUT-03
    Scenario: Python sites
      Given the file "x.py":
        ```
        LIMIT = 1 + 2

        def f(a, b):
            """a < b"""
            if a >= 0 and not b:
                return a * 2 == 1
            return -a < b or False
        ```
      When I run "itos-cc mutation list x.py"
      Then the sites are:
        | line | original | replacement |
        | 5    | >=       | >           |
        | 5    | 0        | 1           |
        | 5    | and      | or          |
        | 5    | not      | (deleted)   |
        | 6    | *        | /           |
        | 6    | ==       | !=          |
        | 6    | 1        | 0           |
        | 7    | -        | (deleted)   |
        | 7    | <        | <=          |
        | 7    | or       | and         |
        | 7    | False    | True        |

    @ID-MUT-04
    Scenario: Go sites
      Given the file "x.go":
        """
        package p

        var limit = 1 + 2

        func f(a int, xs []int) bool {
        	var m map[string][]int
        	_ = m
        	if a != 0 || !ok(xs) {
        		return a-1 > len(xs)
        	}
        	return true
        }
        """
      When I run "itos-cc mutation list x.go"
      Then the sites are:
        | line | original | replacement |
        | 8    | !=       | ==          |
        | 8    | 0        | 1           |
        | 8    | \|\|     | &&          |
        | 8    | !        | (deleted)   |
        | 9    | -        | +           |
        | 9    | 1        | 0           |
        | 9    | >        | >=          |
        | 11   | true     | false       |

    @ID-MUT-05
    Scenario: Kotlin sites
      Given the file "x.kt":
        """
        val limit = 1 + 2

        fun f(a: Int, xs: List<Int>): Boolean {
            if (a == 0 && !xs.isEmpty()) return false
            return a * 2 >= xs.size
        }
        """
      When I run "itos-cc mutation list x.kt"
      Then the sites are:
        | line | original | replacement |
        | 4    | ==       | !=          |
        | 4    | 0        | 1           |
        | 4    | &&       | \|\|        |
        | 4    | !        | (deleted)   |
        | 4    | false    | true        |
        | 5    | *        | /           |
        | 5    | >=       | >           |

    @ID-MUT-06
    Scenario: Scanning prints sites without running tests
      When I run "itos-cc mutation list src/board.py"
      Then each site is printed as "file:line:column `original` → `replacement` in namespace#name"
      And no tests are run
      And the exit code is 0

    # mutation list listed sites function by function, so the sites of an
    # inline callback (app.get("/u", (req, res) => …)) came after every site
    # of the function around it, while mutation run --json lists a file's
    # mutants by line and column (ID-MUT-52). Both list in line and column
    # order (the coordinator's call, 2026-10-07).
    @mutation-list-line-order @ID-MUT-101
    Scenario: Sites are listed in line order, inline callbacks included
      Given src/app.ts has a function with a site before, inside and after an inline callback
      When I run "itos-cc mutation list src/app.ts"
      Then its sites are listed in line and column order
      And in the order "itos-cc mutation run --json src/app.ts" lists the file's "mutants"

  Rule: Running mutants

    @ID-MUT-07
    Scenario Outline: Each file runs its narrowest test command
      Given a source file in a <language> project with <setup>
      When its mutants run
      Then the test command is <command>

      Examples:
        | language   | setup             | command                                                          |
        | Go         | go.mod            | go test -count=1 -failfast on the file's package                 |
        | TypeScript | vitest            | the installed vitest related --run --bail=1 on the file          |
        | TypeScript | jest              | the installed jest --bail --findRelatedTests on the file         |
        | TypeScript | neither installed | <pm> run test, the package manager the project declares          |
        | Python     | pytest importable | python -m pytest -q -x -p no:cacheprovider                       |
        | Python     | no pytest         | python -m unittest discover -f                                   |
        | Kotlin     | Gradle            | gradle test --fail-fast for the module, via ./gradlew if present |
        | Kotlin     | Maven             | mvn -q test                                                      |

    @ID-MUT-08
    Scenario: By default a file is measured and mutated by its own tests
      Given a/a.go is tested only by e2e/e2e_test.go, which imports package a
      When I run "itos-cc mutation run a/a.go"
      Then coverage runs "go test -count=1 -covermode=set -coverprofile=... example.com/m/a"
      And its mutants are uncovered and none runs
      # coverage comes from the tests that kill mutants, so a line only
      # other tests reach is uncovered, never a false survivor

    @ID-MUT-09
    Scenario: The whole suite, as a nightly job
      When I run "itos-cc mutation run --all-tests a/a.go"
      Then coverage and every mutant run the whole suite of its build root: go test ./..., vitest run, jest, or gradle test
      And the e2e test kills them
      # tests that only run the built binary are not in coverage; with
      # --no-coverage too, every mutant runs and they can kill it
      And a later run without --all-tests reuses those kills for functions that have not changed while the Go module tests and support files remain unchanged

    @ID-MUT-10
    Scenario: A custom test command
      When I run "itos-cc mutation run --test-command 'make test' src/board.go"
      Then "make test" runs through the platform shell from the file's build root

    @ID-MUT-11
    Scenario: The real tree is never modified while tests run
      When I run "itos-cc mutation run src/board.ts"
      Then each worker runs mutants in its own private copy of the project
      And dependency directories such as node_modules and .venv are linked, not copied
      And the source files in the working tree are unchanged while tests run

    @ID-MUT-12
    Scenario: Python tests import the worker's copy
      Given a Python project installed in editable mode
      When its mutants run
      Then PYTHONPATH puts the worker's copy of "." and "src" first

    @ID-MUT-13
    Scenario: The baseline proves the copy works
      When mutants are about to run for a test command
      Then that command first runs unmutated inside a worker's copy
      And its duration sets the mutant timeout

    @ID-MUT-14
    Scenario: Timeouts
      Given the baseline took 3 seconds
      When I run "itos-cc mutation run --timeout-factor 10"
      Then a mutant times out after 35 seconds
      And a timed-out mutant counts as killed

    @ID-MUT-15
    Scenario: Fast suites still get the allowance
      Given the baseline took 50 milliseconds
      When mutants run with the default timeout factor of 10
      Then a mutant times out after 5.5 seconds

    # timeout-allowance: a mutant timed out after its baseline's time times
    # the timeout factor, at least 2 seconds. A mutant's run builds the
    # mutated code, which the baseline may have found built already, so on
    # a slow runner (windows-latest, CI runs 37718895582 and 37721105456) a
    # mutant that should survive timed out and counted as killed, hiding a
    # survivor. Decided with the person on 2026-10-07 (q-23): a timeout is
    # the baseline's time times the factor plus a fixed 5 seconds, as PIT and
    # Stryker add a constant, and the 2-second minimum goes. It holds for a
    # file's tests, a --test-command or --all-tests run, each selection of
    # listed tests (ID-MUT-135) and mutation sample alike. ID-MUT-14, 15 and
    # 135 said the old rule and change with the fix.
    @timeout-allowance @ID-MUT-149
    Scenario: A mutant's timeout leaves a fixed allowance beyond its baseline
      Given the baseline took 50 milliseconds
      When mutants run with the default timeout factor of 10
      Then a mutant times out after 5.5 seconds: ten times the baseline, plus 5 seconds

    @ID-MUT-16
    Scenario: A failing baseline
      Given the tests of src/board.py fail without any mutation
      When I run "itos-cc mutation run src/board.py"
      Then stdout says "src/board.py: baseline tests fail; snapshot not updated"
      And the last 20 lines of the test output follow
      And the snapshot of src/board.py is not changed
      And the problem is "mutation.baseline-failed", with file
      And the exit code is 1

    @ID-MUT-17
    Scenario: Parallel workers
      When I run "itos-cc mutation run --workers 4"
      Then up to 4 mutants run at the same time
      And the default is half the CPUs, at least 1

  Rule: Coverage decides which mutants run

    @ID-MUT-18
    Scenario: Mutants on lines no test executes are not run
      Given coverage shows line 12 of src/board.ts is never executed
      When I run "itos-cc mutation run src/board.ts"
      Then the mutants on line 12 are reported as uncovered without running

    @ID-MUT-19
    Scenario: A file the tests never load is entirely uncovered
      Given coverage measured other TypeScript files but never src/unused.ts
      When I run "itos-cc mutation run src/unused.ts"
      Then every mutant in src/unused.ts is uncovered

    # A language counted as measured only from the files of the run, and
    # coverage drops report entries that match no source, so a file no test
    # loads, mutated alone (or as the only file --since selects), printed
    # "no coverage …; running every mutant" though coverage ran for its
    # language; under --fail-uncovered its mutants then failed as survivors.
    # Seen while building slice-2, not yet reproduced with a TypeScript or
    # LCOV fixture: reproduce it first. A coverage command that ran for the
    # file's language measures that language, even when its report names no
    # file of the run (the coordinator's call, 2026-10-07). Go fixtures do
    # not show it, since go test reports a package without tests at 0%.
    @unloaded-file-uncovered @ID-MUT-105
    Scenario: A file no test loads, mutated alone, is entirely uncovered
      Given a TypeScript project whose tests import src/board.ts but never src/unused.ts
      When I run "itos-cc mutation run src/unused.ts"
      Then every mutant in src/unused.ts is uncovered, and none runs
      And stderr does not say "no coverage"
      And "itos-cc mutation run --fail-uncovered src/unused.ts" lists them as uncovered and exits 1

    @ID-MUT-20
    Scenario: No coverage for the language at all
      Given coverage measured no Kotlin file
      When I run "itos-cc mutation run src/Board.kt"
      Then stderr says "itos-cc: no coverage for src/Board.kt; running every mutant"

    @ID-MUT-21
    Scenario: Skipping coverage
      When I run "itos-cc mutation run --no-coverage"
      Then every mutant runs regardless of coverage

    @ID-MUT-22
    Scenario: Coverage is only measured when a mutant has to run
      Given every mutant can be reused from the previous snapshot
      When I run "itos-cc mutation run"
      Then no coverage or test command runs
      And stderr says "itos-cc: no mutations to test"

    # The lines the mutation commands write to stderr themselves (the
    # baseline, coverage, each mutant's outcome) began "mutate:", a command
    # that is gone since the mutation group. They begin "itos-cc:", as every
    # other line itos-cc writes to stderr does (the coordinator's call,
    # 2026-10-07). The output of the test and coverage commands themselves is
    # passed on as it is. ID-MUT-20 and ID-MUT-22 said the old text and changed
    # with the fix.
    @mutation-progress-prefix @ID-MUT-100
    Scenario: Progress lines begin itos-cc:
      Given src/board.ts has a mutant that is killed and one on a line no test executes
      When I run "itos-cc mutation run src/board.ts"
      Then the lines itos-cc writes to stderr about the baseline and each mutant begin "itos-cc: "
      And no line of stderr begins "mutate: "

  # Issue #14: Go records coverage only for code that runs inside the test
  # process, so a project tested end to end, by tests that run its built
  # binary, had every line those tests reach uncovered, and --fail-uncovered
  # failed nearly everything (itos, whose specification runs the built itos,
  # is that project). Go has had integration coverage since 1.20: a binary
  # built with go build -cover writes its coverage into the directory
  # GOCOVERDIR names, and go tool covdata textfmt turns it into an ordinary
  # profile. Decided with the person on 2026-10-07 (the issue's part 1; its
  # part 2, coverage per test, stays the idea test-attribution):
  # - When itos-cc runs a Go coverage command, it sets GOCOVERDIR to a fresh
  #   directory of the run's own under .metrics/coverage/, and after the run
  #   merges whatever data was written there into the coverage, beside go
  #   test's profile. A project opts in by having its test harness build the
  #   binary under test with -cover when GOCOVERDIR is set; itos-cc knows
  #   nothing of the harness. A project that writes nothing there behaves as
  #   before.
  # - Which tests run is unchanged: by default a file's own tests
  #   (ID-MUT-08), so an end-to-end test in another package counts under
  #   --all-tests, as its kills already do (ID-MUT-09).
  # - --json says which coverage reached each covered mutant: "coverage",
  #   the sources whose data covers its line, "in-process", "integration"
  #   or both (the coordinator's call: per mutant, the finest the issue
  #   allows, so a gate can tell).
  # - Coverage is shared, so crap's coverage counts integration data too.
  # - Found while building it: go test -cover sets each test binary's
  #   GOCOVERDIR to a directory of its own, and reads only the test binary's
  #   own data there, so a binary a test builds wrote where nothing read it.
  #   itos-cc runs each test binary itself (go test -exec), with GOCOVERDIR
  #   the run's directory; go test's own profile is unchanged.
  Rule: Coverage from tests that run the built binary

    @integration-coverage @ID-MUT-117
    Scenario: Lines a test reaches through the built binary are covered
      Given a Go module whose only test builds its binary with go build -cover when GOCOVERDIR is set, and runs it
      And the test checks one branch of the binary's output and not another
      When I run "itos-cc mutation run --all-tests --fail-uncovered"
      Then the lines the binary ran are covered, and their mutants run
      And a mutant the test notices is killed, and one it does not notice survives
      And only the mutants on lines the binary never ran are uncovered

    @integration-coverage @ID-MUT-118
    Scenario: A project whose binary writes no coverage behaves as before
      Given a Go module whose only test builds its binary without -cover and runs it
      When I run "itos-cc mutation run --all-tests --fail-uncovered"
      Then every mutant the binary's code holds is uncovered, as before
      And the exit code is 1

    @integration-coverage @ID-MUT-119
    Scenario: Which coverage reached each mutant, as JSON
      Given the module of ID-MUT-117, with one function also called by an in-process test
      When I run "itos-cc mutation run --all-tests --json"
      Then each covered mutant has "coverage", listing "in-process", "integration" or both, as the data that covers its line
      And an uncovered mutant has no "coverage"

    @integration-coverage @ID-MUT-120
    Scenario: The binary's coverage data stays with the run
      When the coverage of ID-MUT-117 runs
      Then GOCOVERDIR names a directory under the run's own run-* directory in .metrics/coverage/
      And nothing of it is left once the run ends

  # Issue #14, part 2: a project tested end to end (itos: about 505 godog
  # scenarios, one go test run of about 3 minutes) cannot afford a whole
  # suite per mutant (--all-tests). Coverage per test lets each mutant run
  # only the tests that reach its line. Decided with the person on
  # 2026-10-07 (q-17, q-18, q-20); freshness of such kills (q-19) is the
  # next slice, listed-tests-freshness.
  # - itos-cc.yaml names the tests under mutation.tests, as itos's adapters
  #   do, sharing only IDs and paths: list, a command printing one test per
  #   line, its ID and, after a tab, optionally the file that defines it;
  #   run, a command with {pattern}; ids_pattern, with {ids}; and join, with
  #   each ({id}) and sep. Optional whole, a command running every listed
  #   test, else run with every ID. Without mutation.tests nothing changes.
  # - Per-test coverage takes one run: itos-cc sets ITOS_CC_TEST_COVERDIR to
  #   a directory of the run's own, and a harness that knows it gives the
  #   processes each test starts GOCOVERDIR=<dir>/<test-id>. When nothing is
  #   written there, itos-cc runs the run command once per test, each with
  #   GOCOVERDIR of its own, as integration-coverage passes it (the
  #   coordinator's names: the variable and the per-test fallback).
  # - A mutant runs its file's own tests first; only if it survives, the
  #   listed tests whose coverage reaches its line, as one run. It is
  #   uncovered only when neither covers its line.
  # - An outcome a listed run decided records scope "listed" (q-15) and the
  #   IDs of the tests that cover its line; mutation sample re-runs it with
  #   those tests. --all-tests still runs the whole suite instead.
  # - A list command that fails is "tests.list-failed", exit 1, and nothing
  #   is judged (the coordinator's call, as a failing baseline is).
  Rule: Mutants run only the listed tests that reach them

    @listed-tests @ID-MUT-121
    Scenario: Listed tests come from itos-cc.yaml
      Given itos-cc.yaml names mutation.tests with list, run, ids_pattern and join
      And the list command prints "ID-A-01<tab>a.feature" and "ID-A-02"
      When I run "itos-cc mutation run"
      Then the list command runs once
      And the listed tests are ID-A-01, defined in a.feature, and ID-A-02, with no file

    @listed-tests @ID-MUT-122
    Scenario: Per-test coverage takes one run when the harness splits it
      Given a Go module whose harness, when ITOS_CC_TEST_COVERDIR is set, gives each test's binary GOCOVERDIR=<dir>/<test-id>
      And its tests ID-A-01 and ID-A-02 reach different lines of the binary
      When I run "itos-cc mutation run"
      Then the listed tests run once for coverage
      And each line the binary ran is covered by the IDs of the tests that reached it

    @listed-tests @ID-MUT-123
    Scenario: Without the harness's help, coverage takes one run per test
      Given the module of ID-MUT-122, whose harness ignores ITOS_CC_TEST_COVERDIR
      When I run "itos-cc mutation run"
      Then the run command runs once per listed test for coverage, each selecting that test alone
      And each line is covered by the same IDs as in ID-MUT-122

    @listed-tests @ID-MUT-124
    Scenario: A mutant runs its own tests first, and the covering tests only if it survives
      Given a mutant its file's own tests kill, and one only ID-A-02 kills
      When I run "itos-cc mutation run"
      Then the first is killed without any listed test running for it
      And the second runs its own tests, survives them, then runs the run command selecting ID-A-02 alone, and is killed

    @listed-tests @ID-MUT-125
    Scenario: Uncovered means neither the own tests nor a listed test reach the line
      Given a line only ID-A-01 reaches, and a line no test reaches
      When I run "itos-cc mutation run --fail-uncovered"
      Then the first line's mutants run and are not uncovered
      And only the second line's mutants are uncovered

    @listed-tests @ID-MUT-126
    Scenario: An outcome decided by listed tests records them
      Given a mutant only ID-A-02 kills
      When I run "itos-cc mutation run --json"
      Then its snapshot entry records scope "listed" and tests ["ID-A-02"]
      And its mutant in --json has "tests": ["ID-A-02"]

    @listed-tests @ID-MUT-127
    Scenario: mutation sample re-runs a listed outcome with its tests
      Given a fresh outcome recorded with scope "listed" and tests ["ID-A-02"]
      When I run "itos-cc mutation sample --count 100"
      Then that mutant runs the run command selecting ID-A-02, after its own tests survive it
      And no "mutation.mismatch" is reported

    @listed-tests @ID-MUT-128
    Scenario: Without mutation.tests nothing changes
      Given itos-cc.yaml has no mutation.tests
      When I run "itos-cc mutation run"
      Then no list command runs, and ITOS_CC_TEST_COVERDIR is not set
      And the outcomes and the snapshot are as before

    @listed-tests @ID-MUT-129
    Scenario: A list command that fails judges nothing
      Given the list command exits 1
      When I run "itos-cc mutation run"
      Then the problem is "tests.list-failed", with its exit code
      And no mutant runs, no snapshot is written, and the exit code is 1

  # A kill made by listed tests (listed-tests) depends on those tests, which
  # import nothing of the code, so the tests' hash of ID-MUT-65 never sees
  # them change. Decided with the person on 2026-10-07: such a kill is
  # fresh while the files its covering tests name are unchanged (q-19), and
  # while the support files are: mutation.tests.support, globs such as
  # features/*_test.go, where the checks the tests run live (q-21, raised by
  # a side agent after q-19). The snapshot records, for each listed
  # outcome, the hashes of those files and of every support file; mutation
  # check, run, sample and the graph compare them through
  # mutate.FreshnessOf, and run no list command to do it. Deleting a test
  # from its file changes that file, so its kills go stale too. One
  # support edit stales every listed kill, and each then reruns only its
  # covering tests. Own-scope outcomes keep today's rule.
  Rule: A kill made by listed tests goes stale when its tests change

    @listed-kill-freshness @ID-MUT-130
    Scenario: A listed kill stays fresh while its tests' files and the support files are unchanged
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And nothing has changed since
      When I run "itos-cc mutation check"
      Then the exit code is 0
      And "itos-cc mutation run" reuses that kill

    @listed-kill-freshness @ID-MUT-131
    Scenario: Editing the file of a covering test makes its kill stale
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And a.feature has changed since
      When I run "itos-cc mutation check"
      Then the problem is "mutation.stale" for that mutant's function, and its message names a.feature
      And "itos-cc mutation run" runs that mutant again, its own tests first

    @listed-kill-freshness @ID-MUT-132
    Scenario: Editing a file no covering test names changes nothing
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And b.feature, which defines only tests that do not cover it, has changed since
      When I run "itos-cc mutation check"
      Then the exit code is 0

    @listed-kill-freshness @ID-MUT-133
    Scenario: Editing a support file makes every listed kill stale
      Given itos-cc.yaml lists features/*_test.go under mutation.tests.support
      And a run recorded listed kills and kills by own tests
      And features/steps_test.go has changed since
      When I run "itos-cc mutation check"
      Then each function holding a listed kill is "mutation.stale", its message naming features/steps_test.go
      And the functions whose kills were all by their own tests stay fresh

    @listed-kill-freshness @ID-MUT-134
    Scenario: The graph agrees
      Given a.feature has changed since a run recorded a kill by ID-A-02
      When the graph is built
      Then the function holding that kill is marked stale, as mutation check calls it

  # listed-tests timed every run of listed tests from one run of all of
  # them: the coverage run (or the sum of the per-test runs), times the
  # timeout factor. For itos, about 3 minutes times 10, so a mutant that
  # hangs a selection of one or two scenarios waited about 30 minutes, and
  # the only proof that the listed tests pass without a mutant was all of
  # them passing together. Found by a side agent before v0.5.0; the fix is
  # the coordinator's call (2026-10-07): each selection of listed tests,
  # the first time a mutant needs it, runs once without a mutant in the
  # worker's copy. Its time, times the timeout factor plus 5 seconds
  # (timeout-allowance, ID-MUT-149), is that selection's timeout (as
  # ID-MUT-14 and 15 for a file's tests), and a selection that fails without a mutant decides none of the
  # mutants that would run it, as a failing baseline decides none of its
  # file's. Selections are cached by their set of IDs for the run; mutation
  # sample follows the same rule.
  Rule: Each selection of listed tests has its own baseline

    @listed-selection-baseline @ID-MUT-135
    Scenario: A selection's timeout comes from its own run
      Given listed tests ID-A-01, which runs in about a second alone, and ID-A-02, which takes about ten
      And a mutant only ID-A-01 reaches makes the binary hang
      When I run "itos-cc mutation run --timeout-factor 3"
      Then ID-A-01 runs once alone without a mutant before that mutant's run
      And the mutant times out after three times that run plus 5 seconds, not after three times both tests' time
      And its outcome is "timeout", counted killed

    @listed-selection-baseline @ID-MUT-136
    Scenario: A selection is run without a mutant once per run
      Given two mutants whose lines only ID-A-02 reaches, both surviving their own tests
      When I run "itos-cc mutation run"
      Then ID-A-02 runs alone without a mutant exactly once

    @listed-selection-baseline @ID-MUT-137
    Scenario: A selection that fails without a mutant decides none of its mutants
      Given ID-A-02 fails when it runs alone without a mutant, though the listed tests pass together
      When I run "itos-cc mutation run"
      Then the problem is "tests.selection-failed", with the IDs of the selection
      And no mutant that would run ID-A-02 is recorded killed, and its file's snapshot keeps what it held
      And the exit code is 1

  # Issue #9, part 2: mutants on lines no test executes are not run, so a
  # changed function with no test passes. A gate passes --fail-uncovered
  # (with --since for a task's commits) to make each uncovered mutant a
  # failure. An explicit flag, not implied by --since, so a gate mode (#8)
  # can turn it on later and plain runs keep today's output. Uncovered
  # mutants are never reused from a snapshot, so each run decides them
  # afresh from coverage. Without --fail-uncovered, --no-coverage still
  # runs every mutant. ADR-0017 makes --no-coverage conflict with strict
  # mode; strict Go also requires independent executable-block evidence,
  # even where no mutation site exists (strict-go-coverage below).
  Rule: Uncovered mutants as failures

    @slice-2 @ID-MUT-47
    Scenario: An uncovered mutant fails the run
      Given coverage shows line 12 of src/board.ts is never executed
      And every mutant on the lines the tests execute is killed
      When I run "itos-cc mutation run --fail-uncovered src/board.ts"
      Then each mutant on line 12 is listed as "uncovered src/board.ts:12:9 `>` → `>=` in board#place"
      And the exit code is 1

    @slice-2 @ID-MUT-48
    Scenario: A file the tests never load fails whole
      Given coverage measured other TypeScript files but never src/unused.ts
      When I run "itos-cc mutation run --fail-uncovered src/unused.ts"
      Then every mutant in src/unused.ts is listed as uncovered
      And the exit code is 1

    @slice-2 @ID-MUT-49
    Scenario: With --since only the judged functions' uncovered mutants fail
      Given a commit after "base" changed "Board#place", which no test executes
      And "Board#clear", unchanged, is not executed by any test either
      When I run "itos-cc mutation run --since base --fail-uncovered"
      Then the mutants of "Board#place" are listed as uncovered
      And none of "Board#clear" is
      And the exit code is 1

    @slice-2 @ID-MUT-50
    Scenario: Uncovered mutants as JSON
      When I run "itos-cc mutation run --fail-uncovered --json src/board.ts"
      Then each uncovered mutant is a "mutation.uncovered" problem with file, line, column, function, original, and replacement
      And "ok" is false

    # ADR-0017 deliberately replaces the former promise that combining
    # --no-coverage with --fail-uncovered silently runs every mutant.
    @slice-2 @strict-go-coverage @ID-MUT-51
    Scenario: Skipping coverage conflicts with strict mode
      When I run "itos-cc mutation run --no-coverage --fail-uncovered src/board.ts"
      Then the problem is "flags.conflict", naming "--no-coverage"
      And no coverage command or mutant runs
      And the exit code is 2

  Rule: Differential runs

    @ID-MUT-23
    Scenario: Killed mutants of unchanged functions stay killed
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since
      When I run "itos-cc mutation run"
      Then those mutants are reused without running

    @ID-MUT-24
    Scenario: Changed functions rerun
      Given "Board#place" changed since the previous run
      When I run "itos-cc mutation run"
      Then every mutant of "Board#place" runs again

    @ID-MUT-25
    Scenario: Moving a function does not count as a change
      Given an import was added above "f" so it moved down the file
      When I run "itos-cc mutation run"
      Then the killed mutants of "f" are still reused

    @ID-MUT-26
    Scenario: Survivors are always retried
      Given a mutant of "Board#place" survived the previous run
      When I run "itos-cc mutation run"
      Then it runs again, since new tests may kill it
      # unless itos-cc.yaml excepts it: an excepted survivor is reused as a
      # kill is, while its function and its tests are unchanged (Rule:
      # Equivalent mutants excepted, each with a reason)

    @ID-MUT-27
    Scenario: Forcing a full rerun
      When I run "itos-cc mutation run --mutate-all"
      Then killed mutants of unchanged functions run again too

  # Units sharing a namespace#name in one file (several Go init functions,
  # Kotlin or TypeScript overloads) were told apart three ways: mutation run
  # kept only the last one's hash, so only it reused its outcomes, and the
  # --since judge and the entries it keeps keyed by name alone; mutation
  # check matched any entry of the name with the same hash; the graph paired
  # them by position. All of them match an entry by name and hash, as check
  # did (the coordinator's call, 2026-10-07, on the person's go-ahead to fix
  # mutate-same-name-units and graph-stale-same-name together). A unit is
  # fresh when an entry of its name has its hash, so reordering such units
  # changes nothing and editing one makes only that one stale; units with
  # the same name and the same hash pair with their entries in file order.
  Rule: Functions sharing a name

    @same-name-units @ID-MUT-114
    Scenario: Functions sharing a name each reuse their own results
      Given src/setup.go declares two init functions with different bodies, each with a mutant the tests kill
      And a previous run recorded both
      When I run "itos-cc mutation run src/setup.go" with nothing changed
      Then the mutants of both init functions are reused, and none runs

    @same-name-units @ID-MUT-115
    Scenario: Editing one of two functions sharing a name reruns only it
      Given a previous run recorded both init functions of src/setup.go, all killed
      And the second init function has changed since
      When I run "itos-cc mutation check src/setup.go"
      Then the only problem is "mutation.stale", for the second init function
      And "itos-cc mutation run src/setup.go" runs only the second init function's mutants

    @same-name-units @ID-MUT-116
    Scenario: Reordering functions sharing a name changes nothing
      Given a previous run recorded both init functions of src/setup.go, all killed
      And the two have swapped places since, unchanged
      When I run "itos-cc mutation check src/setup.go"
      Then the exit code is 0
      And "itos-cc mutation run src/setup.go" reuses every mutant

  # Issue #9, part 1: a gate judges a task's commits, not the working tree,
  # so --since <ref> picks the functions the commits since <ref> changed.
  # The range is git diff <ref>...HEAD: the branch's own commits whatever its
  # base did since, never uncommitted changes. A deleted line counts as a
  # change of the lines either side of it. Paths are relative to the working
  # directory, so from a subdirectory only the changes under it count.
  # Uncovered mutants failing (#9 part 2) and each mutant in --json (#9
  # part 3) are separate items. Decided with the person on 2026-10-06:
  # --since refuses --changed, paths narrow the range, and the functions
  # judged are counted in the summary and named in --json. The started code
  # (mutate.Options.Judge, project.ChangedSince) follows these already.
  Rule: Judging the functions a range of commits changed

    @slice-1 @ID-MUT-37
    Scenario: Only the functions the commits since a ref changed are judged
      Given a commit after "base" changed lines of "Board#place" in src/board.ts
      And "Board#clear" in the same file did not change
      When I run "itos-cc mutation run --since base"
      Then the mutants of "Board#place" run
      And no mutant of "Board#clear" runs
      And stdout says "src/board.ts: … (judged 1 of 2 functions)"

    @slice-1 @ID-MUT-38
    Scenario: Functions not judged keep what their snapshot holds
      Given a previous run recorded a survivor in "Board#clear"
      And only "Board#place" changed since "base"
      When I run "itos-cc mutation run --since base"
      Then the snapshot of src/board.ts still records that survivor in "Board#clear"
      And it is not reported, and the exit code is 0 when every mutant of "Board#place" is killed
      # a function never judged before gets no entry: it neither ran nor has an outcome to keep

    @slice-1 @ID-MUT-39
    Scenario: Uncommitted changes are not in the range
      Given "Board#place" changed since "base" in a commit
      And "Board#clear" has an uncommitted change
      When I run "itos-cc mutation run --since base"
      Then "Board#place" is judged
      And "Board#clear" is not

    @slice-1 @ID-MUT-40
    Scenario: Deleting lines changes the function around them
      Given a commit after "base" only deleted a line inside "Board#place"
      When I run "itos-cc mutation run --since base"
      Then "Board#place" is judged

    @slice-1 @ID-MUT-41
    Scenario: Paths narrow the range
      Given commits after "base" changed src/board.ts and lib/util.ts
      When I run "itos-cc mutation run --since base src"
      Then only src/board.ts is mutated

    @slice-1 @ID-MUT-42
    Scenario: Nothing changed since the ref
      Given no commit after "base" changed a source file
      When I run "itos-cc mutation run --since base"
      Then stderr says "itos-cc: no source files to mutate"
      And the exit code is 0

    @slice-1 @ID-MUT-43
    Scenario: A ref git cannot resolve is a usage error
      When I run "itos-cc mutation run --since nosuch"
      Then stderr says "itos-cc: --since nosuch: not a commit in this repository"
      And the problem is "since.bad-ref", with ref "nosuch"
      And the exit code is 2

    @slice-1 @ID-MUT-44
    Scenario: --since outside a git repository is a missing environment
      Given the working directory is not a git repository
      When I run "itos-cc mutation run --since main"
      Then the problem is "since.no-git"
      And the exit code is 3

    @slice-1 @ID-MUT-45
    Scenario: --since and --changed are not combined
      When I run "itos-cc mutation run --since main --changed"
      Then the problem is "flags.conflict", with flag "--changed"
      And the exit code is 2
      # --changed judges whole files of the working tree, --since functions of
      # commits: together they would judge neither

    @slice-1 @ID-MUT-46
    Scenario: The functions judged, as JSON
      When I run "itos-cc mutation run --since base --json"
      Then each file in "files" has "judged", the namespace#name of each function judged
      And without --since no file has "judged"

    # A file the range changed only outside its functions (its imports) is
    # selected with "judged 0 of N", and its snapshot and summary comment
    # were written though nothing in it ran. Nothing is written for it (the
    # coordinator's call, 2026-10-07). The summary comment counting the whole
    # snapshot while stdout counts what was judged is by design: the comment
    # is the file's state, stdout the run's.
    @since-untouched-files @ID-MUT-102
    Scenario: A file with nothing judged is left as it was
      Given a commit after "base" changed only the imports of src/board.ts
      When I run "itos-cc mutation run --since base"
      Then stdout says "src/board.ts: … (judged 0 of 2 functions)"
      And neither .metrics/mutate/src/board.ts.json nor src/board.ts is written

    # git diff -U0 prints no hunk for a file renamed without a change, so
    # none of its functions was judged and its snapshot stayed at the old
    # path while the new one had none: mutation check then called every
    # function missing. --since follows renames: a renamed file's snapshot
    # moves with it, and only the functions the range changed are judged, as
    # a move is no change (the coordinator's call, 2026-10-07). The scenarios
    # rename a Go file within its package, so no test's import changes and
    # the results stay fresh.
    @since-follows-renames @ID-MUT-103
    Scenario: A renamed file's results follow it
      Given fresh results for every function of src/board.go, all killed
      And a commit after "base" renamed src/board.go to src/grid.go without changing it
      When I run "itos-cc mutation run --since base"
      Then no function is judged
      And .metrics/mutate/src/grid.go.json holds the results .metrics/mutate/src/board.go.json held, under its new path
      And .metrics/mutate/src/board.go.json is gone
      And "itos-cc mutation check src/grid.go" exits 0

    @since-follows-renames @ID-MUT-104
    Scenario: A renamed and edited file judges only what changed
      Given fresh results for every function of src/board.go, all killed
      And a commit after "base" renamed src/board.go to src/grid.go and changed "Board#Place"
      When I run "itos-cc mutation run --since base"
      Then only "Board#Place" is judged
      And the other functions' results are kept in .metrics/mutate/src/grid.go.json

  # Issue #8, part 1: a gate (a commit hook) proves that results exist for
  # exactly the code being committed, without running anything. Decided with
  # the person on 2026-10-06 (q-1 to q-3): it is a subcommand of the group,
  # mutation check, and it gives the verdict a full run would give, from the
  # snapshots alone. For each selected function it compares the function's
  # hash with its entry in .metrics/mutate/: no entry is missing, another
  # hash is stale (a move is not a change, as for reuse), and a fresh entry
  # fails on a recorded survivor, or on a recorded uncovered mutant with
  # --fail-uncovered. Without strict Go statement coverage, a function with
  # no mutation site needs no entry. ADR-0017 adds strict Go obligations. It
  # takes the selection of mutation run: paths, --changed, and --since, which
  # checks only the functions the range changed. Results going stale when
  # the tests change (#8 part 2) and re-running a sample in CI (#8 part 3)
  # are their own items.
  Rule: Checking cached results without running

    @mutation-check @ID-MUT-57
    Scenario: Fresh results with every mutant killed pass
      Given a run recorded every mutant of src/board.ts killed
      And no function of src/board.ts has changed since
      When I run "itos-cc mutation check src/board.ts"
      Then no coverage or test command runs
      And the exit code is 0

    @mutation-check @ID-MUT-58
    Scenario: A function changed since its results is stale
      Given "Board#place" changed since the run that recorded it
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.stale", with file "src/board.ts" and function "Board#place"
      And the exit code is 1

    @mutation-check @ID-MUT-59
    Scenario: A function with no results is missing
      Given "Board#reset" was added after the last run
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.missing", with file "src/board.ts" and function "Board#reset"
      And the exit code is 1
      # a file with no snapshot at all has every function with a site missing

    @mutation-check @ID-MUT-60
    Scenario: A recorded survivor fails the check
      Given a fresh run recorded a survivor in "Board#place"
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.survived", with its file, line, column, function, original, and replacement
      And the exit code is 1

    @mutation-check @ID-MUT-61
    Scenario: A recorded uncovered mutant fails only with --fail-uncovered
      Given a fresh run recorded every covered mutant of src/board.ts killed and one uncovered
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0
      But "itos-cc mutation check --fail-uncovered src/board.ts" reports it as "mutation.uncovered" and exits 1

    @mutation-check @ID-MUT-62
    Scenario: With --since only the functions the range changed are checked
      Given only "Board#place" changed since "base", and its fresh results are all killed
      And "Board#clear", unchanged, has a recorded survivor
      When I run "itos-cc mutation check --since base"
      Then the exit code is 0

    @mutation-check @ID-MUT-63
    Scenario: A function with no mutation site needs no results
      Given "Board#size" has no mutation site and no entry in the snapshot
      And every other function of src/board.ts has fresh results, all killed
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0

    @mutation-check @ID-MUT-64
    Scenario: Each function's state as JSON
      When I run "itos-cc mutation check --json src/board.ts"
      Then each file in "files" has "functions", each with function and state "fresh", "stale" or "missing"
      And the problems are those the plain output prints

    # A fresh entry was judged only by the mutants it records, so when a
    # newer itos-cc adds a mutation operator, a function whose text did not
    # change has sites its entry never recorded, and mutation check passed
    # it although no run judged them. Decided with the person on 2026-10-07
    # (q-13): such a function is stale, and the problem names the sites; a
    # mutation run runs only those sites and reuses the recorded outcomes.
    # The fixtures stand for a new operator by removing a mutant from the
    # entry.
    @check-unrecorded-sites @ID-MUT-106
    Scenario: A site the entry never recorded makes the function stale
      Given fresh results for every function of src/board.ts, all killed
      But the entry of "Board#place" does not record one of its sites, as when a newer itos-cc adds an operator
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.stale", with file and function "Board#place", and its message names that site's line, column, original and replacement
      And the exit code is 1

    @check-unrecorded-sites @ID-MUT-107
    Scenario: mutation run runs only the sites the entry never recorded
      Given the entry of "Board#place" does not record one of its sites, and records every other one killed
      When I run "itos-cc mutation run src/board.ts"
      Then only that site's mutant runs
      And the other mutants of "Board#place" are reused
      And afterwards "itos-cc mutation check src/board.ts" exits 0

  # Issue #8, part 2, and IDEAS.md "Killed mutants must notice test
  # changes": a result was keyed by its function's hash alone, which covers
  # the function's own source, so deleting the test that killed a mutant
  # changed nothing and its kill was reused. Decided with the person on
  # 2026-10-06 (q-4, q-5): each file's snapshot also records the hash of
  # every test file the graph finds importing it (for Go, its package's test
  # files and the test files of packages that import it). A kill is reused,
  # and a function is fresh for mutation check, only while both its own
  # hash and those tests' hashes match. A snapshot written before this
  # records no tests and is stale. These are the baseline dependencies;
  # listed outcomes also follow ADR-0013, and Go all-tests and test-command
  # outcomes also follow ADR-0016 and the all-tests-freshness scenarios.
  # A test that does not import the file leaves an own-scope outcome fresh.
  Rule: Results go stale when their tests change

    @mutation-test-hash @ID-MUT-65
    Scenario: The snapshot records the tests that import the file
      Given src/board.test.ts imports src/board.ts and src/other.test.ts does not
      When I run "itos-cc mutation run src/board.ts"
      Then .metrics/mutate/src/board.ts.json records the hash of src/board.test.ts
      And not of src/other.test.ts

    @mutation-test-hash @ID-MUT-66
    Scenario: Changing a test that imports the file reruns its kills
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since, but src/board.test.ts has
      When I run "itos-cc mutation run src/board.ts"
      Then every mutant of "Board#place" runs again

    @mutation-test-hash @ID-MUT-67
    Scenario: Deleting a test that imports the file reruns its kills
      Given a previous run killed every mutant of "Board#place"
      And src/board.test.ts was deleted since
      When I run "itos-cc mutation run src/board.ts"
      Then no mutant of "Board#place" is reused

    @mutation-test-hash @ID-MUT-68
    Scenario: mutation check calls results stale when their tests changed
      Given fresh results for every function of src/board.ts, all killed
      And src/board.test.ts changed since they were recorded
      When I run "itos-cc mutation check src/board.ts"
      Then each function of src/board.ts is a "mutation.stale" problem whose message names src/board.test.ts
      And the exit code is 1

    @mutation-test-hash @ID-MUT-69
    Scenario: A snapshot that records no tests is stale
      Given .metrics/mutate/src/board.ts.json was written before snapshots recorded tests
      And no function of src/board.ts has changed since
      When I run "itos-cc mutation check src/board.ts"
      Then each function of src/board.ts is a "mutation.stale" problem
      And "itos-cc mutation run src/board.ts" reuses none of its kills

    @mutation-test-hash @ID-MUT-70
    Scenario: A Go file's tests are its package's and those of packages that import it
      Given src/board.go is in package board, tested by src/board_test.go
      And package app imports board and is tested by app/app_test.go
      When I run "itos-cc mutation run src/board.go"
      Then .metrics/mutate/src/board.go.json records the hashes of src/board_test.go and app/app_test.go

    # A snapshot records the tests that import its file once per file, so
    # when they changed, mutation run --since, which rewrites the file's
    # snapshot with the new tests' hashes, could not keep the old outcomes of
    # the functions it did not judge without calling them fresh: it dropped
    # their entries, check then called them missing, and any survivor they
    # recorded was lost. Decided with the person on 2026-10-07 (q-14): it
    # keeps those entries, marked stale, so check calls them stale and their
    # outcomes are kept until a run judges them again.
    @since-keeps-stale-entries @ID-MUT-108
    Scenario: --since keeps the entries it does not judge, marked stale, when their tests changed
      Given fresh results for "Board#place" and "Board#clear", with a survivor recorded in "Board#clear"
      And a commit after "base" changed "Board#place" and src/board.test.ts
      When I run "itos-cc mutation run --since base"
      Then .metrics/mutate/src/board.ts.json still holds the entry of "Board#clear" and its survivor, marked stale
      And "itos-cc mutation check src/board.ts" reports "Board#clear" as "mutation.stale", not "mutation.missing"
      And a later "itos-cc mutation run src/board.ts" runs the mutants of "Board#clear" again

  # Issue #8, part 3: mutation check trusts the snapshots, so a cache written
  # by hand, or against other code, passes it. CI re-runs a few cached
  # mutants and compares. Decided with the person on 2026-10-07 (q-6 to
  # q-8): it is a subcommand of the group, mutation sample, and it writes
  # nothing, neither snapshot nor summary comment. It samples only the fresh
  # entries of its selection (stale and missing ones are mutation check's to
  # report), and among them only mutants that ran: killed, timeout or
  # survived, never uncovered, so no coverage runs. A recorded survivor that
  # still survives is no failure here: failing on survivors is mutation
  # check's. --count N draws N of them from the whole selection, 20 by
  # default, all when there are fewer. The draw is seeded with the HEAD
  # commit's id, so a rerun of one commit samples the same mutants and each
  # new commit others; --seed <text> overrides it to reproduce a run. Any
  # outcome that differs from the snapshot fails as mutation.mismatch,
  # naming both, whichever way it went: the cache was wrong either way.
  # Killed and timeout agree (the coordinator's call, from rule MUT-14: both
  # mean the tests noticed, and a slower machine turns one into the other).
  # It takes the selection of mutation check: paths, --changed and --since.
  Rule: Re-running a sample of cached results

    @mutation-sample @ID-MUT-71
    Scenario: A sample whose outcomes hold passes and writes nothing
      Given fresh results for every function of src/board.ts, with killed mutants and one survivor
      And the tests still kill and miss the same mutants
      When I run "itos-cc mutation sample src/board.ts"
      Then the baseline runs, then each sampled mutant, in a worker's copy
      And no coverage command runs
      And .metrics/mutate/src/board.ts.json and src/board.ts are unchanged
      And the exit code is 0

    @mutation-sample @ID-MUT-72
    Scenario: A recorded kill that now survives is a mismatch
      Given the snapshot records the mutant `>` → `>=` at src/board.ts:5:9 in "Board#place" as killed
      But the tests no longer kill it
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.mismatch", with file, line, column, function, original, replacement, recorded "killed" and outcome "survived"
      And the exit code is 1

    @mutation-sample @ID-MUT-73
    Scenario: A recorded survivor that is now killed is a mismatch too
      Given the snapshot records a mutant of "Board#place" as survived
      But the tests now kill it
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.mismatch", with recorded "survived" and outcome "killed"
      And the exit code is 1

    @mutation-sample @ID-MUT-74
    Scenario: Killed and timeout agree
      Given the snapshot records a mutant of "Board#place" as killed
      And it now runs past its timeout
      When I run "itos-cc mutation sample src/board.ts"
      Then no problem is reported
      And the exit code is 0
      # and a recorded timeout that is now killed agrees too

    @mutation-sample @ID-MUT-75
    Scenario: Only fresh mutants that ran are sampled
      Given "Board#place" has fresh results, with one mutant recorded uncovered
      And "Board#clear" changed since its results, and "Board#reset" has none
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then only the killed, timeout and survived mutants of "Board#place" run
      And no problem names "Board#clear" or "Board#reset"
      And the exit code is 0

    @mutation-sample @ID-MUT-76
    Scenario: --count says how many, 20 by default
      Given the selection holds 50 fresh mutants that ran, across several files
      When I run "itos-cc mutation sample"
      Then 20 of them run
      And "itos-cc mutation sample --count 5" runs 5
      And "itos-cc mutation sample --count 80" runs all 50

    @mutation-sample @ID-MUT-77
    Scenario: A rerun of one commit samples the same mutants
      Given the selection holds 50 fresh mutants that ran
      When I run "itos-cc mutation sample" twice at the same HEAD commit
      Then both runs sample the same mutants
      And stdout names the seed, the HEAD commit's id

    @mutation-sample @ID-MUT-78
    Scenario: --seed reproduces a run
      Given a run printed the seed "4813e48"
      And HEAD has moved since, with the same snapshots
      When I run "itos-cc mutation sample --seed 4813e48"
      Then it samples the mutants that run sampled

    @mutation-sample @ID-MUT-79
    Scenario: Outside a git repository the seed must be given
      Given the working directory is not a git repository
      When I run "itos-cc mutation sample"
      Then the problem is "sample.no-git"
      And the exit code is 3
      But "itos-cc mutation sample --seed 1" runs

    @mutation-sample @ID-MUT-80
    Scenario: A count below 1 is a usage error
      When I run "itos-cc mutation sample --count 0"
      Then the problem is "flags.value-invalid", with flag "--count" and value "0"
      And the exit code is 2

    @mutation-sample @ID-MUT-81
    Scenario: With --since only the functions the range changed are sampled
      Given only "Board#place" changed since "base", and a run since recorded its results
      And "Board#clear", unchanged, has fresh results too
      When I run "itos-cc mutation sample --since base --count 100"
      Then only mutants of "Board#place" run

    @mutation-sample @ID-MUT-82
    Scenario: Nothing to sample
      Given no fresh mutant that ran in the selection
      When I run "itos-cc mutation sample"
      Then stderr says "itos-cc: no cached mutant to sample"
      And no test command runs
      And the exit code is 0
      # stale or missing results are mutation check's to fail

    @mutation-sample @ID-MUT-83
    Scenario: A failing baseline
      Given the tests of src/board.ts fail without any mutation
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.baseline-failed", with file
      And none of its mutants runs
      And the exit code is 1

    # Since q-15 each sampled mutant also has "scope", the scope of the
    # tests it ran with, a key added with sample-recorded-scope.
    @mutation-sample @ID-MUT-84
    Scenario: The sample as JSON
      When I run "itos-cc mutation sample --json src/board.ts"
      Then stdout is one object with "schema": 1, "ok", "seed", and "files"
      And each file has "mutants", each sampled one with line, column, function, original, replacement, recorded, outcome, and scope
      And the problems are those the plain output prints

    # A snapshot did not record which tests decided its outcomes, so
    # mutation sample, re-running with the file's own tests unless told
    # otherwise, read a kill only an end-to-end test makes under --all-tests
    # as a survivor: a mutation.mismatch that was no cache error. Decided
    # with the person on 2026-10-07 (q-15): each recorded outcome keeps its
    # scope, "own" (the file's own tests), "all-tests", or the
    # --test-command line, and a reused outcome keeps the scope it was
    # decided with. mutation sample re-runs each mutant with its recorded
    # scope; --all-tests or --test-command given to sample override it for
    # every mutant. An outcome with no recorded scope, written before this,
    # is taken as "own" (the coordinator's call: it is what a run without
    # those flags used).
    @sample-recorded-scope @ID-MUT-109
    Scenario: Each recorded outcome keeps the scope of the tests that decided it
      When I run "itos-cc mutation run --all-tests src/board.ts"
      Then each mutant in .metrics/mutate/src/board.ts.json records scope "all-tests"
      And with --test-command 'make test' each records "make test", and with neither each records "own"

    @sample-recorded-scope @ID-MUT-110
    Scenario: A reused outcome keeps its scope
      Given a run with --all-tests killed every mutant of "Board#place"
      And nothing has changed since
      When I run "itos-cc mutation run src/board.ts"
      Then the mutants of "Board#place" are reused and still record scope "all-tests"

    @sample-recorded-scope @ID-MUT-111
    Scenario: mutation sample re-runs each mutant with its recorded scope
      Given a run with --all-tests recorded a kill in src/board.ts that only an end-to-end test makes
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then that mutant runs the whole suite and is killed
      And no "mutation.mismatch" is reported

    @sample-recorded-scope @ID-MUT-112
    Scenario: A scope given to mutation sample overrides the recorded one
      Given outcomes of src/board.ts recorded with scope "all-tests"
      When I run "itos-cc mutation sample --test-command 'make test' src/board.ts"
      Then every sampled mutant runs "make test"

    @sample-recorded-scope @ID-MUT-113
    Scenario: An outcome with no recorded scope was decided by the file's own tests
      Given fresh results for src/board.ts written before outcomes recorded their scope
      When I run "itos-cc mutation sample src/board.ts"
      Then each sampled mutant runs the file's own tests

    # mismatch-fix-scope: since sample-recorded-scope each mutant is sampled
    # in a scope, but a mutation.mismatch's fix still said "itos-cc mutation
    # run --mutate-all <file>" with neither flag, so following it on an
    # all-tests outcome re-recorded it under the file's own tests, and a kill
    # only the whole suite makes became a survivor. Decided by the
    # coordinator on 2026-10-07, the person agreeing to the item:
    # - The fix names the scope the sampled mutant ran with (its recorded
    #   one, or the one given to sample): "own" and "listed" add no flag, as
    #   a run without one uses the file's own tests and then the listed
    #   tests; "all-tests" adds --all-tests; a --test-command line adds
    #   --test-command with the line in single quotes, a quote in it written
    #   '\''.
    # - The problem carries the scope as "scope", as the sample's JSON does.
    @mismatch-fix-scope @ID-MUT-147
    Scenario Outline: A mismatch's fix re-runs the mutant in the scope it was sampled with
      Given a run <recorded with> recorded a kill in src/board.ts that those tests no longer make
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then the problem is "mutation.mismatch", with scope "<scope>"
      And its fix says to run "<fix>"

      Examples:
        | recorded with                   | scope     | fix                                                                     |
        | without flags                   | own       | itos-cc mutation run --mutate-all src/board.ts                          |
        | with --all-tests                | all-tests | itos-cc mutation run --mutate-all --all-tests src/board.ts              |
        | with --test-command 'make test' | make test | itos-cc mutation run --mutate-all --test-command 'make test' src/board.ts |

    @mismatch-fix-scope @ID-MUT-148
    Scenario: A scope given to sample is the one the fix names
      Given outcomes of src/board.ts recorded with scope "own", one of them a kill the whole suite no longer makes
      When I run "itos-cc mutation sample --all-tests --count 100 src/board.ts"
      Then the problem is "mutation.mismatch", with scope "all-tests"
      And its fix says to run "itos-cc mutation run --mutate-all --all-tests src/board.ts"

  Rule: Results

    @ID-MUT-28
    Scenario: Summary per file
      When I run "itos-cc mutation run src/board.ts"
      Then stdout says "src/board.ts: 14 killed, 1 survived, 2 uncovered (ran 9, reused 8)"
      And each survivor is listed as "survived src/board.ts:5:9 `>` → `>=` in board#place"

    @ID-MUT-29
    Scenario Outline: Exit codes
      Given <situation>
      When I run "itos-cc mutation run"
      Then the exit code is <code>

      Examples:
        | situation                     | code |
        | every covered mutant is killed | 0    |
        | a baseline fails              | 1    |
        | a mutant survives             | 1    |

    @ID-MUT-30
    Scenario: Survivors as JSON
      When I run "itos-cc mutation run --json src/board.ts"
      Then stdout is one object with "schema": 1, "ok", and "files", each with file, killed, survived, uncovered, ran, reused, and baseline
      And each survivor is a "mutation.survived" problem with file, line, column, function, original, and replacement

    # Issue #9, part 3: a gate reads mutation run's result without parsing text or
    # the cache files, so --json lists every mutant it decided. Each file's
    # "mutants" are in site order, with the keys mutation list gives a site less
    # "file" (its file holds them), plus "outcome" and "reused": "reused" is
    # true for an outcome taken from the snapshot without running, which a
    # gate that trusts cached kills (#8) needs to tell apart. A new key, so
    # "schema" stays 1. With --since only the judged functions' mutants are
    # listed, as only they are counted. Since q-15 each mutant also has
    # "scope", the scope of the tests that decided it, a key added with
    # sample-recorded-scope.
    @slice-3 @ID-MUT-52
    Scenario: Every mutant as JSON
      Given src/board.ts has mutants that are killed, one that survives, and one on a line no test executes
      When I run "itos-cc mutation run --json src/board.ts"
      Then its file in "files" has "mutants", one for each site in site order
      And each has line, column, function, original, replacement, outcome, reused, and scope
      And their outcomes are "killed", "survived", and "uncovered" as each was decided

    @slice-3 @ID-MUT-53
    Scenario: A timed-out mutant's outcome is its own
      Given a mutant of src/board.ts runs past its timeout
      When I run "itos-cc mutation run --json src/board.ts"
      Then its outcome is "timeout"
      And it is counted in "killed"

    @slice-3 @ID-MUT-54
    Scenario: Mutants taken from the snapshot say so
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since, while "Board#clear" has
      When I run "itos-cc mutation run --json src/board.ts"
      Then the mutants of "Board#place" are "killed" with reused true
      And the mutants of "Board#clear" have reused false

    @slice-3 @ID-MUT-55
    Scenario: With --since only the judged functions' mutants are listed
      Given only "Board#place" changed since "base"
      When I run "itos-cc mutation run --since base --json"
      Then the "mutants" of src/board.ts all have function "Board#place"

    @slice-3 @ID-MUT-56
    Scenario: A failing baseline lists no mutant
      Given the tests of src/board.ts fail without any mutation
      When I run "itos-cc mutation run --json src/board.ts"
      Then its file has "baseline": "failed" and "mutants": []

    @ID-MUT-31
    Scenario: Results are cached per file
      When I run "itos-cc mutation run src/billing/invoice.ts"
      Then .metrics/mutate/src/billing/invoice.ts.json records each function's hash and the outcome of every mutant

    @ID-MUT-32
    Scenario: A summary comment at the end of each source file
      Given the file "x.py":
        """
        def a(x):
            return x > 0
        """
      When its mutants run and "> → >=" survives while one other mutant is killed
      Then the file ends with:
        """
        # itos-cc mutation: 1 killed, 1 survived, 0 uncovered
        # survived: line 2 `>` → `>=` in a
        # end itos-cc mutation
        """

    @ID-MUT-33
    Scenario: The summary comment is replaced, not repeated
      Given a file that already ends with an itos-cc mutation comment
      When its mutants run again
      Then the file holds exactly one, updated summary comment
      And the file is only written when the summary changed

    @ID-MUT-34
    Scenario: A comment block followed by code is not ours
      Given an itos-cc mutation comment block followed by more code
      When the summary is rewritten
      Then that block is left in place

    @ID-MUT-35
    Scenario: Turning the summary comment off
      When I run "itos-cc mutation run --no-annotate"
      Then no source file is modified

    # annotate-marker: the summary comment opened "itos-cc mutate:" and
    # closed "end itos-cc mutate", naming the mutate command that became
    # mutation run. Decided by the coordinator on 2026-10-07: it opens
    # "itos-cc mutation:" and closes "end itos-cc mutation", and a run still
    # finds a comment with the old markers, so a file annotated before is
    # rewritten with one comment, never two. ID-MUT-32 and the scenarios of
    # annotate-excepted quote the old text and change with the fix.
    @annotate-marker @ID-MUT-145
    Scenario: A summary comment with the old markers is replaced by one with the new
      Given the file "x.py" ends with a summary comment opening "# itos-cc mutate:" and closing "# end itos-cc mutate"
      When its mutants run again
      Then the file holds exactly one summary comment
      And it opens "# itos-cc mutation:" and closes "# end itos-cc mutation"

    @ID-MUT-36
    Scenario: Nothing to mutate
      Given the selection holds no production source files
      When I run "itos-cc mutation run"
      Then stderr says "itos-cc: no source files to mutate"
      And the exit code is 0

  # Issue #10: an equivalent mutant, one that changes no behaviour (`<` to
  # `!=` on a loop that only counts up, a `0` never read), survives every
  # run, so a gate on survivors blocks forever, and the next change contorts
  # the code or tests an implementation detail to kill it. Decided with the
  # person on 2026-10-07 (q-9 to q-12):
  # - The exceptions live in itos-cc.yaml at the project root, under
  #   mutation.exceptions: itos-cc's first project setting (docs/CLI.md
  #   rule 38), reviewed like any change to the code.
  # - itos-cc mutation except <file>:<line>:<column> --reason '…' writes an
  #   entry for a survivor its fresh snapshot records: the file, the
  #   function, the function's hash, the site's place within the function
  #   (its line counted from the function's first, so a move keeps it), the
  #   original, the replacement and the reason. Excepting a site again
  #   replaces its entry; the file's other keys and comments are kept.
  # - An excepted survivor fails nothing: it is counted "excepted", not
  #   "survived". It is reused without running, as a kill is, while its
  #   function and the tests that import its file are unchanged; when its
  #   tests change it runs again, and if it is now killed its entry is
  #   stale. An entry whose function changed, or whose site is gone, is
  #   stale without running anything, and its mutant is judged as if it
  #   had no entry. A stale entry fails as mutation.exception-stale until
  #   it is removed or the site excepted again. --mutate-all runs excepted
  #   mutants too.
  # - Only survivors: an uncovered mutant needs a test, not a reason.
  # mutation check gives the same verdict from the snapshots. mutation
  # sample needs nothing new: an excepted survivor still survives, which
  # matches what its snapshot records.
  Rule: Equivalent mutants excepted, each with a reason

    @mutation-exceptions @ID-MUT-85
    Scenario: Excepting a survivor records it with its reason
      Given a fresh run recorded the survivor `<` → `!=` at src/board.ts:7:19 in "Board#count"
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'the loop only counts up'"
      Then itos-cc.yaml has one entry under mutation.exceptions
      And it records src/board.ts, "Board#count", the function's hash, the site's line within the function, its column, `<`, `!=`, and the reason
      And the exit code is 0

    @mutation-exceptions @ID-MUT-86
    Scenario: Only a recorded survivor can be excepted
      Given the snapshot records the mutant at src/board.ts:5:9 as killed, and the one at src/board.ts:9:3 as uncovered
      When I run "itos-cc mutation except src/board.ts:5:9 --reason 'x'"
      Then the problem is "exception.no-survivor", with file, line and column
      And the exit code is 2
      And excepting src/board.ts:9:3 is refused the same way

    @mutation-exceptions @ID-MUT-87
    Scenario: A reason is required
      When I run "itos-cc mutation except src/board.ts:7:19"
      Then the problem is "flags.value-missing", with flag "--reason"
      And the exit code is 2
      And "--reason ''" is "flags.value-invalid", exit 2

    @mutation-exceptions @ID-MUT-88
    Scenario: The rest of itos-cc.yaml is kept
      Given itos-cc.yaml holds a comment, another key, and an entry for src/board.ts:7:19
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'reworded'"
      Then the entry's reason is "reworded", and it is still the only one for that site
      And the comment and the other key are unchanged

    @mutation-exceptions @ID-MUT-89
    Scenario: An excepted survivor fails nothing
      Given itos-cc.yaml excepts the survivor of "Board#count", and every other mutant of src/board.ts is killed
      When I run "itos-cc mutation run src/board.ts"
      Then stdout says "src/board.ts: 14 killed, 0 survived, 1 excepted, 0 uncovered (ran 15, reused 0)"
      And the exit code is 0

    @mutation-exceptions @ID-MUT-90
    Scenario: An excepted survivor is reused while nothing changed
      Given a run recorded the excepted survivor of "Board#count"
      And neither "Board#count" nor the tests that import src/board.ts have changed since
      When I run "itos-cc mutation run src/board.ts"
      Then the excepted mutant does not run
      And it is counted "excepted"
      But "itos-cc mutation run --mutate-all src/board.ts" runs it

    @mutation-exceptions @ID-MUT-91
    Scenario: After its tests change, an excepted mutant that still survives stays excepted
      Given a run recorded the excepted survivor of "Board#count"
      And src/board.test.ts changed since, and still does not kill it
      When I run "itos-cc mutation run src/board.ts"
      Then the excepted mutant runs
      And it is counted "excepted"
      And the exit code is 0

    @mutation-exceptions @ID-MUT-92
    Scenario: An excepted mutant that is now killed makes its entry stale
      Given a run recorded the excepted survivor of "Board#count"
      And src/board.test.ts changed since, and now kills it
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with file, function, line, column, original, replacement and why "killed"
      And the exit code is 1

    @mutation-exceptions @ID-MUT-93
    Scenario: An entry whose function changed is stale
      Given itos-cc.yaml excepts a survivor of "Board#count"
      And "Board#count" changed since the entry was written
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with why "changed"
      And the mutant at its site, if it survives, is a "mutation.survived" problem too
      And the exit code is 1
      # moving the function is not a change, as for reuse

    @mutation-exceptions @ID-MUT-94
    Scenario: An entry whose site is gone is stale
      Given itos-cc.yaml excepts a survivor in "Board#count"
      And "Board#count" was deleted, or no longer has that site
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with why "gone"
      And the exit code is 1

    @mutation-exceptions @ID-MUT-95
    Scenario: An entry does not excuse an uncovered mutant
      Given itos-cc.yaml holds an entry for a site whose mutant is uncovered
      When I run "itos-cc mutation run --fail-uncovered src/board.ts"
      Then the problem is "mutation.uncovered" for that site
      And the exit code is 1

    @mutation-exceptions @ID-MUT-96
    Scenario: mutation check passes an excepted survivor and fails a stale entry
      Given fresh results for src/board.ts with one survivor, which itos-cc.yaml excepts
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0
      But after "Board#count" changes, mutation check reports "mutation.exception-stale" and exits 1

    @mutation-exceptions @ID-MUT-97
    Scenario: With --since only the judged functions' entries are checked
      Given only "Board#place" changed since "base"
      And itos-cc.yaml holds an entry for "Board#count" that would be stale
      When I run "itos-cc mutation run --since base"
      Then no "mutation.exception-stale" problem is reported

    @mutation-exceptions @ID-MUT-98
    Scenario: An invalid itos-cc.yaml is a config error
      Given itos-cc.yaml holds an entry with no reason
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "config.invalid", with file "itos-cc.yaml"
      And the exit code is 2

    @mutation-exceptions @ID-MUT-99
    Scenario: Excepted mutants as JSON
      When I run "itos-cc mutation run --json src/board.ts" with one excepted survivor
      Then its file has "excepted": 1 beside "killed", "survived" and "uncovered"
      And that mutant has outcome "survived" and "excepted", the entry's reason

    # annotate-excepted: the summary comment at the end of a source
    # file is built from the snapshot alone, which records an excepted
    # survivor as survived, so the comment said "1 survived" and listed it
    # while mutation run's summary counted it excepted and failed nothing.
    # Decided on 2026-10-07:
    # - The comment counts "excepted" apart, only where there is one, as the
    #   summary line does, and lists each excepted survivor after the
    #   survivors, with its reason.
    # - A survivor is excepted in the comment when itos-cc.yaml holds an
    #   entry for its site that holds: the same function hash, the same site.
    #   That needs no run, so a --since run gives the functions it does not
    #   judge the same comment as a full run. A stale entry excepts nothing:
    #   its survivor is listed as survived.
    # - The comment's text is otherwise unchanged; its marker is
    #   mutation-annotate-marker's.
    # - A reason is written on one line, each run of whitespace one space, so
    #   the comment never puts uncommented text in a source file.
    @annotate-excepted @ID-MUT-138
    Scenario: The summary comment counts an excepted survivor apart, with its reason
      Given itos-cc.yaml excepts the survivor `<` → `!=` at src/board.ts:7:19 in "Board#count" with reason "the loop only counts up", and every other mutant of src/board.ts is killed
      When I run "itos-cc mutation run src/board.ts"
      Then src/board.ts ends with:
        """
        // itos-cc mutation: 14 killed, 0 survived, 1 excepted, 0 uncovered
        // excepted: line 7 `<` → `!=` in count: the loop only counts up
        // end itos-cc mutation
        """

    @annotate-excepted @ID-MUT-139
    Scenario: A stale entry's survivor is listed as survived
      Given itos-cc.yaml excepts a survivor of "Board#count"
      And "Board#count" changed since the entry was written, and the mutant at its site still survives
      When I run "itos-cc mutation run src/board.ts"
      Then the summary comment of src/board.ts counts it as survived and lists it on a "survived:" line
      And the comment holds no "excepted" count and no "excepted:" line

    @annotate-excepted @ID-MUT-140
    Scenario: A reason over several lines is written on one comment line
      Given itos-cc.yaml excepts the survivor of "Board#count" with reason "the loop\n  only counts up"
      When I run "itos-cc mutation run src/board.ts"
      Then the summary comment of src/board.ts has the line "// excepted: line 7 `<` → `!=` in count: the loop only counts up"
      And every line of the comment begins with "//"

    # exceptions-deleted-file: mutation run and mutation check judge the
    # exceptions of the files they select, so an entry whose function or
    # site is gone fails as stale, but one whose whole file was deleted or
    # renamed is never selected and stays in itos-cc.yaml unnoticed.
    # Decided with the person on 2026-10-07 (q-22):
    # - A run or check of the whole project (no paths, no --changed, no
    #   --since) judges each entry whose file is not a source it selects.
    # - With --since, so does a run whose range deleted or renamed the
    #   entry's file: a gate judges a task with --since, and the task that
    #   removed the file is the one to fix its entries.
    # - Such an entry has moved when exactly one selected source holds a
    #   function of the entry's name and hash: it still excepts the mutant at
    #   its site there, so that mutant is no survivor, and it fails as
    #   mutation.exception-stale with why "moved" and the new file, the fix
    #   saying to change the entry's file to it. Otherwise it fails as why
    #   "gone", as an entry whose function is gone fails: no line.
    # - Only mutation except writes itos-cc.yaml: a run never moves an entry.
    # - With paths or --changed, an entry for a file outside the selection
    #   is not judged, as now.
    @exceptions-deleted-file @ID-MUT-141
    Scenario: A run of the whole project fails an entry whose file is gone
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And src/board.ts and its tests were deleted
      When I run "itos-cc mutation run"
      Then the problem is "mutation.exception-stale", with file "src/board.ts", function "Board#count", no line, and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-142
    Scenario: mutation check of the whole project fails it too
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And fresh results for every source file, src/board.ts having been deleted
      When I run "itos-cc mutation check"
      Then the problem is "mutation.exception-stale", with file "src/board.ts" and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-143
    Scenario: With --since, an entry whose file the range deleted is gone
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And a commit after "base" deleted src/board.ts and its tests
      When I run "itos-cc mutation run --since base"
      Then the problem is "mutation.exception-stale", with file "src/board.ts" and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-144
    Scenario: An entry whose file was renamed has moved, and still excepts its mutant
      Given itos-cc.yaml excepts the survivor `<` → `!=` at src/board.ts:7:19 in "Board#count"
      And a commit after "base" renamed src/board.ts to src/grid.ts, changing nothing else
      When I run "itos-cc mutation run --since base"
      Then the problem is "mutation.exception-stale", with file "src/board.ts", why "moved", and new file "src/grid.ts"
      And its fix says to change the entry's file to src/grid.ts
      And there is no "mutation.survived" problem
      And itos-cc.yaml still names src/board.ts
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-146
    Scenario: A run of the whole project finds a renamed file's entry moved
      Given itos-cc.yaml excepts the survivor of "Board#count" in src/board.ts
      And src/board.ts was renamed to src/grid.ts, changing nothing else
      When I run "itos-cc mutation run"
      Then the problem is "mutation.exception-stale", with why "moved" and new file "src/grid.ts"
      And there is no "mutation.survived" problem

  # all-tests-freshness, issue #20, q-24 and ADR-0016 (2026-10-08):
  # - Importer hashes remain every outcome's baseline dependency.
  # - Each Go outcome recorded with scope all-tests or test-command also
  #   depends on every _test.go file beneath its source's nearest Go module
  #   root, excluding nested modules but including build-tagged tests.
  #   Discovery must not silently omit a module's test files merely because
  #   they live outside packages that import the source.
  # - Both broad scopes also depend on project-root mutation.tests.support
  #   matches, using the existing filepath.Glob semantics. Feature files and
  #   other custom-command inputs require support globs; arbitrary external
  #   inputs are not inferred. Own/listed scopes and other languages retain
  #   their existing rules. No list command is needed to check freshness.
  # - These dependencies apply to every broad-scope outcome, including
  #   survivors and excepted survivors, not just kills. A changed, added or
  #   deleted dependency invalidates the outcome; check names the file.
  # - Freshness follows the recorded scope even when a later invocation
  #   uses another scope. Reuse retains that scope and its evidence. Partial
  #   runs never refresh evidence for outcomes they did not rerun.
  # - Legacy broad-scope outcomes lacking this evidence are stale, even
  #   when their source and importer hashes still match, and must rerun.
  # - Run, check, sample and the graph share the same freshness verdict.
  Rule: Go whole-suite outcomes notice changes to their test inputs

    @all-tests-freshness @ID-MUT-150
    Scenario Outline: A binary-only test change makes a whole-suite kill stale
      Given a Go module with a production function and a separate end-to-end _test.go file that runs its built binary without importing its package
      And a mutation run with <scope> recorded a mutant killed only by that end-to-end test
      And only that test file changed since, so it no longer kills the mutant
      When I run "itos-cc mutation check --json" for the production file
      Then its function is stale, the problem is "mutation.stale", and its message names the changed end-to-end test file
      And the exit code is 1
      When I run mutation run with <scope> for the production file
      Then that mutant runs again rather than reusing its recorded kill
      And it survives

      Examples:
        | scope                         |
        | --all-tests                   |
        | --test-command "go test -count=1 ./..." |

    @all-tests-freshness @ID-MUT-151
    Scenario Outline: Whole-suite outcomes notice support files added, changed or removed
      Given a Go whole-suite outcome recorded with <scope> and support globs matching a feature file
      And a support file was <change> since that outcome was recorded
      When I run "itos-cc mutation check --json" for its production file
      Then the function is stale and its "mutation.stale" message names the support file
      And no test or list command runs
      And the exit code is 1

      Examples:
        | scope        | change  |
        | all-tests    | added   |
        | all-tests    | changed |
        | all-tests    | removed |
        | test-command | added   |
        | test-command | changed |
        | test-command | removed |

    @all-tests-freshness @ID-MUT-152
    Scenario: Unchanged broad-scope evidence permits a plain run to reuse a kill
      Given Go kills recorded with all-tests and test-command scopes and current module-test and support hashes
      And none of the source, importer tests, module tests or support files changed
      When I run mutation run without --all-tests or --test-command
      Then those kills are reused
      And each retains its recorded scope and freshness evidence
      And mutation check still reports them fresh

    @all-tests-freshness @ID-MUT-153
    Scenario: Mixed scopes keep their own freshness dependencies
      Given Go mutation outcomes recorded with own, listed, all-tests and test-command scopes
      And importer tests, listed covering tests and listed support files are unchanged
      And only a non-importing module _test.go file changed
      When their freshness is checked
      Then the own and listed outcomes remain usable
      And every all-tests and test-command outcome is stale, including a survivor and an excepted survivor
      And mutation check and the graph agree on each affected function's freshness

    @all-tests-freshness @ID-MUT-154
    Scenario: A partial run cannot bless unjudged whole-suite outcomes
      Given a Go file with two functions whose outcomes were recorded with whole-suite scopes
      And a non-importing module test changed since both outcomes were recorded
      And only one function changed since "base"
      When I run "itos-cc mutation run --since base" for that file
      Then the judged function's outcomes are rerun
      And the unjudged function's outcomes remain stale
      And mutation check reports the unjudged function stale
      And mutation sample excludes its stale outcomes
      And the graph marks the unjudged function stale too

    @all-tests-freshness @ID-MUT-155
    Scenario: Module boundaries and legacy evidence are explicit
      Given a Go broad-scope kill whose source and importer hashes match but whose snapshot records no whole-suite freshness evidence
      When I run mutation check for its production file
      Then its function is stale and mutation run must rerun that outcome before it can be reused
      Given a fresh broad-scope outcome in the outer Go module
      When a build-tagged _test.go file is added beneath that module
      Then that outcome is stale
      But adding a _test.go file only beneath a nested Go module leaves that outcome fresh

  # strict-go-coverage, issue #23, q-25/q-26 and ADR-0017/0018:
  # - --fail-uncovered judges positive-weight Go executable coverage blocks
  #   in every selected, judged function, including functions with no sites.
  #   Empty/comment-only bodies have no executable obligation; prove that
  #   from source, not from the absence of a file's coverage records.
  # - Different measured column spans on one line remain distinct. Matching
  #   spans from successful built-in/integration/listed measurements are
  #   covered if any measured copy executed them. Function literals belong
  #   to their enclosing named Go function, as today's Go unit model does.
  #   Comments, braces and zero-weight records add no separate obligations.
  # - Persist a complete per-function block inventory and its independent
  #   provenance, including a measured-complete inventory. Missing, null,
  #   partial or legacy evidence is not proof of complete coverage.
  # - Evidence belongs to its recorded coverage producer, not to mutant
  #   scopes: --test-command alone does not change the coverage command.
  #   Fingerprint producer/options, relevant Go build environment, module
  #   .go source/test files (build-tagged files included, nested modules
  #   excluded), go.mod/go.sum and project config/support matches. Compare
  #   additions, removals and edits; ignore only the tool-owned annotation
  #   block to avoid making a run's own summary stale its evidence.
  # - Go raw-profile workflows (--coverage-report, --use-existing-coverage
  #   and --coverage-command) conflict with strict mode in this slice. They
  #   retain non-strict behavior. Do not attach today's fingerprints to an
  #   old, failed or unrelated measurement. Opaque inputs require support
  #   globs and are not inferred. No new attestation/sidecar format.
  # - q-27/ADR-0019 refuse an active Go workspace and a local replacement
  #   outside the actual inventoried source module in strict run/check.
  #   An excluded nested module is outside that inventory even if its path
  #   is lexically inside the source tree. Do not follow external trees to
  #   make the scope look supported. Existing evidence cannot bypass the
  #   refusal. GOWORK=off and inputs already in the inventory stay supported;
  #   non-strict commands retain their existing behavior. Report refusal as
  #   mutation.coverage-unsupported (exit 1), with file/function/line, naming
  #   the workspace or replacement responsible in its message.
  # - A fresh independent cache may avoid coverage measurement, including
  #   evidence from a broader recorded suite. Missing/stale evidence makes
  #   a strict run measure coverage even with no pending mutation sites.
  #   Non-strict/no-coverage runs must not manufacture or refresh evidence.
  # - check runs no tests/coverage/list command and writes nothing. Strict
  #   check reports mutation.coverage-missing or mutation.coverage-stale
  #   with file/function/line and exits 1 when it cannot trust the evidence.
  #   The stale message names changed inputs. A run that cannot establish
  #   complete evidence reports missing evidence, never invents covered or
  #   uncovered blocks; actual measurement errors retain CLI exit categories.
  # - Each uncovered span is mutation.uncovered-statement (exit 1), with
  #   current file, function and line. Optional column precision must not be
  #   lost internally. These findings have no original/replacement and do
  #   not change mutant counts. Keep mutation.uncovered for actual mutants,
  #   even when that also produces a statement finding. Survivor exceptions
  #   do not excuse uncovered statements. No new coverage exceptions.
  # - --since/paths keep today's judgment selection. Preserve original
  #   evidence for unjudged functions, never bless it with newly measured
  #   file-wide fingerprints. Pair by existing name/hash identity (including
  #   repeated init functions); diagnose current positions after movement.
  #   File/package moves that change measurement inputs stale coverage.
  # - Non-strict zero-site/reuse behavior, mutant counts, ordinary graph and
  #   sample freshness, survivor exceptions and other-language coverage
  #   behavior remain unchanged. The --no-coverage/--fail-uncovered usage
  #   conflict is universal, and is checked even for empty selections.
  Rule: Strict Go checks prove executable coverage without mutation sites

    @strict-go-coverage @ID-MUT-156
    Scenario: A killed mutant does not excuse an uncovered statement without a site
      Given a Go function with a tested comparison whose mutants are killed and an unexecuted identifier-conditioned branch returning a string with no mutation site
      When I run mutation run with --fail-uncovered --json for its source file
      Then the problem is "mutation.uncovered-statement", with file, function and the unexecuted block's line
      And no original or replacement is invented for that problem
      And the function's mutant counts remain killed mutants only, with zero uncovered mutants
      And the exit code is 1
      When I run mutation check with --fail-uncovered --json for the file
      Then it reports the same uncovered statement and exits 1 without running commands or writing files

    @strict-go-coverage @ID-MUT-157
    Scenario: Executable zero-site functions need coverage while empty bodies do not
      Given a Go function containing no mutation sites whose identifier-conditioned branch is never executed
      And an executable zero-site function in a package no test loads
      When I run mutation run with --fail-uncovered for each source
      Then each unexecuted executable block is an uncovered-statement failure
      But a strict run and cached check of a fully executed zero-site function both pass
      And empty or comment-only function bodies add no coverage obligation
      And a non-strict check of a zero-site function still needs no snapshot entry

    @strict-go-coverage @ID-MUT-158
    Scenario: Covered blocks cannot hide unexecuted same-line or closure blocks
      Given a Go function has covered and uncovered positive-weight blocks with distinct column spans on the same source line
      And another named Go function contains a never-called function literal
      When I run mutation run with --fail-uncovered
      Then the same-line uncovered span fails despite the covered span on that line
      And the literal's uncovered block is attributed to its enclosing named function
      And comments, brace-only lines and zero-statement-weight blocks create no separate findings

    @strict-go-coverage @ID-MUT-159
    Scenario: Reused mutants cannot bypass missing or stale coverage evidence
      Given a non-strict no-coverage run recorded all mutants killed and no independent statement evidence
      When I run mutation run with --fail-uncovered and fresh inputs
      Then coverage is measured even though those mutants can be reused
      And current independent evidence is recorded for every judged executable Go function
      When I repeat the strict run and cached check without any input change
      Then the fresh evidence can be reused without another coverage measurement
      And cached check runs no test, list or coverage command and writes nothing
      But stale independent evidence makes a strict run measure again even if its mutant outcomes remain reusable

    @strict-go-coverage @ID-MUT-160
    Scenario Outline: Incomplete coverage evidence cannot pass strict check
      Given a Go function's mutation outcomes are fresh but its independent coverage evidence is <evidence>
      When I run mutation check with --fail-uncovered --json
      Then its problem is "mutation.coverage-missing", with file, function and line
      And the exit code is 1
      And no test, list or coverage command runs and no file is written

      Examples:
        | evidence                                             |
        | absent in a legacy snapshot                           |
        | null                                                 |
        | missing its executable function's block inventory     |
        | supplied only by measurement of an unrelated Go module |

    @strict-go-coverage @ID-MUT-161
    Scenario Outline: Coverage depends on source and measurement inputs independently of mutants
      Given fresh strict coverage evidence for an unchanged Go function
      And a <input> input was <change> without changing that function
      When I run mutation check with --fail-uncovered --json for the function's file
      Then its problem is "mutation.coverage-stale", naming the changed input
      And the exit code is 1
      And a later strict run cannot bless the old profile with new fingerprints

      Examples:
        | input                          | change  |
        | module production helper       | changed |
        | non-importing module test      | added   |
        | configured support file        | removed |
        | module go.mod                  | changed |
        | recorded coverage build option | changed |

    @strict-go-coverage @ID-MUT-162
    Scenario: Partial judgment and moved functions preserve coverage identity
      Given two Go functions have fresh independent coverage evidence
      And a module test changed since, making both inventories stale
      And only one function changed in commits since "base"
      When I run mutation run with --since base --fail-uncovered
      Then only the changed function is judged and gets newly measured evidence
      And the unjudged function keeps its original stale evidence
      And strict check of the unjudged function cannot pass
      And moving a function down within its file reports uncovered blocks at their current lines after current measurement
      And repeated init functions retain the correct separate inventories when paired by name and hash

    @strict-go-coverage @ID-MUT-163
    Scenario: Strict coverage validates flags and does not widen other modes
      When I combine --fail-uncovered with --no-coverage, even for an empty selection
      Then the problem is "flags.conflict" for "--no-coverage" and the exit code is 2
      When strict Go run is combined with --coverage-report, --use-existing-coverage or --coverage-command
      Then the problem is "flags.conflict" naming the incompatible report flag and the exit code is 2
      And no report, coverage command or mutant is consumed to bless strict evidence
      But non-strict explicit-report and no-coverage runs retain their current behavior
      And other-language statement coverage, ordinary graph/sample freshness and survivor exceptions are unchanged
      And a survivor exception cannot excuse a strict Go uncovered-statement finding

    @strict-go-coverage @ID-MUT-164
    Scenario: Unsupported module scopes cannot reuse strict coverage evidence
      Given a Go function has fresh strict coverage evidence
      When an active go.work is introduced for its measurement environment
      Then strict run and cached strict check fail with "mutation.coverage-unsupported", naming the workspace
      And cached check does not run coverage or tests or refresh its evidence
      But with GOWORK=off the same inventoried module remains supported
      When its go.mod names a local replacement outside the inventoried module
      Then strict run and cached strict check fail with "mutation.coverage-unsupported", naming the replacement
      And a replacement beneath an excluded nested module is refused too
      And no external dependency tree is followed or admitted as fresh coverage
      But replacement inputs already inside the actual inventory remain supported
      And non-strict commands retain their existing behavior

  # Issue #26's ownership prerequisite is split by the person's instructions:
  # T-9 extracts legacy execution; T-10 exposes internal context and lifecycle
  # seams; mutation-command-ownership implements Linux guarantees here;
  # mutation-windows-ownership implements and verifies Windows on a Windows
  # machine later. ADR-0021 still requires both platforms before fail-fast.
  # This split keeps the original Windows gap, not a cross-platform completion
  # claim after Linux tests pass. Windows scenarios remain @wip until verified.
  # The person now prioritizes fresh bounded execution in GitHub #27 over
  # fail-fast #26. Linux ownership is shared groundwork for #27 too; it does
  # not introduce #26's scheduler or flag. Windows work is tracked separately
  # in #29 and stays deferred until a Windows machine is used.
  # Local mutation execution must stay sparse: never mutate this repository's
  # sources, and use only minimal targeted fixture trials on this machine.
  # Exercise helper-process ownership directly where possible; broader mutant
  # trial matrices and full mutation-fixture suites run in CI, not locally.
  # q-35 chose the separate prerequisite after a throwaway spike. Its new
  # internal capabilities are not original-product behavioral-red evidence.
  # Against T-10, each ownership assertion must compile and show an actual
  # behavior failure before its implementation; passing guards are separate.
  # Completed nonzero exits as well as success must survive later cancellation;
  # later cancellation must not overwrite an already observed own timeout.
  # Real lifecycle tests cover cleanup on acquisition/start/wait errors, not
  # just the spike's successful post-Wait cleanup hook. The spike proves no
  # descendant containment, output-drain guarantee or Windows Job ownership.
  # Delivery evidence: cab1f76 committed timeout, normal-return and startup
  # checks before d30f99e. Only normal return demonstrated original behavioral
  # red (WaitDelay expired before I/O complete); timeout/startup were passing
  # guards. Cancellation, exit-precedence and cleanup-error tests arrived after
  # implementation, despite q-35's requirement; do not claim full red-first
  # chronology or relabel older-revision checks as original evidence.
  # Coordinator review found that normal cleanup consumed the shared run-abort
  # budget. 5fcd83c demonstrated actual new red for normal completion and own
  # timeout against d30f99e; f779e4c reserves the shared deadline for parent
  # aborts. All these local checks used helper processes, zero mutant trials.
  # - Supervised commands own ordinary, non-detaching child/grandchild
  #   processes. Windows uses a kill-on-close Job Object with ownership
  #   established before children can escape; Unix uses a process group.
  #   Intentionally daemonizing/session-escaping Unix commands are outside
  #   this contract, not a hidden promise of container-strength isolation.
  # - A command scope closes on normal completion, mutant deadline, parent
  #   cancellation or execution error. Terminate remaining owned processes
  #   and join output/process waiters before returning or restoring/removing
  #   its worker copy. Bound cleanup with one five-second deadline; later
  #   fail-fast callers share that deadline across scopes, not per worker.
  # - A mutant's own deadline still means timeout/killed. Parent/run abort
  #   is distinct cancellation, never a successful kill/timeout judgment.
  #   Actual completed exit status must not be replaced by a later abort.
  # - Startup, containment or cleanup failures return explicit errors; do
  #   not silently run unsupervised, record a kill or erase the error.
  #   Target only captured owned handles/groups, never command-name matches
  #   or unrelated orphaned processes. Ownership establishment must be
  #   race-safe on Windows, not assignment after an unrestricted launch.
  # - Provide reusable context-aware supervision for the next slice's
  #   mutation, baseline, selection, list and coverage command paths. Worker
  #   timeout/normal-return cleanup is verified here; fail-fast scheduling
  #   and mode-specific preparation integration remain the next slice.
  # - This slice exposes no public --fail-fast flag and changes no aggregate
  #   scheduling, mutation scope, valid outcome/count, freshness or baseline
  #   timeout policy. Ordinary worker execution must retain its valid results
  #   while no longer leaving owned descendants behind. Sample's shared
  #   execution path must retain its contract too.
  Rule: Linux mutation commands return only after their owned process trees are cleaned up

    @mutation-command-ownership @ID-MUT-165
    Scenario: A mutant deadline cleans up children and grandchildren
      Given mutation commands run on Linux
      And a test command starts ordinary owned child and grandchild processes that keep running and hold output open
      When the mutant reaches its own timeout
      Then all owned processes are terminated and output waiters are joined before the command returns
      And the mutant is still counted killed by timeout
      And cleanup is bounded and the worker copy can be restored and removed
      And unrelated processes remain untouched

    @mutation-command-ownership @ID-MUT-166
    Scenario: Normal command completion also closes owned descendants
      Given mutation commands run on Linux
      And a baseline or mutant test command starts an ordinary owned descendant and then exits successfully
      And that descendant could continue work after the parent exits, even with its output streams closed
      When the command scope returns
      Then the descendant has stopped and cannot continue fixture work after return
      And the parent's actual successful exit is retained, not invented as timeout or a failed baseline
      And all owned process/output waiters are joined before worker copy removal

    @mutation-command-ownership @ID-MUT-167
    Scenario: Parent cancellation is not a mutant deadline
      Given mutation commands run on Linux
      And a supervised command and its owned child are running under a parent cancellation context
      When the parent cancels without the mutant's own deadline expiring
      Then command admission stops and the owned tree is terminated within the shared cleanup deadline
      And process/output waiters are joined before return
      And the result identifies cancellation, not a killed or timed-out mutant
      But a judgment completed before the cancellation retains its actual result

    @mutation-command-ownership @ID-MUT-168
    Scenario: Supervision failures do not permit an unsafe fallback
      Given mutation commands run on Linux
      And a supervised command cannot establish ownership, start or finish cleanup safely
      When its execution is attempted
      Then the corresponding error is returned rather than a passing or killed judgment
      And no command runs unrestricted after an ownership-establishment failure
      And only captured owned handles or groups can be targeted for cleanup
      And the same supervisor can be used by worker and preparation commands without changing their argument, environment or output contracts
      And intentionally detaching Unix commands are documented as unsupported, not claimed to be contained

  # Windows is a separate, pending part of ADR-0021. Implement and exercise
  # these guarantees on a Windows machine, not by cross-compilation alone.
  # Preserve existing Windows command behavior while shared prerequisites and
  # Linux work land. No Windows Job implementation is required in those items.
  # Race-safe ownership must precede any unrestricted child execution; assigning
  # an already running command to a Job is not sufficient. A Job setup failure
  # must never fall back to an unrestricted launch. The shared five-second
  # cleanup, accurate outcomes, waiter joins and no-unrelated-kills rules above
  # apply here too. No public fail-fast flag is introduced by either platform.
  Rule: Windows mutation commands return only after their owned process trees are cleaned up

    @wip @mutation-windows-ownership @ID-MUT-169
    Scenario: A Windows mutant deadline cleans up its owned Job tree
      Given mutation commands run on Windows
      And a test command starts ordinary owned child and grandchild processes that keep running and hold output open
      When the mutant reaches its own timeout
      Then all owned processes are terminated and output waiters are joined before the command returns
      And the mutant is still counted killed by timeout
      And cleanup is bounded and the worker copy can be restored and removed
      And unrelated processes remain untouched

    @wip @mutation-windows-ownership @ID-MUT-170
    Scenario: Normal Windows command completion also closes owned descendants
      Given mutation commands run on Windows
      And a baseline or mutant test command starts an ordinary owned descendant and then exits successfully
      And that descendant could continue work after the parent exits, even with its output streams closed
      When the command scope returns
      Then the descendant has stopped and cannot continue fixture work after return
      And the parent's actual successful exit is retained, not invented as timeout or a failed baseline
      And all owned process/output waiters are joined before worker copy removal

    @wip @mutation-windows-ownership @ID-MUT-171
    Scenario: Windows parent cancellation is not a mutant deadline
      Given mutation commands run on Windows
      And a supervised command and its owned child are running under a parent cancellation context
      When the parent cancels without the mutant's own deadline expiring
      Then command admission stops and the owned tree is terminated within the shared cleanup deadline
      And process/output waiters are joined before return
      And the result identifies cancellation, not a killed or timed-out mutant
      But a judgment completed before the cancellation retains its actual result

    @wip @mutation-windows-ownership @ID-MUT-172
    Scenario: Windows Job failures do not permit an unsafe fallback
      Given mutation commands run on Windows
      And a supervised command cannot establish Job ownership, start or finish cleanup safely
      When its execution is attempted
      Then the corresponding error is returned rather than a passing or killed judgment
      And no command runs unrestricted after an ownership-establishment failure
      And ownership is established before the command can execute and spawn descendants
      And only captured owned handles can be targeted for cleanup
      And the same supervisor can be used by worker and preparation commands without changing their argument, environment or output contracts

  # GitHub #27, now the person's priority, is fresh bounded execution, not
  # cached mutation sample and not #26 fail-fast. Linux ownership is shared
  # groundwork; T-11 supplies committed-input/static planning and T-12 supplies
  # supervised, fail-closed preparation before this public integration slice.
  # - Opt in with mutation run --count N; N is a positive integer and is ONE
  #   budget across every file/function/worker. --seed TEXT requires --count;
  #   its default is the resolved HEAD commit ID. Cached mutation sample stays
  #   unchanged. The initial public bounded mode is Linux-only; other platforms
  #   refuse it clearly before launching commands. Windows support is #29.
  # - Resolve one Git repository and HEAD once. Counted execution uses frozen
  #   version-controlled source/test/config/support inputs, not current dirty,
  #   staged or untracked project code. --since selects committed changed
  #   functions and explicit paths narrow that set; without --since, discover
  #   the selected committed HEAD paths. No additional checkout or session move
  #   is used. External tools/dependencies/environment remain a declared boundary,
  #   not container-strength or hermetic-build assurance. Reject unsupported
  #   repository/build scopes rather than silently executing live project code.
  # - Enumerate static sites without result-cache admission. Rank globally using
  #   SHA-256 and the TEXT seed, logical root-relative slash paths and identities
  #   distinguishing same-name units/sites, with deterministic tie-breaking.
  #   Report commit, seed and selection algorithm version. Select min(N, eligible)
  #   sites before scheduling; coverage does not silently redraw that selection.
  # - Selected covered sites run freshly even when a matching cached kill exists.
  #   Reused outcomes never spend this budget. One trial is ONE selected mutant's
  #   final judgment, including its own and applicable listed stages; baseline,
  #   list, coverage and conversion commands are not mutant trials. Reapplying
  #   that same mutant after a clean listed baseline is not a second trial.
  # - Count is an upper bound on actual trials, not a latency or test-command
  #   bound. Selected measured-uncovered sites remain selected and reported,
  #   with zero execution for those sites and no redraw. Preserve valid exception,
  #   own/listed, timeout and --fail-uncovered semantics. No applicable test
  #   command or failed/missing/malformed measurement is not a site-free success.
  #   Strict Go executable coverage, including zero-site functions, stays strict.
  # - Prepare fresh required coverage/listed reach on the frozen inputs without
  #   any local measurement/cache prerequisite. Every actual preparation command
  #   must succeed; the presence of a report never hides command/conversion/provider
  #   failures. Do not silently drop configured listed integration tests to save
  #   discovery cost. Refuse incompatible raw/existing coverage inputs; explicit
  #   no-coverage is permitted only where it does not bypass strict coverage or
  #   configured applicable listed-test discovery. Complete-mode flags retain
  #   their existing behavior. Persistent per-scenario coverage reuse is separate.
  # - This first mode publishes NO mutation snapshots, source annotations or
  #   persistent coverage cache. Completed outcomes live in the current report;
  #   existing complete-cache files remain untouched. A sampled pass proves only
  #   its actual judgments, never a new complete-cache proof. Existing genuine
  #   complete proof is a separate fact, not invalidated just by a sampled run.
  # - JSON remains one object. Add a sampling object with budget, eligible,
  #   selected, executed and omitted site counts, seed/algorithm/commit provenance,
  #   sampled assurance and actual completion/stop information. List selected
  #   site identities and outcomes, judged and omitted subjects, and actual stage
  #   states. Undecided/cancelled/blocked sites have no fabricated mutation outcome.
  #   Preserve completed progress in output on interruption/infrastructure error.
  #   Errors fail the command; genuine no-sites is explicitly not applicable.
  # - Own all counted-mode worker and preparation process trees, and join them
  #   before returning/removing private inputs. A run abort shares ONE five-second
  #   cleanup deadline; normal command completion and a mutant's own timeout must
  #   not start or consume that run-abort deadline. Ordinary aggregate counted
  #   execution does not become #26 fail-fast on a survivor.
  # - Local mutant trials stay sparse: none over this repository; at most one or
  #   two tiny selected fixture trials per focused behavioral case where needed.
  #   Static planning, raw snapshots and direct helper processes need no trials.
  #   Broader concurrency/multi-mutant regressions run in CI, not on this machine.
  #   Unknown-flag failures are CLI-admission red, not scheduler-red evidence.
  Rule: Fresh counted mutation judges a bounded committed selection without claiming full proof

    @wip @mutation-counted-run @ID-MUT-173
    Scenario: Counted execution has explicit admission and platform boundaries
      Given the complete mutation run and cached mutation sample commands are available
      When mutation run is given a nonpositive or noninteger count, or seed without count
      Then it fails with a usage error before any mutation trial
      And counted mode requires one supported Git repository and a resolvable HEAD
      And on an unsupported platform counted mode fails clearly before launching commands
      But existing complete and cached-sample behavior remains unchanged

    @wip @mutation-counted-run @ID-MUT-174
    Scenario: Counted execution evaluates frozen committed inputs
      Given a committed project has a source function, tests and measurement settings
      And those files have different staged, unstaged or untracked working-tree versions
      When a count-one run judges a committed selection
      Then discovery, coverage, clean baselines and mutation tests use the resolved committed inputs
      And its report names that commit and logical source paths
      And the live working tree is not rewritten or mistaken for committed proof
      And unsupported scopes fail rather than silently following live project inputs

    @wip @mutation-counted-run @ID-MUT-175
    Scenario: Fresh selection is reproducible across paths and same-name units
      Given committed functions include same-name units and more eligible sites than the budget
      When two count-one runs use the same TEXT seed and committed inputs
      Then they select the same fully identified site regardless of discovery ordering
      And a default seed is the resolved HEAD commit ID
      And reports include the seed and deterministic selection algorithm version
      And alternate-seed selection can be checked without executing mutation trials

    @wip @mutation-counted-run @ID-MUT-176
    Scenario: The global fresh budget holds even when every selected mutant is killed
      Given several committed files and functions have eligible sites but no mutation cache
      And the worker count exceeds the count-one budget
      When a counted run successfully judges its selected covered mutant
      Then exactly one distinct mutation trial executes across the entire selection
      And omitted sites and subjects are reported rather than executed
      And no full mutation prepass prepares the selection
      And the same fresh discovery admits changed functions with missing or stale cache entries
      And a matching prior cached kill never substitutes for the fresh trial

    @wip @mutation-counted-run @ID-MUT-177
    Scenario: Counted mode leaves complete and cached-sample contracts intact
      Given complete-run and cached-sample fixtures have their existing results
      When commands run without the count opt-in
      Then complete scheduling, scope, cache publication and source annotation behavior are unchanged
      And cached mutation sample still rechecks only its eligible recorded outcomes
      And its existing nothing-to-sample behavior is not the counted-mode no-cache behavior

    @wip @mutation-counted-run @ID-MUT-178
    Scenario: Applicable listed integration tests finish a single fresh judgment
      Given a selected committed mutant survives its file's own tests
      And listed integration tests reach its line and kill it
      When count-one execution judges it
      Then own tests run before the applicable listed tests
      And the listed selection baseline runs without the mutant before its mutation stage
      And the final outcome is killed with the correct listed attribution
      And the own and listed mutation stages consume one trial, not two
      And no unselected site executes

    @wip @mutation-counted-run @ID-MUT-179
    Scenario: Fresh outcomes preserve survivor, exception and own-timeout meanings
      Given a counted selection has one covered mutation site
      When its complete applicable test sequence passes on the mutant
      Then the run fails for that selected survivor unless its matching exception is valid
      And an exception must not exclude the site from fresh selection or skip its trial
      But a killed mutant or its own test deadline is a valid completed judgment
      And stale exceptions fail without being trusted as successful outcomes
      And counted mode does not silently become fail-fast scheduling

    @wip @mutation-counted-run @ID-MUT-180
    Scenario: Measured uncovered selection is explicit and is never redrawn
      Given a seeded selected site is reached by neither own nor listed tests
      When count-one preparation measures its coverage successfully
      Then that site is reported uncovered with zero mutation trials executed
      And selection remains unchanged rather than drawing another site
      And the run is not described as a range with no eligible sites
      And fail-uncovered fails that judgment
      And strict Go still checks executable zero-site functions and exempts empty bodies

    @wip @mutation-counted-run @ID-MUT-181
    Scenario: Counted preparation never converts failed measurement into successful assurance
      Given fresh counted preparation requires list, coverage or provider commands
      When a required command fails, or its report is missing or malformed
      Then the command fails and reports the actual failed stage
      And a report produced by a failed command does not erase its failure
      And per-test conversion and coverage failures are not silently ignored
      And no mutation trial starts after failed preparation
      And no preexisting local measurement or per-scenario coverage map is required for successful preparation
      And raw or existing coverage overrides cannot silently bypass required fresh evidence or listed tests

    @wip @mutation-counted-run @ID-MUT-182
    Scenario: Clean baseline failure is not a mutation kill
      Given one selected fresh mutant needs own or applicable listed baselines
      When an unmutated required baseline fails
      Then counted execution fails with its actual baseline problem
      And that site has no invented killed, timeout or survived outcome
      And later stages do not report a baseline as passed when it never ran
      And already completed valid judgments remain visible in the current report

    @wip @mutation-counted-run @ID-MUT-183
    Scenario: Interruption reports partial work and cleans up owned commands
      Given a count-two selection has one completed judgment and one active judgment
      When the counted run is interrupted while the second judgment is active
      Then the completed result remains visible
      And the unfinished judgment is cancelled or undecided, not a killed or timed-out mutant
      And no further command is admitted after the run abort
      And all owned worker and preparation descendants and waiters finish before private input removal
      And one shared five-second post-abort cleanup deadline applies without killing unrelated processes

    @wip @mutation-counted-run @ID-MUT-184
    Scenario: Machine and text output distinguish sampled work from population completeness
      Given a counted selection omits some sites and may stop before all selected work completes
      When its report is emitted
      Then JSON is one object with budget, eligible, selected, executed and omitted counts
      And it lists selected identities and actual outcomes with commit, seed and algorithm provenance
      And judged and omitted subjects and real preparation or baseline stages are explicit
      And cancelled, blocked and unattempted selected work has no fabricated mutation outcome
      And a sampled pass is described as assurance about sampled judgments only
      And count does not claim to bound discovery, baseline commands or total latency

    @wip @mutation-counted-run @ID-MUT-185
    Scenario: Sampled execution publishes no persistent complete proof
      Given a multi-site committed function has no complete mutation results
      When a count-one run completes successfully
      Then no mutation snapshot, source annotation or persistent coverage cache is published
      And complete mutation check still reports its missing or stale complete proof without running tests
      And preexisting snapshot and annotated source bytes remain unchanged
      And any genuinely valid complete cache remains a separate unchanged source of proof

    @wip @mutation-counted-run @ID-MUT-186
    Scenario: Only a genuinely site-free range is not applicable
      Given a supported committed selection contains no eligible mutation sites
      When counted execution completes its applicable admission and strict obligations
      Then the report explicitly says not applicable with zero eligible and selected sites
      And it does not run an avoidable mutation prepass
      But absent cache, missing tests, unsupported input scope and failed coverage or baseline are not site-free success
