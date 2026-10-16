// Package mutants owns pure mutation values and rules. Its model-bearing
// Workers, Toolchain, Fingerprints, Reach, History, MutationSnapshots,
// SourceEdits and TestLister contracts live beside those values: orchestration can use the port
// without a domain-to-child-port-to-domain cycle. The worktrees adapter and
// porttest fake import this package directly (decisions 29 and 47). The domain
// Worker requests name semantic run purposes, never physical scratch prefixes.
// Toolchain supplies observed runner facts to pure Command policy; Fingerprints
// supplies actual current provenance, and Reach supplies import closure. Their
// concrete adapters are testtools, testinputs and importgraph. These contracts
// reuse existing Command and runtime values; they add no wire or copy models.
// History supplies immutable committed bytes, caller-relative changes and
// repository-wide rename observations through githistory and our independent fake.
// Run owns scheduling, reuse, judgments and publication over these bindings.
// Progress reports model facts; the caller presents them. Snapshot codecs live
// in metricsdir; source annotation I/O lives in workdir and listing in testtools.
// Check, CheckCoverage, List and Orphans own read-only orchestration through
// MutationSnapshotReader, MutationCatalog and SourcePresence. Catalog metadata
// remains permissive at the adapter; full snapshot decoding has one codec.
// CheckParsed is shared with the retained Sample/exception boundaries, keeping
// stale-but-judged outcomes distinct from Check's presentation filter. List
// returns an ordered prefix on a later failure; Check presentation admits all
// results first. RenameTarget requires successful no-follow evidence, while
// Present conservatively refuses to call ordinary Lstat errors absence.
// The domain imports no implementation and contains no process/filesystem I/O.
package mutants
