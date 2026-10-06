Feature: Architecture viewer
  The viewer in viewer/ draws the graph from "itos-cc serve" as a canvas you
  drill into: repositories, then directories, then modules, then functions.

  Background:
    Given the viewer is connected to a server with this graph:
      | node                              | kind   |
      | shop                              | repo   |
      | shop/api                          | module |
      | shop/api/store                    | module |
      | shop/web                          | dir    |
      | shop/web/src                      | dir    |
      | shop/web/src/app                  | module |
      | shop/web/src/cart                 | module |
      | shop/web/src/format               | module |
      | shop/app                          | dir    |
      | shop/app/src                      | dir    |
      | shop/app/src/main                 | dir    |
      | shop/app/src/main/com             | dir    |
      | shop/app/src/main/com/acme        | dir    |
      | shop/app/src/main/com/acme/Invoice | module |
      | shop/app/src/main/com/acme/Money  | module |
    And these edges:
      | from                | to                  | kind   | count | via                 |
      | shop/web/src/app    | shop/web/src/cart   | import | 2     |                     |
      | shop/web/src/cart   | shop/web/src/format | import | 1     |                     |
      | shop/web/src/format | shop/web/src/cart   | import | 1     |                     |
      | shop/web/src/app    | shop/api/store      | import | 1     |                     |
      | shop/api            | shop/api/store      | import | 3     |                     |
      | shop/web/src/app    | shop/api            | http   | 2     | GET /api/invoices/* |
      | shop/api/store      | shop/web/src/app    | http   | 1     | POST /hooks         |

  Rule: What a level shows

    Scenario: A lone repository opens directly
      When the viewer starts
      Then it shows the inside of "shop"

    Scenario: The system level lists repositories
      When I go up to the system level
      Then the boxes are "shop"

    Scenario: Single-child directory chains collapse into one box
      When I look at "shop"
      Then the boxes are "api", "app/src/main/com/acme", and "web/src"
      And opening "app/src/main/com/acme" goes straight to "shop/app/src/main/com/acme"

    Scenario: Dependencies roll up to the boxes on the current level
      When I look at "shop"
      Then the arrows are:
        | kind   | from     | to       | count |
        | import | web/src  | api      | 1     |
        | http   | web/src  | api      | 2     |
        | http   | api      | web/src  | 1     |

    Scenario: A package's own code sits beside its subpackages
      When I open "shop/api"
      Then the boxes are "api (own code)" and "store"
      And an import arrow with count 3 goes from "api (own code)" to "store"

    Scenario: Dependencies from outside the level are counted
      When I open "shop/api"
      Then "store" shows 1 import coming from outside this level

    Scenario: Cycles are red at every level
      When I open "shop/web/src"
      Then the arrows between "cart" and "format", in both directions, are marked as a cycle
      And the boxes "cart" and "format" carry a cycle badge
      But the arrow from "app" is not part of a cycle

    Scenario: HTTP calls are not dependency cycles
      When I look at "shop"
      Then "web/src" and "api" call each other over HTTP
      But no arrow is marked as a cycle

    Scenario: Boxes sit above what they depend on
      When a level is laid out
      Then each box is placed above the boxes it imports
      And arrows are routed with rounded corners

  Rule: Color and badges

    Scenario Outline: Box color follows the grade
      Given a box graded <grade>
      Then its hue is <hue>

      Examples:
        | grade | hue |
        | 1     | 0   |
        | 5.5   | 65  |
        | 10    | 130 |

    Scenario: Unmeasured boxes are grey
      Given a box without a grade
      Then it has no hue and is drawn grey

    Scenario: A box summarizes its functions
      Given a box with 53 functions, 6 of them mutation-tested, and every mutant run killed
      Then it shows "53 fn", its worst CRAP, and "mut 100% · 6/53"
      And a bar splits its functions into risky, worth a look, low risk, and unmeasured

    Scenario: Badges call out what needs attention
      Given a box with surviving mutants, stale functions, and duplicates
      Then it shows "N survived", "N stale", and "N dup" badges
      And a box that can be opened shows "open ↵"

    Scenario: Live updates point at what moved
      Given a box is on screen
      When its numbers or functions change on the server
      Then the canvas updates without losing the current level
      And the changed box is highlighted

  Rule: Navigation

    Scenario Outline: Opening a box
      When I <action> on the box "web/src"
      Then the canvas shows the inside of "shop/web/src"

      Examples:
        | action                    |
        | double-click              |
        | select it and press Enter |

    Scenario: Going up
      Given I am looking at "shop/web/src"
      When I press Esc
      Then the canvas shows the inside of "shop/web"

    Scenario: Breadcrumbs
      When I am looking at "shop/web/src"
      Then the breadcrumbs read "shop", "web", "src"

    Scenario: The current level disappears after a rebuild
      Given I am looking at "shop/web/src/gone/deeper"
      When a rebuild removes it
      Then the canvas shows "shop/web/src", the nearest level that still exists
      And a level outside every repository falls back to the system level

    Scenario: The connection drops and recovers
      When the event stream disconnects
      Then the viewer shows that it is offline
      And it reconnects by itself and fetches the latest graph

  Rule: The side panel

    Scenario: Nothing selected
      Then the side panel explains how to click, double-click, and press Esc

    Scenario: Selecting a box
      When I click the box "web/src"
      Then the side panel shows its kind, language, and path
      And its function count, worst CRAP, functions by CRAP band, mutant counts, and how many functions are mutation-tested
      And imports coming in from and going out of this level
      And the routes it serves and calls, and the external packages it uses
      And a table of its functions, worst CRAP first, with CC, coverage, CRAP, and killed/survived/uncovered

    Scenario: Selecting a function shows its source
      Given I selected a box
      When I click a function in its table
      Then the side panel shows the function's lines from "GET /api/source", with 3 lines of context around them

    Scenario: Selecting an arrow
      When I click the import arrow from "web/src" to "api"
      Then the side panel shows the number of imports and each module pair behind it
      And an HTTP arrow also lists the routes it calls
