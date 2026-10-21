# Proposal: feature-owned models and consumer-owned interfaces

Status: **evaluated in T-86 and settled by ADR-0051**, which supersedes ADR-0029's
layout. Its paths and package names are illustrative: the slices live under
`internal/slices/<name>/`, as `docs/go.md` describes them.

Approach: **localized, consumer-owned interfaces (Option 2)**, satisfied
implicitly by feature-owned models.

## Ownership

Each feature directory owns its commands, application logic, domain models,
ports, adapters, fakes and tests. Commands sharing semantics belong to one
feature: mutation run, check, list, sample and except share mutation ownership.
There is no global directory collecting feature business models or their ports.

A consuming feature declares the small interface describing the behavior it
needs. Another feature's model can satisfy that interface implicitly. The
consumer's code depends on its local interface and does not import the provider.

Substantial reusable engines, such as language parsing and coverage, retain
explicit, stable APIs under their own ownership. Inventory these boundaries
during T-86; avoid duplicating an engine or recreating a miscellaneous shared
domain package merely to make the directory layout uniform.

## Illustrative Go example

These project/task names illustrate the proposal; they are not new commands.

The project feature owns its model:

```go
// internal/features/project/domain.go
package project

type Project struct {
    Name   string
    Status string
}

func (p Project) IsActive() bool { return p.Status == "ACTIVE" }
func (p Project) ProjectName() string { return p.Name }
```

The task feature owns the interface it consumes:

```go
// internal/features/task/domain.go
package task

type ProjectState interface {
    IsActive() bool
}

func CanAssign(p ProjectState) bool {
    return p.IsActive()
}
```

Explicit composition outside both features brings the values together:

```go
// internal/assembly/example.go
package assembly

import (
    "example.com/app/internal/features/project"
    "example.com/app/internal/features/task"
)

var _ task.ProjectState = project.Project{}

func CanAssign(p project.Project) bool {
    return task.CanAssign(p)
}
```

Passing the model requires no mapper because its method set already meets the
consumer's contract. A consumer needing the name can declare
`ProjectName() string` in its own narrow interface. Add observations only when an
actual use case needs them.

The original example's task command imported the project feature. For strict
isolation of the entire feature directory, that import belongs in assembly or a
narrow integration bridge outside both features. Ordinary command discovery
should still use feature-local declarations and deterministic generated assembly
through the existing Kong fork, as T-86 already proposes. Avoid turning assembly
into a shared handwritten command list that every feature must edit.

## Contract limits

- Go matches interface method sets, including exact parameter and return types.
  Fields alone do not satisfy interface methods; pointer receivers also affect
  which values satisfy a contract.
- A method returning a provider-owned named type does not automatically satisfy a
  method returning a different consumer-owned type or interface. Choose simple
  observations where appropriate; use a small explicit integration adapter when
  actual contracts differ. This proposal does not promise that every interaction
  can avoid translation.
- Keep model observations pure. Filesystem, Git, storage and process operations
  use explicit consumer-owned ports with caller context and typed failures.
  Passing a model must not conceal lazy external reads.
- Share a value directly only when its semantics, ownership and lifetime fit the
  consumer. Mutable models and returned slices/maps need explicit aliasing and
  concurrency rules; interface satisfaction alone does not establish safety.
- Provider replacement can leave consumer domain code unchanged when the
  contract holds. Removing a provider can still require composition changes.
  Avoid adding a large collection of getters solely to accommodate every caller.

## Evaluation and enforcement in T-86

Inventory real cross-feature needs before choosing interfaces or migration
steps. Split the follow-up into independently landable tasks. Evaluate localized
interfaces against concrete mutation, graph and coverage interactions, including
cases where exact named types require an integration bridge.

Enforce the intended import graph with accepted/refused architecture fixtures:
features do not import sibling features, while explicit assembly can import the
features it connects. Keep pure domain code independent of CLI and concrete I/O.
Use outcome tests and our own fakes to hold each consumer's contract, and shared
adapter contracts where applicable.

Verify that an ordinary new command or feature change stays in its feature
directory, apart from deterministic generated assembly and explicitly assigned
integration changes. Preserve command behavior, help, scenario IDs, storage
formats, notices and the configured quality policy. Revise or supersede affected
ADRs through the decision workflow when T-86 executes; implementation details and
the final directory layout remain to be settled there.

Related: [engineering practices](engineering.md),
[ADR-0029](decisions/0029-go-code-is-a-shared-domain-behind-ports-separate-adapters-and-thin-slices-named-for-their-commands.md)
and the T-86 task specification in [the ledger](../tasks/phase-1.yaml).
