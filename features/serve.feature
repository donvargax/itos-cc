Feature: Serving the architecture graph
  "itos-cc serve" watches one or more repositories and publishes their
  architecture over HTTP for the viewer.

  Scenario: Serving the working directory
    When I run "itos-cc serve"
    Then stderr says it is serving [.] on http://127.0.0.1:7070
    And it listens on localhost only

  Scenario: Serving several repositories on another port
    When I run "itos-cc serve --port 8080 . ../other-repo"
    Then both repositories are in one graph on http://127.0.0.1:8080

  Scenario: The graph as JSON
    Given the server is running
    When I request "GET /api/graph"
    Then the response is JSON with the version, nodes, and edges
    And it is not cached

  Scenario: Change notifications
    Given the server is running at graph version 3
    When I connect to "GET /api/events"
    Then I receive a server-sent event "version" with data 3
    And when a source file is saved I receive the next version
    And a keepalive comment arrives every 15 seconds while nothing changes

  Scenario: A slow client skips versions rather than blocking
    Given a client that has not read its last event
    When the graph changes twice
    Then the server keeps rebuilding and the client gets a later version

  Scenario: Source of a file in the graph
    Given the repository "shop" contains "web/src/app.ts" in the graph
    When I request "GET /api/source?repo=shop&file=web/src/app.ts"
    Then the response is the file's text as text/plain

  Scenario Outline: Files outside the graph are refused
    When I request "GET /api/source?repo=<repo>&file=<file>"
    Then the response is 404

    Examples:
      | repo    | file              |
      | shop    | ../../etc/passwd  |
      | shop    | .env              |
      | unknown | web/src/app.ts    |

  Scenario: Watching for changes
    When I run "itos-cc serve --interval 1s"
    Then sources and .metrics snapshots are checked every second
    And the default interval is 500ms

  Scenario: A file caught mid-save keeps the previous graph
    Given a source file does not parse while it is being saved
    When the server rebuilds
    Then the error is logged
    And the previous graph is still served

  Scenario: Without a built viewer, / says how to get one
    When I run "itos-cc serve" and request "GET /"
    Then the response names the API endpoints and suggests starting the viewer or passing --ui viewer/dist

  Scenario: Serving a built viewer from the same port
    Given the viewer was built into viewer/dist
    When I run "itos-cc serve --ui viewer/dist ."
    Then "GET /" serves the viewer
    And a client-side route such as "GET /shop/web" falls back to index.html

  Scenario: Stopping
    Given the server is running
    When I press Ctrl-C
    Then the server shuts down and exits with 0
