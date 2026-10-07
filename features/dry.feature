Feature: Duplicate functions
  "Which functions are the same code with the names changed?" Each function's
  syntax tree is normalized and fingerprinted, and pairs in the same language
  are compared by the Jaccard similarity of their fingerprints.

  Scenario: Defaults
    When I run "itos-cc dry" without options
    Then pairs scoring at least 0.82 are reported
    And functions shorter than 4 lines or with fewer than 20 normalized nodes are not candidates

  Scenario Outline: Renamed code scores 1
    Given the <language> functions:
      """
      <first>
      <second>
      """
    When the two functions are compared directly
    Then their similarity is 1.00

    Examples:
      | language   | first                                                                        | second                                                                                 |
      | TypeScript | function alpha(xs) { const ys = filter(xs, odd); return map(ys, 1); }        | function beta(items) { const kept = filter(items, even); return map(kept, 2); }        |
      | Go         | func alpha(xs []int) []int { ys := filter(xs, odd); return mapf(ys, 1) }     | func beta(items []int) []int { kept := filter(items, even); return mapf(kept, 2) }     |
      | Kotlin     | fun alpha(xs: List<Int>): List<Int> { val ys = filter(xs, odd); return map(ys, 1) } | fun beta(items: List<Int>): List<Int> { val kept = filter(items, even); return map(kept, 2) } |

  Scenario: Field names do not matter
    Given the TypeScript functions:
      """
      function f(order) { return order.total * 2; }
      function g(row) { return row.amount * 3; }
      """
    When the two functions are compared directly
    Then their similarity is 1.00

  Scenario Outline: Called functions and operators do matter
    Given the TypeScript function "function f(xs) { const ys = filter(xs, odd); return map(ys, inc); }"
    And a copy of it with <change>
    When the two functions are compared directly
    Then their similarity is above 0 and below 1

    Examples:
      | change                                   |
      | reject called instead of filter          |
      | " - 1" subtracted from the return value  |

  Scenario: Comments do not count
    Given the Python functions:
      """
      def f(x):
          return g(x)

      def f(x):
          # explain
          return g(x)  # why
      """
    When the two functions are compared directly
    Then their similarity is 1.00

  Scenario: Unrelated code scores low
    Given the TypeScript functions:
      """
      function f(xs) { for (const x of xs) { if (x > 0) { total += x; } } return total; }
      function g(s) { return fetch(url(s)).then(parse).catch(report); }
      """
    When the two functions are compared directly
    Then their similarity is at most 0.3

  Scenario: Near copies are reported
    Given invoice.py renders an invoice's lines and total
    And receipt.py renders a receipt the same way, plus one footer line
    And parse.py holds an unrelated parser
    When I run "itos-cc dry"
    Then exactly one pair is reported: invoice.py#render and receipt.py#render
    And its score is at least 0.82 but below 1

  Scenario: Only functions in the same language are compared
    Given a TypeScript function and a Python function with the same logic
    When I run "itos-cc dry"
    Then they are not compared

  Scenario: Small functions are not candidates
    Given two identical 2-line functions
    When I run "itos-cc dry"
    Then they are not reported

  Scenario: Text output
    Given two functions that score 0.95 in TypeScript
    When I run "itos-cc dry"
    Then stdout shows:
      """
      DUPLICATE score=0.95 typescript
        src/a.ts:3-12  a#total
        src/b.ts:7-16  b#sum
      """
    And pairs are listed best first

  Scenario: Copies of one function are one group
    Given four copies of one helper, every pair of them scoring between 0.90 and 1.00
    When I run "itos-cc dry"
    Then stdout shows one entry, not six pairs:
      """
      DUPLICATE score=0.90–1.00 go
        a.go:1-9  a#f
        b.go:1-9  b#f
        c.go:1-9  c#f
        d.go:1-9  d#f
      """
    And a group holds every function linked to another member by a pair at or above the threshold
    And groups are listed best first, then largest first

  Scenario: Checking a change against the whole project
    Given src/new.ts duplicates a function in src/old.ts
    And src/x.ts and src/y.ts duplicate each other
    When I run "itos-cc dry src/new.ts"
    Then the pair from src/new.ts and src/old.ts is reported
    But the pair from src/x.ts and src/y.ts is not reported

  Scenario: Checking changed files
    When I run "itos-cc dry --changed"
    Then only pairs with at least one function in a changed file are reported
    And those functions are compared against every source in the project

  Scenario: Tuning what counts as a duplicate
    When I run "itos-cc dry --threshold 0.9 --min-lines 8 --min-nodes 40"
    Then only pairs scoring at least 0.9 among functions of 8+ lines and 40+ nodes are reported

  Scenario: Snapshot and JSON
    When I run "itos-cc dry --json"
    Then stdout and .metrics/dry.json hold the version, the threshold, the candidates, and the groups
    And each candidate has score, language, and left and right sides with file, lines, namespace, name, and nodes
    And each group has language, min_score, max_score, and its members

  Scenario: No duplicates is an empty list
    Given no pair reaches the threshold
    When I run "itos-cc dry --json"
    Then "candidates" and "groups" are empty lists, not null

  Scenario: A focus with no source files
    When I run "itos-cc dry docs"
    And no source path contains "docs"
    Then stderr says "itos-cc: no source files to check"
    And the exit code is 0
