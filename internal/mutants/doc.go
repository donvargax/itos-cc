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
// The domain imports no implementation and contains no process/filesystem I/O.
package mutants
