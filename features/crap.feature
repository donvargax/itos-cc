Feature: CRAP scores
  "Which functions are complex and under-tested?" Each function scores
  CRAP = CC² × (1 − coverage)³ + CC. 1–5 is low risk, 5–30 is worth a look,
  and above 30 is complex and under-tested.

  Scenario Outline: The CRAP formula
    Given a function with complexity <cc> and <coverage>% coverage
    When it is scored
    Then its CRAP score is <crap>

    Examples:
      | cc | coverage | crap  |
      | 1  | 0        | 2.0   |
      | 5  | 100      | 5.0   |
      | 5  | 0        | 30.0  |
      | 10 | 50       | 22.5  |
      | 10 | 0        | 110.0 |

  Scenario: Scoring a project runs its tests with coverage first
    Given a project with tests
    When I run "itos-cc crap"
    Then each language's coverage command runs, with its output on stderr
    And stdout is a table with the columns CRAP, CC, and COV%
    And each row names the function as "namespace#name  file:line"
    And rows are sorted from the highest CRAP score down

  Scenario: Functions without coverage are listed last
    Given a function that no coverage report applies to
    When I run "itos-cc crap"
    Then its CRAP and COV% are shown as "N/A"
    And it is listed after every scored function, by complexity

  Scenario: A file the tests never load counts as uncovered
    Given coverage measured other Python files in the project
    And the report never mentions src/unused.py
    When I run "itos-cc crap"
    Then the functions of src/unused.py have 0% coverage

  Scenario: A language the tests did not measure is unknown, not uncovered
    Given the coverage report mentions no Kotlin file
    When I run "itos-cc crap"
    Then the Kotlin functions have no CRAP score

  Scenario: Only the worst functions
    When I run "itos-cc crap --top 20"
    Then only the 20 highest-scoring functions are printed
    But .metrics/crap.json still records every function

  Scenario: JSON output
    When I run "itos-cc crap --json"
    Then stdout is a snapshot with "version" and "entries"
    And each entry has namespace, name, language, file, start_line, end_line, complexity, coverage, and crap

  Scenario: Every run writes a snapshot
    When I run "itos-cc crap"
    Then .metrics/crap.json holds every scored function

  Scenario: Failing a build on a threshold
    Given a function scores 42.0
    When I run "itos-cc crap --threshold 30"
    Then stderr names the worst function and says it scores 42.0, above the threshold 30.0
    And the exit code is 2

  Scenario: Staying under the threshold
    Given every function scores 30 or less
    When I run "itos-cc crap --threshold 30"
    Then the exit code is 0

  Scenario Outline: A threshold is not passed by code coverage could not measure
    Given a Go project whose <problem>
    When I run "itos-cc crap --threshold 30"
    Then its functions show "N/A"
    And stderr says "itos-cc: no coverage for <dir> (go): <why>"
    And stderr says the threshold cannot be checked for code without coverage
    And the exit code is 4

    Examples:
      | problem                                         | why                                                              |
      | tests do not compile                            | its coverage run measured none of its files; go: exit status 1   |
      | report is missing, with --use-existing-coverage | no report on disk measures its files                             |
    # also a missing tool, such as Vitest that is not installed, and a
    # --coverage-report that cannot be read; N/A is not 0%: a broken test
    # setup is not untested code, but it must not pass the gate

  Scenario: Without a threshold, coverage that could not be measured is a warning
    Given a Go project whose tests do not compile
    When I run "itos-cc crap"
    Then stderr says "itos-cc: no coverage for <dir> (go): ..."
    And the exit code is 0

  Scenario: Nothing to score
    Given the selection holds no production source files
    When I run "itos-cc crap"
    Then stderr says "itos-cc: no source files to score"
    And the exit code is 0
