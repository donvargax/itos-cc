# Engineering practices

These rules apply to the whole project, regardless of language or public interface.
Language and interface guides add their own tooling and interface contracts. The rules
describe the intended design; the project's configuration and CI say which rules a command
can enforce.

## Design

Organize work as vertical feature slices. Keep an entry point thin: it reads the caller's
input, carries domain values into the application, and presents the result. Application
code coordinates the work; the domain owns its behavior and knows nothing about the user
interface, output formatting or process exit codes.

- Reach files, networks, processes and other outside systems through domain-owned ports.
  Concrete adapters implement those ports; the domain does not import its adapters.
- An adapter translates an outside model only where that model would otherwise leak into
  the domain. Do not add a translation layer where the models already agree.
- Carry domain objects rather than copying their fields into an options struct or DTO.
  Presentation data belongs at the interface boundary; it is not a second domain model.
- A slice does not import another slice's entry point. Shared behavior belongs in a domain
  package named for what it models, not a miscellaneous helpers package.
- Give platform-dependent logic the platform as a value. Detect the host at the boundary,
  so tests can exercise other platforms without changing the machine they run on.

Use the smallest structure that holds these boundaries. Do not add wrappers, ports or
mapping code merely to make a small example resemble a larger application.

## Tests

Specify public behavior before implementing it. Acceptance tests call public entry points,
not internal functions. Commit their failing steps first, confirm each fails at the step
that checks the intended behavior, then implement it. A failing setup or syntax error is
not evidence that the behavior is missing.

Unit tests exercise the domain with our own fakes and stubs of its ports. Assert outcomes
and returned failures, not a script of expected calls. Do not use mocking libraries.
Properties belong with the domain behavior they describe; state the assumptions that
make a property valid rather than making the implementation fit an invalid property.

Behavior reachable through acceptance tests is held there. A unit test outside the domain
needs an explanation of what the public entry point cannot exercise. Keep useful tests
while resolving an unclear boundary; do not delete coverage merely to satisfy a layout.

Test platform-sensitive behavior on the supported platforms. A success on one machine
does not establish file, path, permission or line-ending behavior on another.

## Dependencies

Look for an existing library before building code that one likely already supplies. Weigh
its maintenance, complexity, likely changes and value. The person makes the choice, and
the specification records it before implementation. If the specification leaves that
choice open, stop and propose candidates rather than deciding inside the code.

Commit dependency lockfiles and verify locked installs. Pin external tools and workflow
actions, verify downloads using the declared checksum or provenance, and document how to
upgrade them. Preserve license notices when harvesting proven infrastructure.

## What is enforced

Keep the agent rules short. Put lasting design reasons beside the scenario, task, package
or decision a reader will look at. Generate the machine-checkable rules from the actual
project configuration; do not maintain a second list that silently disagrees with it.

Use formatting, lint, dependency checks, tests, coverage and code proof where the stack
supplies them. A valid check's failure is fixed in the work, not by weakening the check.
If a check is wrong, stop and explain why. An uncovered behavior needs a test; an exception
needs a reason a person can review. Never copy another project's proof cache.

No credentials belong in committed files, fixtures or logs. State which scans actually
run and what they establish. A configured workflow is not proof that its external service
is active, and a sample of mutation results is not a complete changed-code proof.
