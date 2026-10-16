// Package mutants owns pure mutation values and rules. Its model-bearing
// Workers, Toolchain, Fingerprints and Reach contracts live beside those values: orchestration can use the port
// without a domain-to-child-port-to-domain cycle. The worktrees adapter and
// porttest fake import this package directly (decisions 29 and 47). The domain
// Worker requests name semantic run purposes, never physical scratch prefixes.
// Toolchain supplies observed runner facts to pure Command policy; Fingerprints
// supplies actual current provenance, and Reach supplies import closure. Their
// concrete adapters are testtools, testinputs and importgraph. These contracts
// reuse existing Command and runtime values; they add no wire or copy models.
// The domain imports no implementation and contains no process/filesystem I/O.
package mutants
