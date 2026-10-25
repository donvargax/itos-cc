# slice-56, graph-snapshot's second item (q-86, q-95, ADR-0060; the person
# 2026-10-24): itos-cc graph writes the architecture graph, what serve's
# /api/graph returns today, as .metrics/graph.json through the snapshot
# store, so a viewer or web app reads files alone. Decided:
# 1. The command is graph, a vertical slice in internal/slices/graph
#    (ADR-0051). T-208's move of graph/ there lands in the same push, its
#    refactor commits first (assemble's gate refuses a slice folder that
#    declares no command, so the move cannot land alone; the coordinator's
#    call, 2026-10-24). Mutation freshness reaches graph through a port graph
#    owns, bound in cmd to a function the mutation slice exports, so
#    mutation check's rule stays mutation's.
# 2. The whole project, with no selection flags; exclude leaves nothing out
#    of the graph (README says so for serve today).
# 3. One project per file. The repository node is named by the git remote's
#    repository name (origin's URL, its last path segment without .git), the
#    folder's name when there is no remote, so a local clone and CI agree.
# 4. "version" is the snapshot format version, 1, not a rebuild counter. The
#    file is sorted and holds no time, so a rerun on unchanged input writes
#    it byte for byte as before. Its shape is otherwise /api/graph's, field
#    for field, so the viewer needs no type change.
# 5. --json prints one object with "schema": 1, "ok", and the counts
#    "repositories", "modules", "functions" and "edges", not the graph.
#    Exit 0 whatever the graph holds; config, environment and an unwritable
#    snapshot keep their usual exits.
# serve and the viewer are unchanged by this item (the viewer reading the
# file is parked with serve's rework, serve-static-on-kong). The nightly
# committing the file is graph-nightly. Before this item every scenario
# fails at its When: itos-cc has no graph command.
Feature: The architecture graph as a snapshot
  "What depends on what, and where is it risky?", written once to a file that
  any viewer can read without asking itos-cc.

  @slice-56 @ID-GRAPH-41 @wip
  Scenario: The graph is written as a snapshot
    Given a Go module with the packages app and app/store, app importing app/store
    When I run "itos-cc graph"
    Then the exit code is 0
    And .metrics/graph.json holds "version": 1, nodes and edges
    And in .metrics/graph.json the repository is a repo node, app and app/store are module nodes, and app imports app/store

  @slice-56 @ID-GRAPH-42 @wip
  Scenario: The repository node is named by the git remote's repository name
    Given a Go module with the packages app and app/store, app importing app/store
    And its git remote origin is "https://example.com/acme/shop.git" while its folder is named "checkout"
    When I run "itos-cc graph"
    Then in .metrics/graph.json the repo node is named "shop"

  @slice-56 @ID-GRAPH-43 @wip
  Scenario: A function's mutation state is mutation check's
    Given a Go module with the packages app and app/store, app importing app/store
    And .metrics/mutate holds a fresh snapshot for one function of app and a stale one for another
    When I run "itos-cc graph"
    Then each function in .metrics/graph.json is marked stale exactly when "itos-cc mutation check" calls it stale

  @slice-56 @ID-GRAPH-44 @wip
  Scenario: The graph sees every file, whatever exclude says
    Given a Go module with the packages app and app/store, app importing app/store
    And itos-cc.yaml holds 'exclude: ["app/store/**"]'
    When I run "itos-cc graph"
    Then in .metrics/graph.json app/store is a module node

  @slice-56 @ID-GRAPH-45 @wip
  Scenario: An unchanged project writes the same file
    Given a Go module with the packages app and app/store, app importing app/store
    And "itos-cc graph" has written .metrics/graph.json
    When I run "itos-cc graph" again
    Then .metrics/graph.json is byte for byte as it was

  @slice-56 @ID-GRAPH-46 @wip
  Scenario: The summary for a script
    Given a Go module with the packages app and app/store, app importing app/store
    When I run "itos-cc graph --json"
    Then stdout is one object with "schema": 1, "ok", "repositories", "modules", "functions", and "edges"
    And the exit code is 0
