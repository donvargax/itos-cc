Feature: Architecture graph
  "What depends on what, and where is it risky?" The graph is a tree of
  repositories, directories, modules, and functions, with import and HTTP
  edges between modules and metrics from the .metrics snapshots on every node.

  Background:
    Given the repository "shop" with:
      | part | language   | layout                                                      |
      | api  | Go         | a package with an internal/store subpackage                 |
      | app  | Kotlin     | com.acme.billing (Invoice, Receipt) and com.acme.util       |
      | py   | Python     | src/shop with cli, billing/invoice, and billing/tax         |
      | web  | TypeScript | src/app, src/cart, and src/format, with tsconfig path aliases |

  Rule: Nodes

    @ID-GRAPH-01
    Scenario: The tree
      When the graph is built
      Then "shop" is a repo node
      And "shop/web/src" is a dir node whose parent is "shop/web"

    @ID-GRAPH-02
    Scenario: A module is a file, except in Go
      When the graph is built
      Then each TypeScript, Python, and Kotlin file is a module
      And each Go package is a module

    @ID-GRAPH-03
    Scenario: A Go package with subpackages is one node
      When the graph is built
      Then "shop/api" is a Go module whose parent is "shop" and which contains "shop/api/internal/store"

    @ID-GRAPH-04
    Scenario: A Python package is its __init__ module
      When the graph is built
      Then "shop/py/src/shop/billing" is a Python module

    @ID-GRAPH-05
    Scenario: Modules list their functions
      When the graph is built
      Then "shop/py/src/shop/billing/invoice" has one function, "invoice"

    @ID-GRAPH-06
    Scenario: Two repositories with the same directory name
      When I serve "a/shop" and "b/shop" together
      Then they are named "shop" and "shop-2"

  Rule: Import edges

    @ID-GRAPH-07
    Scenario: Each language's imports resolve to modules
      When the graph is built
      Then the import edges are:
        | from                                               | to                                              |
        | shop/api                                           | shop/api/internal/store                         |
        | shop/app/src/main/kotlin/com/acme/billing/Invoice  | shop/app/src/main/kotlin/com/acme/util/Format   |
        | shop/app/src/main/kotlin/com/acme/billing/Invoice  | shop/app/src/main/kotlin/com/acme/util/Money    |
        | shop/app/src/main/kotlin/com/acme/billing/Receipt  | shop/app/src/main/kotlin/com/acme/billing/Invoice |
        | shop/app/src/main/kotlin/com/acme/util/Format      | shop/app/src/main/kotlin/com/acme/util/Money    |
        | shop/py/src/shop/billing/invoice                   | shop/py/src/shop/billing/tax                    |
        | shop/py/src/shop/cli                               | shop/py/src/shop/billing/invoice                |
        | shop/py/src/shop/cli                               | shop/py/src/shop/billing/tax                    |
        | shop/web/src/app                                   | shop/web/src/cart/total                         |
        | shop/web/src/cart/total                            | shop/web/src/format/index                       |
        | shop/web/src/cart/view                             | shop/web/src/cart/total                         |
        | shop/web/src/cart/view                             | shop/web/src/format/index                       |

    @ID-GRAPH-08
    Scenario: Kotlin classes in the same package depend without an import
      Given Receipt uses Invoice from the same package without importing it
      When the graph is built
      Then Receipt has an import edge to Invoice

    @ID-GRAPH-09
    Scenario Outline: Import forms each language recognizes
      Given the <language> source:
        """
        <source>
        """
      When its imports are read
      Then they include "<import>"

      Examples:
        | language   | source                                           | import             |
        | TypeScript | import { a } from './a'                          | ./a                |
        | TypeScript | export { c } from '../c'                         | ../c               |
        | TypeScript | const d = require('./d')                         | ./d                |
        | TypeScript | await import('./e')                              | ./e                |
        | TypeScript | import type { F } from './f'                     | ./f                |
        | Python     | from ..core.models import User                   | ..core.models      |
        | Python     | importlib.import_module("shop.plugins")          | shop.plugins       |
        | Python     | __import__('shop.legacy')                        | shop.legacy        |
        | Go         | import m "example.com/demo/board"                | example.com/demo/board |
        | Kotlin     | import com.acme.util.*                           | com.acme.util.*    |
        | Kotlin     | import kotlin.math.max as mx                     | kotlin.math.max    |

    @ID-GRAPH-10
    Scenario: A Python import inside a function counts
      Given a Python function that runs "import json"
      When its imports are read
      Then "json" is one of them

    @ID-GRAPH-11
    Scenario: A dynamic Python import of a variable is not guessed
      Given a Python function that runs "importlib.import_module(name)"
      When its imports are read
      Then that call adds no import

    @ID-GRAPH-12
    Scenario: TypeScript path aliases
      Given a tsconfig.json with comments and trailing commas:
        """
        {
          // comment with "quotes" and a // inside
          "compilerOptions": { "baseUrl": "src", "paths": { "@app/*": ["app/*", "legacy/*"], "@app/core": ["core/index"], }, },
        }
        """
      Then imports resolve as:
        | import       | candidates                                        |
        | @app/button  | src/app/button, src/legacy/button, src/@app/button |
        | @app/core    | src/core/index, src/@app/core                      |
        | utils/format | src/utils/format                                   |

    @ID-GRAPH-13
    Scenario: tsconfig "extends" and Vite-style "references" are followed
      Given tsconfig.json references tsconfig.app.json, which extends tsconfig.base.json with the paths
      When the graph is built
      Then "shop/web/src/cart/view" imports through "@/format", "~format", and "@/cart/total" resolve to project files

    @ID-GRAPH-14
    Scenario: External packages are listed, not drawn
      When the graph is built
      Then the modules list these external packages:
        | module                                            | external           |
        | shop/web/src/app                                  | @scope/zod, react  |
        | shop/py/src/shop/billing/invoice                  | os                 |
        | shop/api                                          | fmt, net/http      |
        | shop/app/src/main/kotlin/com/acme/billing/Invoice | kotlin.math        |
      And no edge points at an external package

  Rule: HTTP edges

    @ID-GRAPH-15
    Scenario: A request links to the module serving its route
      Given shop/api serves "GET /api/invoices/{id}"
      And shop/web/src/cart/invoices fetches "/api/invoices/${id}"
      When the graph is built
      Then there is an http edge from shop/web/src/cart/invoices to shop/api via "GET /api/invoices/*"
      And shop/api lists "GET /api/invoices/*" among the routes it serves

    @ID-GRAPH-16
    Scenario: Requests link across repositories
      Given a frontend repository calls a route that a service in another repository serves
      When both repositories are served together
      Then an http edge links the frontend module to the service module

    @ID-GRAPH-17
    Scenario Outline: Routes each framework serves
      Given the <language> source declares <declaration>
      Then the module serves "<route>"

      Examples:
        | language   | declaration                                              | route                |
        | Go         | mux.HandleFunc("GET /api/graph", graph)                  | GET /api/graph       |
        | Go         | r.Get("/users/{id}", user) on a chi router               | GET /users/*         |
        | Go         | e.POST("/users/:id/tags", tag) on echo                   | POST /users/*/tags   |
        | TypeScript | app.get("/users/:id", ...)                               | GET /users/*         |
        | TypeScript | router.post("/users", createUser)                        | POST /users          |
        | Python     | @app.route("/health")                                    | /health              |
        | Python     | @router.get("/users/{user_id}")                          | GET /users/*         |
        | Python     | @bp.post("/users/<int:id>/tags")                         | POST /users/*/tags   |
        | Kotlin     | @RequestMapping("/api/users") with @GetMapping("/{id}")  | GET /api/users/*     |
        | Kotlin     | @RequestMapping("/api/users") with @PostMapping          | POST /api/users      |
        | Kotlin     | Ktor route("/v1") { get("/health") { } }                 | GET /v1/health       |

    @ID-GRAPH-18
    Scenario Outline: Requests each client makes
      Given the <language> source calls <call>
      Then the module calls "<route>"

      Examples:
        | language   | call                                                           | route             |
        | Go         | http.Get("http://localhost:7070/api/graph?x=1")                | GET /api/graph    |
        | Go         | http.NewRequest("DELETE", "https://svc/users/"+id, nil)        | DELETE /users/*   |
        | TypeScript | axios.get("/api/users", { params: {} })                        | GET /api/users    |
        | TypeScript | fetch(`${API}/api/source?repo=${repo}`)                        | /api/source       |
        | TypeScript | new EventSource("/api/events")                                 | GET /api/events   |
        | TypeScript | api.delete(`/users/${id}`)                                     | DELETE /users/*   |
        | Python     | requests.get(f"http://billing/invoices/{user_id}")             | GET /invoices/*   |
        | Python     | httpx.post("/audit", json={})                                  | POST /audit       |
        | Kotlin     | rest.getForObject("http://billing/invoices/$id", ...)          | GET /invoices/*   |

    @ID-GRAPH-19
    Scenario: Calls that are not HTTP are ignored
      Given the source calls cache.get("key") and cache.Get("not-a-route")
      Then neither is a route or a request

    @ID-GRAPH-20
    Scenario: Requests too vague to place are skipped
      Given the TypeScript source calls fetch(`${base}/${path}`)
      Then it is not listed as a request

    @ID-GRAPH-21
    Scenario Outline: Matching a request to a route
      Given a route "<serves>"
      And a request "<calls>"
      Then they <match>

      Examples:
        | serves              | calls             | match        |
        | GET /users/*        | /users/42         | match        |
        | GET /users/*        | POST /users/42    | do not match |
        | /users              | DELETE /users     | match        |
        | GET /users/*/tags   | GET /users/*      | do not match |
        | GET /api/source     | /api/*            | match        |

  Rule: Metrics on every node

    @ID-GRAPH-22
    Scenario: Snapshots are joined onto functions
      Given .metrics holds crap.json, dry.json, and mutation snapshots
      When the graph is built
      Then each function carries its complexity, coverage, CRAP, mutant counts, and duplicate count

    @ID-GRAPH-23
    Scenario: CRAP moves as you edit
      Given crap.json recorded 80% coverage for a function
      When I add a branch to that function and save
      Then its CRAP is recomputed from the new complexity and the recorded coverage

    @ID-GRAPH-24
    Scenario: Mutation results of edited functions are stale
      Given a function's mutation snapshot hash no longer matches its source
      When the graph is built
      Then the function is marked stale
      And its module counts one stale function

    @ID-GRAPH-25
    Scenario: Results match their own file
      Given a Go package with an init function in both a.go and b.go
      And crap.json and mutation snapshots that name a.go's init
      When the graph is built
      Then only a.go's init carries a.go's results

    @ID-GRAPH-26
    Scenario: Snapshots written from another directory still match
      Given a snapshot names "repo/lang/kotlin.go"
      Then it matches the function in "lang/kotlin.go"
      But not the function of the same name in "lang/golang.go"

    @ID-GRAPH-27
    Scenario Outline: CRAP bands
      Given a function with CRAP <crap>
      Then it is in the <band> band

      Examples:
        | crap | band    |
        | 5    | low     |
        | 17   | medium  |
        | 30   | high    |
        | none | unknown |

    @ID-GRAPH-28
    Scenario Outline: Grades run from 1 (worst) to 10 (best)
      Then <measure> grades <grade>

      Examples:
        | measure                                              | grade |
        | a module whose worst CRAP is 3                       | 10    |
        | a module whose worst CRAP is 30                      | 1     |
        | a module whose worst CRAP is 17.5                    | 5.5   |
        | 9 killed and 1 survived mutant                       | 9.1   |
        | 0 killed and 4 survived mutants                      | 1     |
        | a directory with 9 low-risk and 1 high-risk function | 8.2   |
        | a directory with 1 low-risk and 1 high-risk function | 1     |
        | a directory with 7 low, 3 medium, and 40 unmeasured  | 8.4   |

    @ID-GRAPH-29
    Scenario: A module is as risky as its worst function
      Given a module with one function at CRAP 120
      Then its CRAP grade is 1

    @ID-GRAPH-30
    Scenario: One bad file does not paint a whole system red
      Given a repository with one module at CRAP 120 and nine at CRAP 2
      And every function has been mutation-tested, with 37 mutants killed and 1 survived overall
      Then the repository's CRAP grade is 8.2
      And its mutation grade is the overall kill rate
      And 10 of its 10 functions are mutation-tested

    @ID-GRAPH-31
    Scenario: The overall grade combines CRAP and mutation
      Given a node with both a CRAP grade and a mutation grade
      Then its grade is their average
      But a node with only one of them is graded by that one
      And a node with neither is not graded

  Rule: Rebuilding

    @ID-GRAPH-32
    Scenario: Unchanged sources do not rebuild
      Given the graph has been built
      When it is built again with no file changed
      Then it reports no change and keeps the same version

    @ID-GRAPH-33
    Scenario: Saving a source file or a snapshot rebuilds
      When a source file or a file under .metrics changes
      Then the next build has a new version
      And only changed files are parsed again
