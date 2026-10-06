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
        | TypeScript | package.json     | a "coverage" script                          | npm run coverage, read from coverage/lcov.info                    |
        | TypeScript | package.json     | vitest 5 or later as a dependency            | npx vitest run with the v8 coverage provider writing LCOV         |
        | TypeScript | package.json     | jest as a dependency                         | npx jest --coverage writing LCOV                                  |
        | TypeScript | package.json     | neither vitest nor jest                      | npx c8 around npm test, writing LCOV                              |
        | Python     | pyproject.toml   | pytest importable                            | coverage run --branch -m pytest, then coverage lcov               |
        | Python     | setup.py         | pytest not importable                        | coverage run --branch -m unittest discover, then coverage lcov    |
        | Kotlin     | build.gradle.kts | the build mentions kover                     | gradle koverXmlReport                                             |
        | Kotlin     | build.gradle     | the build does not mention kover             | gradle test jacocoTestReport                                      |
        | Kotlin     | pom.xml          | nothing else                                 | mvn with the jacoco-maven-plugin prepare-agent, test, and report  |

    Scenario: Vitest without its coverage provider
      Given a TypeScript project using vitest 5.0.2 without @vitest/coverage-v8 installed
      When coverage is run for it
      Then "npm install --no-save @vitest/coverage-v8@5.0.2" runs first
      And package.json is not modified

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
