Feature: Measuring a past commit
  "What did units, dry and scrap say at that commit?" With --at <commit>, a command
  that reads sources alone measures the commit's tree as if it were checked out, read
  from git's objects: the working tree is never read and never changed, and the
  commit's own itos-cc.yaml is the settings. Each result is kept per commit under the
  git common directory, so every worktree of the repository shares it.

  Background:
    Given the project is a git repository
    And "src/a.ts" holds:
      """
      export function alpha(xs: number[]): number[] {
        const a = xs.filter((x) => x > 1);
        const b = a.map((x) => x * 2);
        const c = b.filter((x) => x < 100);
        return c.map((x) => x + 1);
      }

      export function beta(items: number[]): number[] {
        const kept = items.filter((i) => i > 1);
        const doubled = kept.map((i) => i * 2);
        const small = doubled.filter((i) => i < 100);
        return small.map((i) => i + 1);
      }
      """
    And "src/skip.ts" holds:
      """
      export function skipped(): number {
        return 1;
      }
      """
    And "src/a.test.ts" holds:
      """
      test("adds", () => {
        expect(1 + 1).toBe(2);
      });

      test("subtracts", () => {
        expect(2 - 1).toBe(1);
      });
      """
    And "itos-cc.yaml" holds:
      """
      exclude:
        - src/skip.ts
      """
    And everything is committed
    And "src/a.ts" holds:
      """
      export function gamma(): number {
        return 3;
      }
      """
    And "src/a.test.ts" holds:
      """
      test("only", () => {
        expect(1).toBe(1);
      });
      """
    And "itos-cc.yaml" holds:
      """
      exclude: []
      """
    And everything is committed
    And "src/wip.ts" holds:
      """
      export function wip(): number {
        return 4;
      }
      """

  @T-250 @ID-HIST-01
  Scenario: units at a past commit lists that commit's units, not the working tree's
    When itos-cc runs with "units --at HEAD~1"
    Then it exits with code 0
    And its standard output says "a#alpha"
    And its standard output says "a#beta"
    And its standard output does not say "gamma"
    And its standard output does not say "wip"
    And its standard output does not say "skipped"

  @T-250 @ID-HIST-02
  Scenario: units at a past commit prints the same object --json always does
    When itos-cc runs with "units --at HEAD~1 --json"
    Then it exits with code 0
    And stdout is one object with "schema": 1, "ok", and "units"
    And its standard output says "alpha"
    And its standard output does not say "gamma"

  @T-250 @ID-HIST-03
  Scenario: dry at a past commit reports that commit's duplicates and records nothing
    When itos-cc runs with "dry --at HEAD~1"
    Then it exits with code 0
    And its standard output says "DUPLICATE score=1.00 typescript"
    And its standard output says "src/a.ts:1-6  a#alpha"
    And the project holds no ".metrics/dry.json"

  @T-250 @ID-HIST-04
  Scenario: scrap at a past commit measures that commit's tests and records nothing
    When itos-cc runs with "scrap --at HEAD~1"
    Then it exits with code 0
    And its standard output says "src/a.test.ts"
    And its standard output says "examples=2"
    And the project holds no ".metrics/scrap.json"

  @T-250 @ID-HIST-05
  Scenario: the commit's own itos-cc.yaml is the settings
    When itos-cc runs with "units --at HEAD~1"
    Then its standard output does not say "skipped"
    And its error output says "1 sources left out by exclude in itos-cc.yaml"
    When itos-cc runs with "units"
    Then its standard output says "skip#skipped"
    And its error output does not say "left out"

  @T-250 @ID-HIST-06
  Scenario: a path selects within the commit's tree
    When itos-cc runs with "units --at HEAD~1 src/a.ts"
    Then it exits with code 0
    And its standard output says "a#alpha"
    When itos-cc runs with "units --at HEAD~1 src/wip.ts"
    Then the problem is "paths.unmatched", with argument "src/wip.ts"

  @T-250 @ID-HIST-07
  Scenario: the result is kept under the git common directory
    When itos-cc runs with "units --at HEAD~1"
    Then the git common directory holds one kept result of "units"

  @T-250 @ID-HIST-08
  Scenario: a corrupt kept result is measured again, never a failure
    Given itos-cc runs with "units --at HEAD~1"
    And the kept result of "units" is overwritten with "{not json"
    When itos-cc runs with "units --at HEAD~1"
    Then it exits with code 0
    And its standard output says "a#alpha"
    And the kept result of "units" is valid JSON

  @T-250 @ID-HIST-09 @binary-only
  Scenario: a linked worktree reuses the result another worktree kept
    Given itos-cc runs with "units --at HEAD~1"
    And a linked worktree of the project
    When itos-cc runs in the linked worktree with "units --at HEAD~1"
    Then it exits with code 0
    And its standard output says "a#alpha"
    And the git common directory still holds the one kept result of "units", as it was

  @T-250 @ID-HIST-10
  Scenario Outline: commands that run tests refuse --at
    When itos-cc runs with "<command> --at HEAD~1"
    Then it exits with code 80
    And the problem is "at.unsupported"
    And its message says "<command>"

    Examples:
      | command         |
      | crap            |
      | mutation run    |
      | mutation list   |
      | mutation check  |
      | mutation sample |

  @T-250 @ID-HIST-11
  Scenario: a revision git cannot name is refused
    When itos-cc runs with "units --at no-such-revision"
    Then it exits with code 80
    And the problem is "at.bad-rev"
    And its message says "no-such-revision"

  @T-250 @ID-HIST-12
  Scenario: --at and --changed cannot be used together
    When itos-cc runs with "units --at HEAD~1 --changed"
    Then it exits with code 80
    And the problem is "flags.conflict", with flag "--at"

  @T-250 @ID-HIST-13
  Scenario: --at needs a git repository
    Given a folder in no git repository holding "src/a.ts"
    When itos-cc runs with "units --at HEAD"
    Then it exits with code 82
    And the problem is "at.no-git"
