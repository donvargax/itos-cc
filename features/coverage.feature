Feature: Coverage
  crap and mutation run need to know which lines and branches the tests execute. itos-cc runs
  each language's own coverage tools per build root, or reads reports the
  user already has, and matches each report entry to a source file on disk.

  Rule: Chosen files run only the tests that load them

    @ID-COV-01 @binary-only
    Scenario: A Go file is measured by the tests that link its package
      Given a/a.go, tested by its own package and by e2e/e2e_test.go, which imports package a
      And slow/slow_test.go, which never loads package a
      When I run "itos-cc crap a/a.go"
      Then coverage runs "go test ... -coverpkg=example.com/m/a ... example.com/m/a example.com/m/e2e"
      And its coverage is the same as with the whole module's tests
      # a test binary that does not link a package cannot cover it

    @ID-COV-02 @binary-only
    Scenario: A TypeScript file is measured by the tests that import it
      When I run "itos-cc crap src/board.ts" in a Vitest or Jest project
      Then coverage runs vitest related --run src/board.ts, or jest --findRelatedTests src/board.ts

    @ID-COV-03 @binary-only
    Scenario: mutation run measures a Go file by its own package's tests
      When I run "itos-cc mutation run a/a.go"
      Then coverage runs "go test ... -coverprofile=... example.com/m/a", without -coverpkg
      # the tests mutation run kills a/a.go's mutants with; see mutate.feature

    @ID-COV-04 @binary-only
    Scenario Outline: The whole suite
      When I run "itos-cc <command>"
      Then coverage runs the whole suite of each build root

      Examples:
        | command                         |
        | crap                            |
        | crap --all-tests a/a.go         |
        | mutation run --all-tests a/a.go |
      # Python, Kotlin, Dart, a coverage script, and c8 always run the whole suite

  Rule: Each build root gets the language's own coverage command

    @ID-COV-05 @binary-only
    Scenario Outline: Choosing the coverage command
      Given a <language> project whose build root holds <marker>
      And <setup>
      When coverage is run for it
      Then the command is <command>

      Examples:
        | language   | marker           | setup                                        | command                                                           |
        | Go         | go.mod           | nothing else                                 | go test ./... -covermode=set -coverpkg=./... -coverprofile=...    |
        | TypeScript | package.json     | a "coverage" script                          | <pm> run coverage, read from coverage/lcov.info                   |
        | TypeScript | package.json     | vitest 5 or later as a dependency            | the installed vitest run with the v8 provider writing LCOV        |
        | TypeScript | package.json     | jest as a dependency                         | the installed jest --coverage writing LCOV                        |
        | TypeScript | package.json     | neither vitest nor jest                      | the installed c8 around <pm> run test, writing LCOV               |
        | Python     | pyproject.toml   | pytest importable                            | coverage run --branch -m pytest, then coverage lcov               |
        | Python     | setup.py         | pytest not importable                        | coverage run --branch -m unittest discover, then coverage lcov    |
        | Kotlin     | build.gradle.kts | the build mentions kover                     | gradle koverXmlReport                                             |
        | Kotlin     | build.gradle     | the build does not mention kover             | gradle test jacocoTestReport                                      |
        | Kotlin     | pom.xml          | it or a parent declares jacoco-maven-plugin  | mvn -q jacoco:prepare-agent test jacoco:report                    |

      # slice-32, q-74 (a): a Flutter app or package, run with flutter test,
      # never pub get (--no-pub: nothing the lockfile has not fetched is
      # downloaded, as npx is never used). With no path flutter test runs
      # test/ alone, so integration_test/ never counts (q-76).
      @slice-32
      Examples:
        | language   | marker           | setup                                        | command                                                           |
        | Dart       | pubspec.yaml     | a dependency on the flutter SDK              | flutter test --no-pub --coverage --coverage-path=... writing LCOV |

      # slice-17, issue #45: a vitest that resolves from the package's
      # node_modules/.bin counts as Vitest, though the package names it in
      # neither dependencies nor devDependencies, when its test script
      # names vitest, vp or vite or a vite or vitest config sits beside its
      # package.json (MUT-280). crap and coverage choose their command as
      # mutation does. Binary-only, as COV-05 is. Before slice-17 the
      # command is the installed c8 around <pm> run test, failing at the
      # Then.
      @slice-17
      Examples:
        | language   | marker       | setup                                                                                                                              | command                                                    |
        | TypeScript | package.json | vitest 5 or later installed through another package, named in neither its dependencies nor its devDependencies, its test script "vitest run" | the installed vitest run with the v8 provider writing LCOV |

    @ID-COV-06 @binary-only
    Scenario: The project's own package manager runs its scripts
      Given a TypeScript project in a workspace whose root declares "packageManager": "pnpm@9.12.0"
      When coverage is run for it
      Then <pm> is pnpm
      # otherwise the nearest lockfile decides (pnpm, yarn, bun, npm), and npm when there is none

    @ID-COV-07 @binary-only
    Scenario: Node tools come from node_modules, never from the registry
      Given a TypeScript project in a workspace, with its tools installed at the workspace root
      When coverage is run for it
      Then vitest, jest, or c8 runs from the nearest node_modules/.bin
      And npx is never used, so nothing that the project's lockfile does not pin is downloaded

    @ID-COV-08 @binary-only
    Scenario Outline: A tool that is not installed is not fetched
      Given a <language> project <missing>
      When coverage is run for it
      Then nothing runs for it
      And stderr says what to install, or to use --coverage-command
      And only complexity is reported for it

      Examples:
        | language   | missing                                                      |
        | TypeScript | using vitest that is not installed                           |
        | TypeScript | using vitest 5.0.2 without @vitest/coverage-v8 installed     |
        | TypeScript | using jest that is not installed                             |
        | TypeScript | using neither vitest nor jest, without c8 installed          |
        | Python     | whose python cannot import coverage                          |
        | Kotlin     | built by Maven, with no pom.xml declaring jacoco-maven-plugin |

      # slice-32: flutter is never downloaded (decision 26), and a package
      # pub has not resolved is not resolved for it: the user runs
      # flutter pub get, as they install node_modules.
      @slice-32
      Examples:
        | language   | missing                                                      |
        | Dart       | whose flutter is not installed                               |
        | Dart       | with no .dart_tool/package_config.json, never resolved       |
      # the user makes the tool available; itos-cc never installs or downloads it

    @ID-COV-09 @binary-only
    Scenario: Only the current Vitest major is supported
      Given a TypeScript project using vitest 4.1.11
      When coverage is run for it
      Then nothing runs for it
      And stderr says "Vitest 4.1.11 is not supported; upgrade to Vitest 5 or later to measure coverage"
      And only complexity is reported for it
      # older Vitest writes V8 blocks, not branches; Jest's istanbul branches are the same in every version

    @ID-COV-10 @binary-only
    Scenario: Python uses the project's own virtualenv
      Given a Python project with a .venv directory
      When coverage is run for it
      Then the .venv's python runs the tests

    @ID-COV-11 @binary-only
    Scenario: A Gradle wrapper is preferred
      Given a Kotlin Gradle module under a build root that holds gradlew
      When coverage is run for it
      Then the gradlew wrapper is used instead of gradle

    # slice-32: FVM pins a Flutter app's SDK as gradlew pins Gradle's.
    # Without .fvm/flutter_sdk (fvm install never ran there), the flutter on
    # PATH runs, as when there is no .fvmrc.
    @slice-32 @ID-COV-40 @binary-only
    Scenario: A Flutter app's FVM SDK is preferred
      Given a Flutter app whose .fvmrc pins the SDK that .fvm/flutter_sdk links to
      When coverage is run for it
      Then .fvm/flutter_sdk/bin/flutter runs the tests instead of the flutter on PATH

    # slice-32, q-74 (a): pure Dart packages (dart test) are a later idea.
    @slice-32 @ID-COV-41 @binary-only
    Scenario: A Dart package that does not use Flutter is not measured yet
      Given a Dart package whose pubspec.yaml does not depend on the flutter SDK
      When coverage is run for it
      Then nothing runs for it
      And stderr says only Flutter apps and packages are measured
      And only complexity is reported for it

    # slice-32: a pub workspace's members are its build roots, each run in
    # its own directory, as flutter test writes SF: paths relative to it.
    @slice-32 @ID-COV-42 @binary-only
    Scenario: Each member of a pub workspace is measured in its own directory
      Given a pub workspace whose root pubspec.yaml lists the Flutter packages app and core
      When coverage is run
      Then flutter test runs in app and in core, and nothing runs at the workspace root
      And each report's lib/ paths are read against its member's directory

    @ID-COV-12 @binary-only
    Scenario: Several build roots and languages are measured separately and merged
      Given a repository with a Go module and a TypeScript package
      When coverage is run
      Then each build root runs its own coverage command
      And the reports are merged into one

    @ID-COV-13 @binary-only
    Scenario: Failing tests still contribute coverage
      Given a project whose tests fail but still write a coverage report
      When coverage is run
      Then the failure is logged to stderr
      And the report is still used

    @ID-COV-14 @binary-only
    Scenario: Raw reports stay out of version control
      When coverage is run
      Then reports are written under .metrics/coverage/
      And .metrics/coverage/.gitignore ignores everything in it

  # slice-16 decisions (coordinator, 2026-10-21: the spec drafter's
  # recommendations, taken as routine calls inside the issue's scope):
  # 1. itos-cc measures; the project's own test script enforces its floor. Every coverage
  #    command itos-cc composes for these tools sets the floor to nothing from the command
  #    line, which overrides the config (q-G: every command, not only selections): - Vitest,
  #    each vitest related, vitest run and the inventory's run:
  #    --coverage.thresholds.lines=0 --coverage.thresholds.functions=0
  #    --coverage.thresholds.branches=0 --coverage.thresholds.statements=0
  #    --coverage.thresholds.100=false --coverage.thresholds.autoUpdate=false.
  #    autoUpdate=false is required: with zero thresholds and the project's autoUpdate on,
  #    Vitest would write what one file's tests measured into the project's config, in the
  #    project's own folder. - Jest, each jest --coverage … with or without
  #    --findRelatedTests: --coverageThreshold={}, a JSON value that replaces the config's
  #    whole object, its path and glob keys included. - coverage.py (q-H: included, the same
  #    bug for a Python file's reaching tests), each coverage lcov: --fail-under=0. - The
  #    agent confirms each flag against the pinned tools before relying on it: Vitest 5.0.2
  #    and coverage.py from testdata/tools, and Jest by its documented CLI, since no Jest is
  #    pinned. A flag a pinned tool refuses stops the slice with a question; nothing is
  #    guessed. - Go, Kotlin (koverXmlReport, jacocoTestReport and jacoco:report run no
  #    verification) and Dart have no floor in the commands itos-cc runs, and change
  #    nothing. A project's own coverage script (<pm> run coverage) and --coverage-command
  #    run exactly as given.
  # 2. Under --fail-uncovered, a coverage command that failed proves nothing, even when it
  #    wrote a report first (q-E, recommended (b)). Its build root is coverage.measured-
  #    nothing for its language and dir, as ID-MUT-204 and the help already promise, and the
  #    run stops before any mutant. It is never mutation.coverage-missing for the root's
  #    functions. The message (Unmeasured.Reason) names the command and its exit status; the
  #    subject keys stay dir and language (q-I), so the assembly's coverage classifier
  #    (cmd/itos-cc/coverage.go:69-75) and crap's wording are untouched. This is what is
  #    left once decision 1 holds: a floor the command line cannot reach (a Vitest threshold
  #    for a glob pattern), or a test that fails only under coverage. - Mechanism. The
  #    Report records a plan whose commands failed although it read data, beside Missing.
  #    Plan.measured (coverage/measure.go:193-204) today clears Missing as soon as the
  #    report names a source. Mutation's strict gaps (unmeasuredToJudge and strict) read
  #    that record for a language with mutants to judge. - What stays. crap, and runs
  #    without --fail-uncovered, keep using a failed run's report as ID-COV-13 says.
  #    RunSupervised (counted runs) is already fail-closed and does not change. - The
  #    issue's other suggestion. It also asks that a threshold's exit status not count as a
  #    failed measurement when the tests passed. That is not adopted: measure.go decides by
  #    exit status and report alone, never by what the runner prints (measure.go:120-122),
  #    and exit status cannot tell a missed floor from a failing test.
  # 3. Help: mutation run's coverage.measured-nothing line (run_contract.go:399) already
  #    says "a language's coverage command failed or wrote no report", and stays. The
  #    coverage engine's help or README text for Vitest and Jest gains one sentence: itos-
  #    cc's coverage run sets the project's thresholds to zero, which its own test script
  #    still enforces. docs/CLI.md's coverage.measured-nothing row names crap --threshold,
  #    mutation run --fail-uncovered; today it names crap alone (stale).
  # Owns and must not touch: see the table above. Must not touch coverage/pertest.go,
  # internal/coveragetools and the coverage port's Failure (slice-15's), or
  # coverage/supervised.go.
  # slice-16, issue #46: Vitest's test.coverage.thresholds, Jest's
  # coverageThreshold and coverage.py's fail_under hold a whole package to
  # a floor. A coverage run of the tests that load the chosen files leaves
  # every other file at 0%, misses that floor and exits 1 after writing its
  # report, and itos-cc read the run as failed: under --fail-uncovered every
  # function of the file was mutation.coverage-missing, its fix asking for a
  # test that exists. Decided by the coordinator on 2026-10-21: itos-cc
  # measures, and the project's own test script enforces its floor, so
  # every Vitest, Jest and coverage.py command itos-cc composes, a
  # selection's, the whole suite's and the inventory of files no test
  # loads, sets each threshold to nothing from the command line, which
  # overrides the config, and Vitest's autoUpdate off. A floor the command
  # line cannot reach, such as Vitest's threshold for a glob pattern, still
  # fails the run, which ID-MUT-270 reports.
  Rule: The project's coverage floor never fails itos-cc's measurement

    # Binary-only for good: the floor, and the command that clears it, are
    # the real tools', as COV-05's are; the fake measures as declared. The
    # TypeScript row is issue #46's project, run by cd.yml's languages job
    # with the pinned Vitest and coverage.py. Before slice-16 the command
    # exits 1 after writing its report, failing at the first Then, and
    # every function of the file is mutation.coverage-missing.
    @slice-16 @ID-COV-44 @binary-only
    Scenario Outline: A run of one file's tests is not held to the project's <language> coverage floor
      Given a <language> project with two files, each fully covered by its own test, and <floor>
      And the whole suite meets that floor, and either file's test alone does not
      When I run "itos-cc mutation run --fail-uncovered --json" for one of the two files
      Then stderr says no coverage command failed
      And no problem is "mutation.coverage-missing" or "coverage.measured-nothing"

      Examples:
        | language   | floor                                                        |
        | TypeScript | vitest.config.ts's test.coverage.thresholds.statements of 80 |
        | Python     | pyproject.toml's [tool.coverage.report] fail_under of 80     |

    # Binary-only for good, as COV-05's Jest row: the project's jest is a
    # placeholder the command is chosen for, so the step reads the command
    # itos-cc announces. No Jest is pinned to measure with. Before slice-16
    # the command has no --coverageThreshold, failing at the Then.
    @slice-16 @ID-COV-45 @binary-only
    Scenario: Jest's coverage command clears the project's coverageThreshold
      Given a TypeScript project whose build root holds package.json
      And jest as a dependency
      When I run "itos-cc crap src/board.ts"
      Then coverage runs jest with --findRelatedTests src/board.ts and --coverageThreshold={}

    # A guard, not a red: Vitest's thresholds.autoUpdate writes a higher
    # measured value back into the config. Before slice-16 a run of one
    # file's tests never reaches the floor of 80, so nothing is written;
    # with thresholds of 0 from the command line and autoUpdate left on,
    # every run would rewrite the project's vitest.config.ts with what one
    # file's tests measured, as coverage runs in the project's own folder.
    # Binary-only for good: the config is the real Vitest's to write.
    @slice-16 @ID-COV-46 @binary-only
    Scenario: Measuring never rewrites the project's coverage floor
      Given the TypeScript project of ID-COV-44, its thresholds set to autoUpdate
      When I run "itos-cc mutation run --json" for one of the two files
      Then vitest.config.ts is byte for byte as committed

    # The inventory of a TypeScript file no test loads runs Vitest with no
    # test, so every file it measures is at 0%: the floor failed it, and
    # the file's functions were mutation.coverage-missing instead of its
    # lines mutation.uncovered-statement, failing at the Then before
    # slice-16. Binary-only for good: the inventory is the real Vitest's
    # run, which the fake does not make.
    @slice-16 @ID-COV-47 @binary-only
    Scenario: The lines of a file no test loads are listed under the project's floor
      Given the TypeScript project of ID-COV-44, with a third file no test imports
      When I run "itos-cc mutation run --fail-uncovered --json" for that file
      Then its executable lines are "mutation.uncovered-statement" and its mutants "mutation.uncovered"
      And no problem is "mutation.coverage-missing"

  Rule: The user can bring their own coverage

    @ID-COV-15 @binary-only
    Scenario: Reading reports already on disk
      Given a Go project with a coverage.out from an earlier run
      When I run "itos-cc crap --use-existing-coverage"
      Then no tests are run
      And coverage is read from the first existing report for each build root

    @ID-COV-16
    Scenario: Naming reports explicitly
      When I run "itos-cc crap --coverage-report a/lcov.info --coverage-report b/coverage.out"
      Then both reports are read, relative to the working directory

    @ID-COV-17 @binary-only
    Scenario: Running a custom coverage command
      When I run "itos-cc crap --coverage-command 'make cover' --coverage-report cover.out"
      Then "make cover" runs through the platform shell
      And cover.out is read afterwards

    @ID-COV-18
    Scenario: A custom command must say where its report lands
      When I run "itos-cc crap --coverage-command 'make cover'"
      Then stderr says "itos-cc: --coverage-command needs --coverage-report to say where its report lands. Add --coverage-report FILE."
      And the exit code is 80

    @ID-COV-19
    Scenario: Skipping coverage
      When I run "itos-cc crap --no-coverage"
      Then no tests are run
      And only complexity is reported

  Rule: Report formats are detected from their content

    @ID-COV-20
    Scenario Outline: Loading a report
      Given a report file "<file>"
      When it is loaded
      Then it is read as <format>
      And it has one entry for "<source>"

      Examples:
        | file       | format            | source                           |
        | go.out     | a Go cover profile | example.com/demo/board/board.go |
        | lcov.info  | LCOV              | src/demo/board.ts                |
        | jacoco.xml | JaCoCo XML        | demo/game/Board.kt               |

    @ID-COV-21
    Scenario Outline: Weighting covered code
      Given a <format> report
      When the covered share of a function is computed
      Then each measured piece is weighted by <weight>

      Examples:
        | format            | weight                        |
        | LCOV              | one per line                  |
        | Go cover profile  | the block's statement count   |
        | JaCoCo/Kover XML  | the line's instruction count  |

  Rule: A function with branches is scored by the branches it took

    @ID-COV-22
    Scenario: Branch coverage comes first
      Given a coverage.py LCOV report for:
        """
        def pick(x):
            r = 0
            if x > 0:
                r = 1
            else:
                r = 2
            return r
        """
      And the tests call pick(1) once
      When the covered share of pick is computed
      Then it is 50%, one of its two branches
      # by lines it would be 5 of 6

    @ID-COV-23
    Scenario Outline: Where branches come from
      Given a <format> report
      Then a decision is <decision>

      Examples:
        | format            | decision                                                 |
        | LCOV              | a BRDA block with two or more branches                   |
        | JaCoCo/Kover XML  | a line with branch counters, weighted by its branches    |
        | Go cover profile  | never: Go has no branch data, so statements decide       |

    @ID-COV-24
    Scenario: A function without branches is scored by its lines
      Given a function whose lines hold no decision in the report
      Then its coverage is the share of its lines or statements that ran

    @ID-COV-25
    Scenario: One-branch LCOV blocks are not decisions
      Given an LCOV report from c8, Node's test runner, or a Vitest older than 5
      And it lists the function body as a block and leaves out the arm that ran
      When the covered share of a function is computed
      Then those one-branch blocks are ignored and its lines decide

  Rule: Report paths are matched to source files

    @ID-COV-26
    Scenario: Exact matches
      Given a report entry whose path is absolute or relative to the report's base directory
      When it is matched to sources
      Then it matches that source file exactly

    @ID-COV-27
    Scenario: Matching by the longest shared tail
      Given a report entry "example.com/demo/board/board.go"
      And a source file "/work/demo/board/board.go"
      When it is matched to sources
      Then it matches that source by its trailing path components

    @ID-COV-28
    Scenario: A path that names a file on disk is that file
      Given sources "/p/b/main.go" only, as when --changed selects it
      And a report entry "a/b/main.go" or "/p/a/b/main.go", a file that exists
      When it is matched to sources
      Then it matches neither source
      # the tail "b/main.go" would lend another file's coverage to b/main.go

    @ID-COV-29
    Scenario: A module path names the file its tail finds under the project
      Given sources "metrics/metrics.go" only, and a project file "graph/metrics.go"
      And a Go profile entry "example.com/m/graph/metrics.go"
      When it is matched to sources
      Then it matches neither source
      # its longest tail that exists, graph/metrics.go, is not a source

    @ID-COV-30 @binary-only
    Scenario: A symlinked directory still matches
      Given a source reached through a symlinked directory
      And a report entry that names the same file by the symlink's target
      When it is matched to sources
      Then it matches that source

    @ID-COV-31
    Scenario: An ambiguous tail is not guessed
      Given sources "/p/a/util.py" and "/p/b/util.py"
      And a report entry "util.py" from a base directory that holds neither
      When it is matched to sources
      Then it matches neither source

    @bug-1 @ID-COV-32
    Scenario: Separators do not matter
      Given a report written with "\" separators and sources with "/" separators, or the reverse
      When it is matched to sources
      Then the paths still match

    @ID-COV-33
    Scenario: Entries for dependencies, tests, and generated code are dropped
      Given a report entry that matches no selected source
      When the report is built
      Then that entry is ignored

    # slice-32: flutter test's LCOV names only the libraries a test loaded,
    # so a lib/ file it does not name was measured and nothing ran it: 0%,
    # not unknown as a language nobody measured is (ID-CRAP-05). A file
    # whose coverage:ignore-file pragma Flutter honours is not coverable at
    # all, loaded or not.
    @slice-32 @ID-COV-43 @binary-only
    Scenario: A Dart library no test loads is uncovered, not unmeasured
      Given a Flutter app whose test loads lib/board.dart and no test loads lib/unused.dart or lib/ignored.dart
      And lib/ignored.dart's first line is "// coverage:ignore-file"
      When I run "itos-cc crap"
      Then lib/unused.dart's functions are at 0% coverage
      But lib/ignored.dart's functions have no CRAP score

  Rule: A piece of code listed more than once counts once

    @ID-COV-34
    Scenario: Go test binaries each list every block
      Given a module with packages "a", "b", and "c", each fully covered by its own tests
      And a profile from go test -coverpkg=./..., which lists every block once per test binary
      When the covered share of a function in "a" is computed
      Then it is 100%, as go tool cover -func says
      # counting each copy would make it 33%

    @ID-COV-35
    Scenario: Several reports of the same file are combined
      Given two reports given with --coverage-report, such as unit and integration runs
      And each covers a line of a file the other does not
      When the covered share of the file is computed
      Then a line or block is covered when any report covered it
      And a decision counts the branches of the report that took the most

  Rule: A function's coverage starts where calling it starts

    @ID-COV-36
    Scenario: Python coverage starts at the body
      Given the Python function:
        """
        @cache
        def f(
            x,
        ):
            return x
        """
      Then its coverage starts at line 5, not at its first line 2
      # importing a module executes every def line

    @ID-COV-37
    Scenario: TypeScript coverage starts at the first statement
      Given the TypeScript function:
        """
        export const f = (x: number) => {
          // why
          return x;
        };
        """
      Then its coverage starts at line 3
      # loading a module executes every export const line

  # coverage-prefix: the lines itos-cc writes to stderr itself while it
  # measures coverage (the command it runs, why one failed) began
  # "coverage:", a command that does not exist, where every other line
  # itos-cc writes begins "itos-cc:" (mutation-progress-prefix changed the
  # mutation commands' lines). Decided by the coordinator on 2026-10-07:
  # - A line naming a command begins "itos-cc: coverage <dir>$ <command>",
  #   as the baseline's "itos-cc: baseline <dir>$ <command>" does, and
  #   "itos-cc: coverage $ <command>" for --coverage-command; any other line
  #   begins "itos-cc: coverage: ". That holds for every such line, of crap
  #   and of mutation run alike: the coverage command, go tool covdata, the
  #   listed tests' coverage and a language that cannot be measured.
  # - The output of the coverage commands themselves is passed on as it is.
  Rule: What itos-cc says while it measures

    @coverage-prefix @ID-COV-38 @binary-only
    Scenario: The coverage command is announced as itos-cc's
      When I run "itos-cc crap a/a.go"
      Then stderr has a line "itos-cc: coverage <dir>$ go test …" naming the command coverage runs
      And no line of stderr begins "coverage:"

    @coverage-prefix @ID-COV-39 @binary-only
    Scenario: A coverage command that fails is reported as itos-cc's
      Given a test of a/a.go fails
      When I run "itos-cc crap a/a.go"
      Then stderr has a line beginning "itos-cc: coverage: go: " that gives the command's exit status
      # go test passes on the failing binary's own "coverage: N% of
      # statements" line, so this one does not ask that no line begins so

  # slice-61 (go-scope-fallback-silent): a Go run measures chosen files with the tests
  # of the packages that link them, from go list -test ./... (ID-COV-01).
  # When go list fails, or lists no package in the folder of any chosen
  # file, the run falls back to the whole module, -coverpkg=./... ./...,
  # and said nothing: bug-10 hid behind the second on macOS, where go list
  # named the physical /private/var folders. Decided by the coordinator on
  # 2026-10-26:
  # - The fallback stays; one line says why, with the coverage-prefix's
  #   "itos-cc: coverage: <dir>: " and the words "measuring the whole
  #   module, as":
  #   - "go list failed: " and the first line go list wrote to stderr, or
  #     its exit status when it wrote none;
  #   - "go list lists no package in <folder>", the first chosen file's
  #     folder as an absolute path, then "; the first it lists is in
  #     <folder>", the absolute folder of the first package go list lists,
  #     so a path told two ways shows at once. When it lists none, the line
  #     ends after the chosen folder.
  # - A run with no file chosen, which means the whole module, says
  #   nothing new, nor does a run whose files go list finds.
  # - Only goPlan (internal/covrun/run.go), the plan internal/coveragetools'
  #   measurer runs, falls back silently; goScopeSupervised, which
  #   mutation's fresh environment (freshenv) runs, already fails with its
  #   reason and is unchanged.
  # Before the slice, no such line is written: ID-COV-48 and ID-COV-49 fail
  # at their last Then. ID-COV-50 holds already and guards the quiet runs.
    @slice-61 @ID-COV-48 @binary-only @wip
    Scenario: A Go file in no package go list lists says why the whole module measures it
      Given a/a.go, tested by its own package, and tools/gen.go, which builds only under the ignore constraint
      When I run "itos-cc crap tools/gen.go"
      Then coverage runs "go test ... -coverpkg=./... ... ./..."
      And stderr has a line "itos-cc: coverage: <dir>: measuring the whole module, as go list lists no package in <dir>/tools; the first it lists is in <dir>/a"

    @slice-61 @ID-COV-49 @binary-only @wip
    Scenario: A Go module go list cannot list says why the whole module measures a file
      Given a/a.go, tested by its own package, and b/b.go, which imports example.com/m/nope, a package of no module
      When I run "itos-cc crap a/a.go"
      Then coverage runs "go test ... -coverpkg=./... ... ./..."
      And stderr has a line beginning "itos-cc: coverage: <dir>: measuring the whole module, as go list failed: " that names "no required module provides package example.com/m/nope"

    @slice-61 @ID-COV-50 @binary-only @wip
    Scenario: A Go file go list finds, and a run of the whole suite, say nothing of the whole module
      When I run "itos-cc crap a/a.go", and then "itos-cc crap --all-tests a/a.go"
      Then no line of stderr says "measuring the whole module"
