# Decisions

The decisions that still stand, one record each. A record superseded keeps its
file and leaves this list, which itos decision record writes.

<!-- itos:decisions:begin -->

- [ADR-0001: Mutation testing is the command group mutation](0001-mutation-testing-is-the-command-group-mutation.md)
- [ADR-0002: mutation check gives a full run's verdict from the cache](0002-mutation-check-gives-a-full-run-s-verdict-from-the-cache.md)
- [ADR-0004: Project settings live in itos-cc.yaml at the project root](0004-project-settings-live-in-itos-cc-yaml-at-the-project-root.md)
- [ADR-0005: An excepted survivor is reused as a kill is, and its entry goes stale with its function or its tests](0005-an-excepted-survivor-is-reused-as-a-kill-is-and-its-entry-goes-stale-with-its-function-or-its-tests.md)
- [ADR-0006: A site a snapshot never recorded makes its function stale](0006-a-site-a-snapshot-never-recorded-makes-its-function-stale.md)
- [ADR-0007: Each recorded outcome keeps the scope of the tests that decided it](0007-each-recorded-outcome-keeps-the-scope-of-the-tests-that-decided-it.md)
- [ADR-0008: Snapshots live at the project root, and every reader finds them where mutation check does](0008-snapshots-live-at-the-project-root-and-every-reader-finds-them-where-mutation-check-does.md)
- [ADR-0009: A project's tests are listed by its own command and run by its own template, sharing only IDs and paths](0009-a-project-s-tests-are-listed-by-its-own-command-and-run-by-its-own-template-sharing-only-ids-and-paths.md)
- [ADR-0010: Coverage per test comes from one suite run the harness splits by test, one run per test the fallback](0010-coverage-per-test-comes-from-one-suite-run-the-harness-splits-by-test-one-run-per-test-the-fallback.md)
- [ADR-0011: A kill by listed tests holds while its covering tests and the files defining them are unchanged](0011-a-kill-by-listed-tests-holds-while-its-covering-tests-and-the-files-defining-them-are-unchanged.md)
- [ADR-0012: A mutant runs its file's own tests first, and the listed tests reaching its line only if it survives them](0012-a-mutant-runs-its-file-s-own-tests-first-and-the-listed-tests-reaching-its-line-only-if-it-survives-them.md)
- [ADR-0013: The hashes of the files mutation.tests.support matches join every listed kill's freshness](0013-the-hashes-of-the-files-mutation-tests-support-matches-join-every-listed-kill-s-freshness.md)
- [ADR-0014: An exception whose file is renamed follows it and fails once as moved](0014-an-exception-whose-file-is-renamed-follows-it-and-fails-once-as-moved.md)
- [ADR-0015: A mutant's timeout is its baseline's time times the factor plus a fixed 5 seconds](0015-a-mutant-s-timeout-is-its-baseline-s-time-times-the-factor-plus-a-fixed-5-seconds.md)
- [ADR-0016: Go whole-suite mutation outcomes depend on module tests and configured support files](0016-go-whole-suite-mutation-outcomes-depend-on-module-tests-and-configured-support-files.md)
- [ADR-0017: Strict Go mutation checks require executable coverage independent of mutation sites](0017-strict-go-mutation-checks-require-executable-coverage-independent-of-mutation-sites.md)
- [ADR-0018: Strict Go statement coverage admits measured built-in reports and fingerprinted cache evidence](0018-strict-go-statement-coverage-admits-measured-built-in-reports-and-fingerprinted-cache-evidence.md)
- [ADR-0019: Strict Go coverage refuses workspace and out-of-inventory replacement scopes](0019-strict-go-coverage-refuses-workspace-and-out-of-inventory-replacement-scopes.md)
- [ADR-0020: Mutation fail-fast cancels unfinished work at the first observed final failure](0020-mutation-fail-fast-cancels-unfinished-work-at-the-first-observed-final-failure.md)
- [ADR-0021: Fail-fast owns ordinary test process trees with explicit Unix containment limits](0021-fail-fast-owns-ordinary-test-process-trees-with-explicit-unix-containment-limits.md)
- [ADR-0022: Fail-fast preserves valid partial mutation evidence and reports incomplete work explicitly](0022-fail-fast-preserves-valid-partial-mutation-evidence-and-reports-incomplete-work-explicitly.md)

<!-- itos:decisions:end -->
