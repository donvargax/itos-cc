# Security and trust boundaries

Do not put credentials in committed files, fixtures or logs. The project's owner and module
path identify a public project, not authentication data.

itos-cc reads the files its selection chooses, the paths its arguments name or the working
directory, through the selection domain's `Tree` port. It is not a filesystem sandbox; a caller
must decide which paths it permits. It writes snapshots under `.metrics/`, `itos-cc.yaml` when
`mutation except` records an entry, and, unless `--no-annotate`, a summary comment at the end
of each source `mutation run` judged. `crap` and the `mutation` commands run the project's own
tests and coverage tools, and the commands `--coverage-command` and `itos-cc.yaml`'s
`mutation.tests` name, so running itos-cc in a project runs that project's code: run it only
where you would run the project's tests. `serve` listens on localhost only.

Dependencies are locked with go.mod/go.sum and verified in CI. Tools keep their checked pins,
upstream identities, licenses and notices. Go's vulnerability scan examines used dependencies,
including test dependencies and the standard library. Mutation testing judges recorded
behavior, and no push runs mutation proof yet (`docs/go.md`); these controls do not isolate
untrusted programs.

gitleaks is pinned in `tools/bin/pinned`, but the project's own CI runs no standing credential
scan. itos's hooks and commit rules are workflow controls, not a security sandbox.
