Feature: Coverage
  crap and mutate need to know which lines and branches the tests execute. itos-cc runs
  each language's own coverage tools per build root, or reads reports the
  user already has, and matches each report entry to a source file on disk.

  Rule: Each build root gets the language's own coverage command

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

    Scenario: The project's own package manager runs its scripts
      Given a TypeScript project in a workspace whose root declares "packageManager": "pnpm@9.12.0"
      When coverage is run for it
      Then <pm> is pnpm
      # otherwise the nearest lockfile decides (pnpm, yarn, bun, npm), and npm when there is none

    Scenario: Node tools come from node_modules, never from the registry
      Given a TypeScript project in a workspace, with its tools installed at the workspace root
      When coverage is run for it
      Then vitest, jest, or c8 runs from the nearest node_modules/.bin
      And npx is never used, so nothing that the project's lockfile does not pin is downloaded

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
      # the user makes the tool available; itos-cc never installs or downloads it

    Scenario: Only the current Vitest major is supported
      Given a TypeScript project using vitest 4.1.11
      When coverage is run for it
      Then nothing runs for it
      And stderr says "Vitest 4.1.11 is not supported; upgrade to Vitest 5 or later to measure coverage"
      And only complexity is reported for it
      # older Vitest writes V8 blocks, not branches; Jest's istanbul branches are the same in every version

    Scenario: Python uses the project's own virtualenv
      Given a Python project with a .venv directory
      When coverage is run for it
      Then the .venv's python runs the tests

    Scenario: A Gradle wrapper is preferred
      Given a Kotlin Gradle module under a build root that holds gradlew
      When coverage is run for it
      Then the gradlew wrapper is used instead of gradle

    Scenario: Several build roots and languages are measured separately and merged
      Given a repository with a Go module and a TypeScript package
      When coverage is run
      Then each build root runs its own coverage command
      And the reports are merged into one

    Scenario: Failing tests still contribute coverage
      Given a project whose tests fail but still write a coverage report
      When coverage is run
      Then the failure is logged to stderr
      And the report is still used

    Scenario: Raw reports stay out of version control
      When coverage is run
      Then reports are written under .metrics/coverage/
      And .metrics/coverage/.gitignore ignores everything in it

  Rule: The user can bring their own coverage

    Scenario: Reading reports already on disk
      Given a Go project with a coverage.out from an earlier run
      When I run "itos-cc crap --use-existing-coverage"
      Then no tests are run
      And coverage is read from the first existing report for each build root

    Scenario: Naming reports explicitly
      When I run "itos-cc crap --coverage-report a/lcov.info --coverage-report b/coverage.out"
      Then both reports are read, relative to the working directory

    Scenario: Running a custom coverage command
      When I run "itos-cc crap --coverage-command 'make cover' --coverage-report cover.out"
      Then "make cover" runs through the platform shell
      And cover.out is read afterwards

    Scenario: A custom command must say where its report lands
      When I run "itos-cc crap --coverage-command 'make cover'"
      Then stderr says "--coverage-command needs --coverage-report to say where the report lands"
      And the exit code is 1

    Scenario: Skipping coverage
      When I run "itos-cc crap --no-coverage"
      Then no tests are run
      And only complexity is reported

  Rule: Report formats are detected from their content

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

    Scenario Outline: Where branches come from
      Given a <format> report
      Then a decision is <decision>

      Examples:
        | format            | decision                                                 |
        | LCOV              | a BRDA block with two or more branches                   |
        | JaCoCo/Kover XML  | a line with branch counters, weighted by its branches    |
        | Go cover profile  | never: Go has no branch data, so statements decide       |

    Scenario: A function without branches is scored by its lines
      Given a function whose lines hold no decision in the report
      Then its coverage is the share of its lines or statements that ran

    Scenario: One-branch LCOV blocks are not decisions
      Given an LCOV report from c8, Node's test runner, or a Vitest older than 5
      And it lists the function body as a block and leaves out the arm that ran
      When the covered share of a function is computed
      Then those one-branch blocks are ignored and its lines decide

  Rule: Report paths are matched to source files

    Scenario: Exact matches
      Given a report entry whose path is absolute or relative to the report's base directory
      When it is matched to sources
      Then it matches that source file exactly

    Scenario: Matching by the longest shared tail
      Given a report entry "example.com/demo/board/board.go"
      And a source file "/work/demo/board/board.go"
      When it is matched to sources
      Then it matches that source by its trailing path components

    Scenario: A path that names a file on disk is that file
      Given sources "/p/b/main.go" only, as when --changed selects it
      And a report entry "a/b/main.go" or "/p/a/b/main.go", a file that exists
      When it is matched to sources
      Then it matches neither source
      # the tail "b/main.go" would lend another file's coverage to b/main.go

    Scenario: A module path names the file its tail finds under the project
      Given sources "metrics/metrics.go" only, and a project file "graph/metrics.go"
      And a Go profile entry "example.com/m/graph/metrics.go"
      When it is matched to sources
      Then it matches neither source
      # its longest tail that exists, graph/metrics.go, is not a source

    Scenario: A symlinked directory still matches
      Given a source reached through a symlinked directory
      And a report entry that names the same file by the symlink's target
      When it is matched to sources
      Then it matches that source

    Scenario: An ambiguous tail is not guessed
      Given sources "/p/a/util.py" and "/p/b/util.py"
      And a report entry "util.py" from a base directory that holds neither
      When it is matched to sources
      Then it matches neither source

    Scenario: Separators do not matter
      Given a report written with "\" separators and sources with "/" separators, or the reverse
      When it is matched to sources
      Then the paths still match

    Scenario: Entries for dependencies, tests, and generated code are dropped
      Given a report entry that matches no selected source
      When the report is built
      Then that entry is ignored

  Rule: A piece of code listed more than once counts once

    Scenario: Go test binaries each list every block
      Given a module with packages "a", "b", and "c", each fully covered by its own tests
      And a profile from go test -coverpkg=./..., which lists every block once per test binary
      When the covered share of a function in "a" is computed
      Then it is 100%, as go tool cover -func says
      # counting each copy would make it 33%

    Scenario: Several reports of the same file are combined
      Given two reports given with --coverage-report, such as unit and integration runs
      And each covers a line of a file the other does not
      When the covered share of the file is computed
      Then a line or block is covered when any report covered it
      And a decision counts the branches of the report that took the most

  Rule: A function's coverage starts where calling it starts

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
