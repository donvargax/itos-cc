// Package mutants owns pure mutation values and rules. Its model-bearing
// Workers contract lives beside those values: orchestration can use the port
// without a domain-to-child-port-to-domain cycle. The worktrees adapter and
// porttest fake import this package directly (decisions 29 and 47). The domain
// Worker requests name semantic run purposes, never physical scratch prefixes.
// The domain imports neither implementation, and contains no process or filesystem I/O.
package mutants
