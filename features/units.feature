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

  @ID-UNIT-06
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
