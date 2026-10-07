Feature: Test-code structure
  "Which test files should an agent leave alone, table-drive, refactor, or
  split?" scrap finds the examples (test cases) in each test file, scores
  each one, clusters examples that repeat each other, and gives the file one
  action for an assistant to follow.

  Rule: Finding examples

    Scenario Outline: Test frameworks
      Given a <language> test file using <framework>
      When I run "itos-cc scrap"
      Then each test case is an example, grouped by its enclosing <group>

      Examples:
        | language   | framework             | group                         |
        | TypeScript | Vitest, Jest, Mocha   | describe, context, or suite block |
        | Python     | pytest and unittest   | test class                        |
        | Go         | testing               | parent test, for t.Run subtests   |
        | Kotlin     | JUnit and kotest      | test class, or kotest describe, context, feature, or given block |

    Scenario: Go subtests and table loops
      Given a Go test that loops over a table of cases and asserts in the loop body
      When it is measured
      Then it is marked as a table
      And the loop does not count as logic

    Scenario: Shared setup is counted separately
      Given a test file with beforeEach, fixtures, setUp, or @BeforeEach
      When it is measured
      Then those lines count as setup, not as any example's size

    Scenario: A Kotlin test class with several annotations
      Given the Kotlin test file "ApiTest.kt" with a class annotated @SpringBootTest and @AutoConfigureMockMvc
      And a @Test method "lists" with one assertEquals
      When it is measured
      Then "ApiTest/lists" is an example with 1 assertion

  Rule: Measuring examples

    Scenario: Assertion helpers count as assertions
      Given the Go test file "x_test.go":
        """
        package x

        func assertParsed(t *testing.T, in string, want int) {
        	t.Helper()
        	if got := Parse(in); got != want {
        		t.Errorf("Parse(%q) = %d", in, want)
        	}
        }

        func TestParse(t *testing.T) {
        	assertParsed(t, "1", 1)
        }

        func TestCases(t *testing.T) {
        	for in, want := range map[string]int{"1": 1, "2": 2} {
        		if got := Parse(in); got != want || got < 0 {
        			t.Errorf("Parse(%q) = %d", in, got)
        		}
        	}
        }

        func TestCheckout(t *testing.T) {
        	checkout(cart)
        }
        """
      When it is measured
      Then the examples are:
        | example      | assertions | decisions | mocks | table |
        | TestParse    | 1          | 0         | 0     | no    |
        | TestCases    | 1          | 0         | 0     | yes   |
        | TestCheckout | 0          | 0         | 0     | no    |
      # assertParsed asserts, so calling it is an assertion; checkout is not a helper

    Scenario: A setup helper that fails on its own errors is not an assertion
      Given a Go helper "write" whose only failure is if err := os.WriteFile(…); err != nil { t.Fatal(err) }
      And an example that calls write twice and an asserting helper once
      When it is measured
      Then it has 1 assertion
      # the guard checks the setup worked, not the behavior under test

    Scenario Outline: Assertions each language recognizes
      Given a <language> example containing <code>
      Then it counts as an assertion

      Examples:
        | language   | code                                   |
        | TypeScript | expect(x).toBe(1)                      |
        | TypeScript | assert.equal(x, 1)                     |
        | Python     | assert x == 1                          |
        | Python     | with pytest.raises(ValueError):        |
        | Go         | t.Errorf("got %d", x)                  |
        | Go         | require.NoError(t, err)                |
        | Kotlin     | x shouldBe 1                           |
        | Kotlin     | coVerify { repo.save(any()) }          |

    Scenario Outline: Helper names that read as assertions
      Given a test calls a helper named "<name>" defined outside the file
      Then the call <counts> as an assertion

      Examples:
        | name        | counts       |
        | assertValid | counts       |
        | expect_ok   | counts       |
        | checkLimits | counts       |
        | verify      | counts       |
        | mustParse   | counts       |
        | checkout    | does not count |

    Scenario: Fixture text is not test size
      Given a pytest example whose body is a 40-line triple-quoted string and one assertion
      When it is measured
      Then it spans 44 raw lines but only 3 code lines

    Scenario Outline: A case table is not test size
      Given a test whose 44 cases are <table>
      When it is measured
      Then it is a table of a few code lines, not a large example
      And scrap does not ask to split it

      Examples:
        | table                                                                  |
        | a Go []struct literal it ranges over, inline or named, with or without t.Run |
        | a Python list it loops over, or a parametrize decorator                |
        | a TypeScript array it loops over with for…of, or it.each's argument    |
        | a Kotlin listOf it loops over, or a @CsvSource                         |

    Scenario Outline: Scoring an example
      Given an example with <lines> code lines, <decisions> branches or loops, <mocks> mocks, and <assertions> assertions
      When it is scored
      Then its score is <score>

      Examples:
        | lines | decisions | mocks | assertions | score |
        | 8     | 0         | 0     | 2          | 1     |
        | 8     | 0         | 0     | 0          | 7     |
        | 35    | 2         | 4     | 1          | 13    |
      # 1, plus (lines − 15) / 5, plus 2.5 per decision, plus 1.5 per mock beyond 2,
      # plus 6 with no assertions, plus (assertions − 10) / 2

    Scenario Outline: Smells
      Given an example with <measure>
      Then it has the smell "<smell>"

      Examples:
        | measure                  | smell           |
        | no assertions            | no-assertions   |
        | more than 30 code lines  | large           |
        | a branch or loop         | logic           |
        | 3 or more mocks          | mock-heavy      |
        | more than 10 assertions  | assertion-heavy |

  Rule: Choosing an action for the file

    Scenario: A clean file is left alone
      Given the Python test file "test_ok.py":
        """
        def test_adds():
            assert add(1, 2) == 3

        def test_parses():
            assert parse('x') == {'x': None}
        """
      When I run "itos-cc scrap test_ok.py"
      Then its action is LEAVE_ALONE
      And its pressure is 0
      And it has no recommendations

    Scenario: A coverage matrix should become a table
      Given the TypeScript test file "parse.test.ts":
        """
        import { it, expect } from "vitest";

        it("parses one", () => { expect(parse("1")).toBe(1); });
        it("parses two", () => { expect(parse("2")).toBe(2); });
        it("parses ten", () => { expect(parse("10")).toBe(10); });
        it("parses neg", () => { expect(parse("-4")).toBe(-4); });
        """
      When I run "itos-cc scrap parse.test.ts"
      Then its action is AUTO_TABLE_DRIVE
      And it has one "table" cluster of 4 examples
      And the first recommendation says to fold them into "it.each / test.each"

    Scenario Outline: The table idiom matches the language
      Given a <language> test file whose small examples differ only in their data
      Then the recommendation names <idiom>

      Examples:
        | language   | idiom                            |
        | TypeScript | it.each / test.each              |
        | Python     | @pytest.mark.parametrize         |
        | Go         | a table of cases run with t.Run  |
        | Kotlin     | @ParameterizedTest               |

    Scenario: An example that asserts nothing asks for refactoring
      Given the Python test file "test_x.py":
        """
        def test_runs():
            run()
        """
      When I run "itos-cc scrap test_x.py"
      Then its action is AUTO_REFACTOR
      And its first recommendation is HIGH confidence: add an assertion

    Scenario: Repeated scaffolding asks for a helper
      Given several large examples whose bodies are at least 85% alike
      When I run "itos-cc scrap"
      Then the file's action is AUTO_REFACTOR
      And a MEDIUM recommendation says to extract a helper or fixture

    Scenario: A mock-heavy file needs review first
      Given a test file with 4 examples, 3 of which set up 3 or more mocks
      When I run "itos-cc scrap"
      Then its action is REVIEW_FIRST

    Scenario: A large, troubled file should be split first
      Given a test file with 25 examples
      And 5 examples scoring 6 or more, spread across 3 groups
      When I run "itos-cc scrap"
      Then its action is MANUAL_SPLIT
      And a recommendation says to split the file by responsibility, then rerun scrap on each part

    Scenario: Recommendations are ranked and capped
      Given a test file with many problems
      When I run "itos-cc scrap"
      Then HIGH recommendations come before MEDIUM ones, and MEDIUM before LOW
      And at most 10 recommendations are listed
      And each names the example and its line range

  Rule: Output and progress over time

    Scenario: Text output
      When I run "itos-cc scrap"
      Then each file is printed as "file  ACTION  pressure=…  examples=…  avg=…  max=…"
      And files are listed from the highest pressure down
      And recommendations follow, indented, with their confidence

    Scenario: Files to leave alone stay quiet
      Given a file whose action is LEAVE_ALONE but has a LOW hint
      When I run "itos-cc scrap"
      Then the hint is not printed
      But it is printed with --verbose

    Scenario: Verbose output
      When I run "itos-cc scrap --verbose"
      Then every example is printed with its score, lines, assertions, decisions, mocks, and smells

    Scenario: JSON output
      When I run "itos-cc scrap --json"
      Then stdout lists each file's report with action, pressure, examples, recommendations, clusters, and details

    Scenario Outline: Comparing with the previous run
      Given .metrics/scrap.json recorded a pressure of <before> for a file
      When I run "itos-cc scrap" and its pressure is now <after>
      Then its verdict is <verdict>

      Examples:
        | before | after | verdict   |
        | 10     | 3     | improved  |
        | 3.2    | 3     | unchanged |
        | 3      | 4     | worse     |

    Scenario: A refactor that made things worse is called out
      Given a file's pressure rose since the previous run
      When I run "itos-cc scrap"
      Then its line ends with "worse from" and the previous pressure
      And a warning says to check new helpers and duplication before keeping the change

    Scenario: Snapshots keep files that were not part of this run
      Given .metrics/scrap.json holds reports for a.test.ts and b.test.ts
      When I run "itos-cc scrap a.test.ts"
      Then .metrics/scrap.json still holds b.test.ts's report

    Scenario: An unreadable previous snapshot is ignored
      Given .metrics/scrap.json is not valid JSON
      When I run "itos-cc scrap"
      Then stderr says it is ignoring the unreadable snapshot
      And the run completes

    Scenario: Nothing to measure
      Given the selection holds no test files
      When I run "itos-cc scrap"
      Then stderr says "itos-cc: no test files to measure"
      And the exit code is 0
