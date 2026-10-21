Feature: Units: the functions and methods every tool measures
  Every tool works on the same units, named by namespace and function name,
  so results from crap, dry, mutation run, and the graph join on the same key.

  @ID-UNIT-01
  Scenario: Listing production units
    When I run "itos-cc units"
    Then stdout lists each unit as "file:start-end  namespace#name  kind"

  @ID-UNIT-02
  Scenario: Listing production units as JSON
    When I run "itos-cc units --json"
    Then stdout is one object with "schema": 1, "ok", and "units"
    And each unit has file, language, namespace, name, kind, private, start_line, and end_line

  @ID-UNIT-03
  Scenario: Listing test code instead
    When I run "itos-cc units --tests"
    Then only units from test files are listed

  @ID-UNIT-04
  Scenario: No units is an empty list
    Given the selection holds no supported files
    When I run "itos-cc units --json"
    Then "units" is an empty list

  @ID-UNIT-05
  Scenario: TypeScript units
    Given the file "src/demo/board.ts":
      """
      import { x } from "./x";

      export function place(b: Board): void {
        const inner = () => 1;
        [1, 2].forEach((n) => n + 1);
      }

      export const score = (b: Board): number => b.cells.length;
      const helper = function () { return 1; };

      describe("board", () => {
        const notAUnit = () => 2;
      });

      export abstract class Board {
        cells: number[] = [];
        abstract draw(): void;
        constructor() {}
        public place(n: number) { this.cells.push(n); }
        private clear() { this.cells = []; }
        #reset() {}
        onClick = () => this.clear();
      }
      """
    When its units are listed
    Then the units are:
      | kind     | namespace        | name        | lines | private |
      | function | demo.board       | place       | 3-6   | no      |
      | function | demo.board       | score       | 8-8   | no      |
      | function | demo.board       | helper      | 9-9   | no      |
      | method   | demo.board.Board | constructor | 18-18 | no      |
      | method   | demo.board.Board | place       | 19-19 | no      |
      | method   | demo.board.Board | clear       | 20-20 | yes     |
      | method   | demo.board.Board | #reset      | 21-21 | yes     |
      | method   | demo.board.Board | onClick     | 22-22 | no      |
    And nested arrow functions, callbacks, and abstract methods are not units

  # Binary only (decision 44): its steps measure the routes with crap and
  # mutation run, still hand-parsed, which reach the disk themselves.
  @ID-UNIT-06 @binary-only
  Scenario: Inline route callbacks are units of their own
    Given the file "src/api/routes.js":
      """
      const app = express();
      app.use((req, res, next) => next());
      app.get("/users", (req, res) => {
        if (req.query.all) { res.json([]); }
      });

      export function mount(router) {
        router.post("/users", function (req, res) {
          return req.body ?? res.sendStatus(400);
        });
        router.route("/items").get((req, res) => res.json([])).delete(async (req, res) => {
          if (!req.params.id) return;
        });
        router.get("/users", (req, res) => res.json(req.user));
        return router;
      }
      """
    When its units are listed
    Then the units are:
      | kind     | namespace  | name           | lines | private |
      | function | api.routes | USE            | 2-2   | no      |
      | function | api.routes | GET /users     | 3-5   | no      |
      | function | api.routes | mount          | 7-16  | no      |
      | function | api.routes | POST /users    | 8-10  | no      |
      | function | api.routes | GET /items     | 11-11 | no      |
      | function | api.routes | DELETE /items  | 11-13 | no      |
      | function | api.routes | GET /users#2   | 14-14 | no      |
    And get, post, put, patch, delete, head, options, all, and use on any object are routes
    And a route's complexity, mutation sites, duplication, coverage, and hash are its own, not mount's
    And the line a route starts on is mount's, since mounting runs it
    But test files have no route units

  @ID-UNIT-07
  Scenario: TSX files are parsed with the TSX grammar
    Given the file "src/ui/cell.tsx" with a component that returns JSX
    When its units are listed
    Then the component "ui.cell#Cell" is a unit

  @ID-UNIT-08
  Scenario: Python units
    Given the file "src/demo/board.py":
      """
      import os

      def place(board, n):
          def inner():
              return n
          return inner()

      @cache
      def _hidden():
          pass

      class Board:
          def __init__(self):
              self.cells = []

          @property
          def size(self):
              return len(self.cells)

          def _clear(self):
              self.cells = []

          class Cell:
              def value(self):
                  return 0
      """
    When its units are listed
    Then the units are:
      | kind     | namespace             | name     | lines | private |
      | function | demo.board            | place    | 3-6   | no      |
      | function | demo.board            | _hidden  | 9-10  | yes     |
      | method   | demo.board.Board      | __init__ | 13-14 | no      |
      | method   | demo.board.Board      | size     | 17-18 | no      |
      | method   | demo.board.Board      | _clear   | 20-21 | yes     |
      | method   | demo.board.Board.Cell | value    | 24-25 | no      |

  @ID-UNIT-09
  Scenario: A Python package's __init__ is named after the package
    Given the file "src/demo/__init__.py" defining "main"
    When its units are listed
    Then the unit is "demo#main"

  @ID-UNIT-10
  Scenario: Kotlin units
    Given the file "src/main/kotlin/demo/game/Board.kt":
      """
      package demo.game

      import kotlin.math.max

      fun place(board: Board, n: Int) {
          fun inner() = n
          listOf(1).forEach { it + 1 }
      }

      private fun hidden() = 1

      class Board {
          private val cells = mutableListOf<Int>()

          fun place(n: Int) { cells.add(n) }

          private fun clear() { cells.clear() }

          companion object {
              fun empty() = Board()
          }
      }

      object Rules {
          internal fun max() = 9
      }
      """
    When its units are listed
    Then the units are:
      | kind     | namespace                 | name   | lines | private |
      | function | demo.game                 | place  | 5-8   | no      |
      | function | demo.game                 | hidden | 10-10 | yes     |
      | method   | demo.game.Board           | place  | 15-15 | no      |
      | method   | demo.game.Board           | clear  | 17-17 | yes     |
      | method   | demo.game.Board.Companion | empty  | 20-20 | no      |
      | method   | demo.game.Rules           | max    | 25-25 | no      |

  @ID-UNIT-11
  Scenario: A Kotlin class with several annotations keeps its methods
    Given a Kotlin class annotated with both @Configuration and @EnableWebSecurity
    When its units are listed
    Then its method is a unit of that class

  @ID-UNIT-12
  Scenario: Go units are namespaced by module import path
    Given the Go module "example.com/demo" with package "board"
    When the units of board/board.go are listed
    Then the units are:
      | kind     | namespace                    | name   | private |
      | function | example.com/demo/board       | New    | no      |
      | method   | example.com/demo/board.Board | Place  | no      |
      | method   | example.com/demo/board.Board | size   | yes     |
      | function | example.com/demo/board       | helper | yes     |

  # A source file outside any project root (no package.json, go.mod or .git
  # in its directory or above it) was namespaced by its directory's absolute
  # path, dotted (.tmp.….x#place), so function IDs, and the mutation results
  # and debt keyed by them, depended on where the checkout lives. The
  # working directory stands in for the root (the coordinator's call,
  # 2026-10-07, as the idea proposed).
  @namespace-working-dir @ID-UNIT-13
  Scenario: A file outside any project root is named from the working directory
    Given x.ts declares function place, in a directory with no package.json, go.mod or .git in it or above it
    When I run "itos-cc units x.ts" from that directory
    Then the unit is "x#place"
    And it is "x#place" from a copy of that directory at another path

  # Before units moved onto kong (T-32), units ---changed was accepted and
  # exited 0. An unknown flag is a usage error, the person's call
  # (2026-10-11), and the fix suggests the flag it looks like. The product
  # already behaves so: a guardrail, with no red to show. The refactor that
  # changed it carried no release note, so the scenario goes live in a fix
  # commit whose Upgrading footer says units ---changed now fails with exit 2.
  @slice-5 @ID-UNIT-14
  Scenario: A flag spelled with three dashes is an unknown flag
    When I run "itos-cc units ---changed"
    Then the problem is "flags.unknown", with flag "---changed"
    And its fix suggests "--changed"
    And the exit code is 80

  # slice-6, issue #16 and decision 45: itos-cc.yaml's exclude leaves the
  # sources it names out of units, but never a test: units --tests still
  # lists the test files under an excluded folder.
  @slice-6 @ID-UNIT-15
  Scenario: Sources that itos-cc.yaml excludes are not listed, their tests are
    Given itos-cc.yaml excludes "features/**"
    And the file "features/steps.go":
      """
      package features

      func step() int { return 1 }
      """
    And the file "features/steps_test.go":
      """
      package features

      import "testing"

      func TestStep(t *testing.T) { _ = step() }
      """
    When I run "itos-cc units features"
    Then no unit of features/steps.go is listed
    When I run "itos-cc units --tests features"
    Then a unit of features/steps_test.go is listed

  # slice-28, dart-support (q-74: Flutter apps and packages). Dart's grammar
  # gives a function a signature and a body as two siblings: a unit runs from
  # its signature's line, never an annotation's, to its body's end. Only a
  # declaration with a body, a block or =>, is a unit: abstract and external
  # members and a constructor ending in ; are none, and closures and local
  # functions count in the unit around them. The namespace is the nearest
  # pubspec.yaml's name and the path under lib/, as a package: URI names the
  # library; constructors are C and C.named, a setter s=, an operator
  # operator+; an unnamed extension's members sit in the file's namespace.
  # A unit is private when its own name, or a named constructor's part after
  # the dot, starts with _: build in _CounterState is not.
  @slice-28 @ID-UNIT-16
  Scenario: Dart units
    Given the file "pubspec.yaml":
      """
      name: demo
      dependencies:
        flutter:
          sdk: flutter
      """
    And the file "lib/game/board.dart":
      """
      import 'package:flutter/widgets.dart';

      int place(List<int> board, int n) {
        int inner() => n;
        board.forEach((c) => c + 1);
        return inner();
      }

      int _hidden() => 1;

      external int native();

      abstract class Shape {
        double area();
      }

      class Board extends Shape {
        Board(this.cells);

        Board.empty() : cells = [] {
          cells.add(0);
        }

        factory Board.of(int n) => Board([n]);

        final List<int> cells;

        @override
        double area() => cells.length.toDouble();

        int get size => cells.length;

        set size(int n) {
          cells.length = n;
        }

        Board operator +(Board other) => Board([...cells, ...other.cells]);

        void _clear() {
          cells.clear();
        }
      }

      class Rules {
        Rules() {
          print('ready');
        }
      }

      mixin Walker {
        int walk(int steps) => steps;
      }

      extension Score on Board {
        int score() => size * 2;
      }

      extension on int {
        bool get even => this % 2 == 0;
      }

      enum Side {
        left,
        right;

        Side get other => this == left ? right : left;
      }

      extension type Cell(int value) {
        bool get empty => value == 0;
      }

      class Counter extends StatefulWidget {
        const Counter({super.key});

        @override
        State<Counter> createState() => _CounterState();
      }

      class _CounterState extends State<Counter> {
        int _n = 0;

        @override
        Widget build(BuildContext context) {
          return GestureDetector(
            onTap: () => setState(() => _n++),
            child: Text('$_n'),
          );
        }
      }
      """
    When the units of lib/game/board.dart are listed
    Then the units are:
      | kind     | namespace                     | name        | lines | private |
      | function | demo.game.board               | place       | 3-7   | no      |
      | function | demo.game.board               | _hidden     | 9-9   | yes     |
      | method   | demo.game.board.Board         | Board.empty | 20-22 | no      |
      | method   | demo.game.board.Board         | Board.of    | 24-24 | no      |
      | method   | demo.game.board.Board         | area        | 29-29 | no      |
      | method   | demo.game.board.Board         | size        | 31-31 | no      |
      | method   | demo.game.board.Board         | size=       | 33-35 | no      |
      | method   | demo.game.board.Board         | operator+   | 37-37 | no      |
      | method   | demo.game.board.Board         | _clear      | 39-41 | yes     |
      | method   | demo.game.board.Rules         | Rules       | 45-47 | no      |
      | method   | demo.game.board.Walker        | walk        | 51-51 | no      |
      | method   | demo.game.board.Score         | score       | 55-55 | no      |
      | method   | demo.game.board               | even        | 59-59 | no      |
      | method   | demo.game.board.Side          | other       | 66-66 | no      |
      | method   | demo.game.board.Cell          | empty       | 70-70 | no      |
      | method   | demo.game.board.Counter       | createState | 77-77 | no      |
      | method   | demo.game.board._CounterState | build       | 84-89 | no      |

  # slice-28: a file outside lib/ is named by its path from the package, the
  # directory holding the nearest pubspec.yaml, under the pubspec's name.
  @slice-28 @ID-UNIT-17
  Scenario: A Dart test file is named from its path in the package
    Given the file "app/pubspec.yaml":
      """
      name: demo
      """
    And the file "app/test/game/board_test.dart":
      """
      void main() {
        print('board');
      }
      """
    When I run "itos-cc units --tests app/test"
    Then the unit is "demo.test.game.board_test#main"
