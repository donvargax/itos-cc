# Upgrading

Read the current branch's configuration and tool comments before moving a pin. Move itos's
repository pin with `itos pin <version>` and the workflow action with it, to that release's
commit with a `# v<version>` comment and no `version` input, so the action installs the launcher
of the release its ref names. Then apply each step of the release's Upgrading section that the
branch's configuration meets, and run `itos init --agent-rules`. Preserve verified checksums
when upgrading the tools fetched by `tools/bin/pinned`; never update only a version beside
stale hashes.

Go dependencies are locked in go.mod/go.sum. Use the project's gates to check module
verification and tidy-diff, and review code and vulnerability results before an update lands.
The Go toolchain is pinned too: its standard library is part of the resulting code, so security
patches matter even when module versions stay unchanged.

Tools by itos's owner can be pinned on release. Other updates normally wait seven days, with
security fixes following the existing security-update policy. Renovate reads
`.github/renovate.json5` from `main`, so its requests follow that file's managers and are judged
by the project's own CI. The file alone does not make Renovate run: the automation stays
unverified until the Renovate app is installed and a request has landed.

Mutation proof is off, waiting on T-46 now that the refactor's final comparison (T-132) is
done (`docs/go.md`, Quality checks), so no push judges mutation records yet. Once it is on, a pin move that changes code or
relevant test imports needs fresh records, made from the push's range start, the nearest
ancestor with a green `ci.yml` run, which `itos ci run` judges the proof from.

git-cliff publishes a `.sha512` file per archive rather than a checksums file: move its pin by
checking each archive against that file first, then pinning the archive's SHA-256.

The release job in `cd.yml` runs only on a push to `main`, after its commits, test and languages
jobs pass on every system (decision 48; `ci.yml`, the Linux push gate, never releases), and cuts
a release when the commits since the last tag call for one: the five archives, each
built on its own platform's runner, and `checksums.txt`, the archives attested, published with
the notes `tools/bin/release-notes` writes.
