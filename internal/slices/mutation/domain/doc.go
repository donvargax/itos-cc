// Package domain is the mutation slice's own domain (ADR-0051), kept apart
// from its CLI and application code: it owns pure mutation values and
// rules, and its importers name it mutants. Its model-bearing Workers,
// Toolchain, Fingerprints, Reach, History, MutationSnapshots, SourceEdits,
// TestLister and SettingsReader contracts live beside those values:
// orchestration can use the port without a domain-to-child-port-to-domain
// cycle. The slice's adapters and the porttest fakes import this package
// directly (decisions 29 and 47). The domain Worker requests name semantic
// run purposes, never physical scratch prefixes. Toolchain supplies observed
// runner facts to pure Command policy; Fingerprints supplies actual current
// provenance, and Reach supplies import closure. These contracts reuse
// existing Command and runtime values; they add no wire or copy models.
// History supplies immutable committed bytes, caller-relative changes and
// repository-wide rename observations through githistory and our
// independent fake. Run owns scheduling, reuse, judgments and publication
// over these bindings. Progress reports model facts; the caller presents
// them.
//
// Its concrete adapters are the slice's nine under
// internal/slices/mutation/adapters, each a subpackage of its name:
// testtools, the Toolchain and TestLister; testinputs, the Fingerprints and
// the coverage-input inventories; worktrees, the Workers; githistory, the
// History; importgraph, the Reach over internal/imports, the
// import-resolution engine; freshenv, the counted Fresh; snapshots, the one
// snapshot codec, MutationSnapshots, MutationSnapshotReader and
// MutationCatalog over the snapshot port's Store, and the CoverageCache of
// the listed tests' and each plan's own coverage beside them; exceptions,
// the Exceptions writer over config's reading codec; and sources,
// SourceEdits, SourcePresence and SourceStatus over the disk.
//
// Check, CheckCoverage, List and Orphans own read-only orchestration through
// MutationSnapshotReader, MutationCatalog and SourcePresence. Catalog metadata
// remains permissive at the adapter; full snapshot decoding has one codec.
// Sample, Survivor, Renew and Elsewhere own sampling and exception judgment.
// CheckParsed is shared by these domain operations, keeping
// stale-but-judged outcomes distinct from Check's presentation filter. List
// returns an ordered prefix on a later failure; Check presentation admits all
// results first. RenameTarget requires successful no-follow evidence, while
// Present conservatively refuses to call ordinary Lstat errors absence.
// Exceptions updates the current document, whose YAML the exceptions adapter
// rewrites and scenario memory bytes reuse. SourceStatus observes
// following-Stat kind, independently of SourcePresence, and each operation
// applies its own policy. Sample owns no publication capability and reuses
// the one execution scheduler. Config is the one semantic settings value,
// aliased by config.Config, holding the selection engine's settings, root
// and exclude. Its reader supplies current settings at each actual admission
// point; the existing config codec and disk or memory bytes remain outside
// this package. The domain imports no implementation and contains no
// process/filesystem I/O.
package domain
