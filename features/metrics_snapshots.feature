Feature: Snapshots under .metrics
  Every command writes a JSON snapshot under .metrics/ that projects commit,
  so a clone has the numbers without rerunning and nobody reruns mutants that
  are already killed.

  @ID-SNAP-01
  Scenario Outline: Each command writes its snapshot
    When I run "itos-cc <command>"
    Then <snapshot> is written

    Examples:
      | command      | snapshot                         |
      | crap         | .metrics/crap.json               |
      | dry          | .metrics/dry.json                |
      | scrap        | .metrics/scrap.json              |
      | mutation run | .metrics/mutate/SOURCE_PATH.json |

  @ID-SNAP-02
  Scenario: An unchanged result is an unchanged file
    Given a command has written its snapshot
    When I run it again and nothing in the code has changed
    Then the snapshot file is byte-for-byte the same
    # snapshots carry no timestamps and are sorted

  @ID-SNAP-03
  Scenario: Every snapshot records its format version
    When any snapshot is written
    Then it has "version": 1

  @ID-SNAP-04
  Scenario: Readers never see half a snapshot
    When a snapshot is written
    Then it is written to a temporary file of its own and renamed into place
    And two runs writing the same snapshot at once leave one whole snapshot

  @ID-SNAP-05
  Scenario: Raw coverage is not committed
    When coverage is run
    Then raw reports go to .metrics/coverage/, which holds a .gitignore of "*"

  @ID-SNAP-06
  Scenario: Runs at the same time keep their coverage apart
    Given an agent runs "itos-cc mutation run" while a commit hook runs "itos-cc crap"
    When both run coverage
    Then each writes its reports to a run-* directory of its own under .metrics/coverage/
    And each reads only its own
    And the directory is deleted when the run ends, after its reports replace the previous run's
    # so --use-existing-coverage reads the latest run, and nothing accumulates

  @ID-SNAP-07
  Scenario: Directories of killed runs are cleaned up
    Given a run-* directory left by a run that was killed a day or more ago
    When coverage is run
    Then that directory is deleted

  # .metrics/ was relative to the working directory, and each snapshot named
  # its files from there, so where a command ran decided where its results
  # landed and what they were called: a run from src/ wrote src/.metrics/,
  # which check from the root never read, and the graph matched snapshots by
  # path suffix to paper over it (ID-GRAPH-26). Decided with the person on
  # 2026-10-07 (q-16): every command keeps .metrics/ at the project root,
  # and every path a snapshot records is relative to the root. The root is
  # the git top level of the working directory; outside a git repository it
  # is the working directory, as before (the coordinator's call). Paths on
  # the command line, file selection (ID-MUT-41) and the paths stdout and
  # --json print stay relative to the working directory. A .metrics/ left
  # in a subdirectory by an earlier version is no longer read or written.
  @wip @snapshots-at-root @ID-SNAP-08
  Scenario: Snapshots live at the project root, wherever a command runs
    Given a git repository with src/board.go and its tests
    When I run "itos-cc mutation run board.go" from src/
    Then the snapshot is .metrics/mutate/src/board.go.json at the repository's root, and it names the file "src/board.go"
    And no .metrics directory is created under src/

  @wip @snapshots-at-root @ID-SNAP-09
  Scenario Outline: Every command's snapshot names files from the root
    Given a git repository with src/board.go and its tests
    When I run "itos-cc <command>" from src/
    Then <snapshot> at the repository's root names src/board.go as "src/board.go"

    Examples:
      | command | snapshot           |
      | crap    | .metrics/crap.json |
      | dry     | .metrics/dry.json  |
      | scrap   | .metrics/scrap.json |

  @wip @snapshots-at-root @ID-SNAP-10
  Scenario: A command finds the same results from any directory
    Given a run from the repository's root recorded every mutant of src/board.go killed
    When I run "itos-cc mutation check board.go" from src/
    Then the exit code is 0
    And "itos-cc mutation check src/board.go" from the root exits 0 too

  @wip @snapshots-at-root @ID-SNAP-11
  Scenario: Paths on the command line and in output stay relative to the working directory
    Given a git repository with src/board.go and its tests
    When I run "itos-cc mutation run --json board.go" from src/
    Then stdout's summary names the file "board.go"
    And the file in "files" is "board.go"

  @wip @snapshots-at-root @ID-SNAP-12
  Scenario: Outside a git repository the working directory is the root
    Given a directory with board.go that is in no git repository
    When I run "itos-cc crap" there
    Then .metrics/crap.json is written in that directory, naming the file "board.go"
