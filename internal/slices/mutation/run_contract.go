package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract preserves the run command’s existing help contract.
func (Run) Contract() cli.Contract {
	return cli.Contract{
		About: `
Changes one operator, boolean, or 0/1 at a time inside each function and runs
the file's own tests: its Go package, the Vitest or Jest tests that import it,
in Python the test files that reach it (named test_*.py or *_test.py), run
with python -m pytest -x <files>, or python -m unittest <modules> without
pytest, in Kotlin the test classes the test files that reach it declare,
run with gradle test --fail-fast --tests <class> per module, or mvn test
-Dtest=<classes>, in Dart the test files under its package's test/ that
reach it (named *_test.dart), run with flutter test --no-pub <files>, or
the whole suite where nothing narrower exists. A Dart file's tests under
integration_test/ or test_driver/ never run: they need a device. A
mutant is killed when the tests fail, time out or go over a cap, and
survives when they pass. Mutants on lines those tests never execute are
uncovered and are not run, and every mutant of a Python, Kotlin or Dart
file no such test reaches is uncovered, with no test run for it. A Python
or Kotlin test that runs code only through a subprocess, the CLI or
reflection reaches nothing: run --all-tests or --test-command for such
code.

Each command's tests first run once without any mutant, its baseline, in
a worker's copy. A baseline that fails is mutation.baseline-failed: none
of its files' mutants is judged, and the report gives its output, whole up
to 40 lines, else its first 20 lines and its last 20 with a line saying
how many were left out between ("output" in --json). What the baseline
used bounds each mutant's run: it times out after the baseline's time
times --timeout-factor, plus 5s, and is killed once its tests write more
than the baseline's output, stdout and stderr together, times
--output-factor, plus 16 MiB, or hold more than the baseline's peak
memory, its process group's resident memory summed, times
--memory-factor, plus 256 MiB. Memory is measured on Linux alone: on
macOS and Windows no memory cap holds, which the run says once on stderr.
A mutant killed over a cap is "over-limit", with "limit" "output" or
"memory", and counts as killed, as a timeout does.

--all-tests runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests can kill mutants too. It is slow: run it
nightly. In Go, a test that runs the built binary shows up in coverage when
it builds the binary with go build -cover while GOCOVERDIR is set: itos-cc
sets it, and merges what the binary wrote with go test's own coverage. In
Python, the Python processes a test starts show up too: coverage.py starts
in them through the .pth file it installs (7.13 and later), from an
rcfile of the run's own that COVERAGE_PROCESS_START names, which holds the
project's [run] settings under itos-cc's, and what they wrote is combined
beside the in-process coverage. A coverage.py that does not start in them
is coverage.tool-missing with --fail-uncovered, where Python has mutants to
judge, and otherwise a log line, the run going on without them. A project
whose coverage.py configuration sets [run] parallel = true, or patch =
subprocess, has its parallel data files combined, with its [paths], before
the report; under its own patch = subprocess the processes a test starts
write beside the in-process data, and their lines count as in-process. In
TypeScript under Vitest or Jest, the Node processes a test starts show up
too: NODE_V8_COVERAGE names a directory of the run's own, and the
project's c8 reports what they wrote beside the runner's coverage, leaving
out what the runner's own processes wrote; a project without c8 is
coverage.tool-missing or a log line likewise. A Kotlin harness opts in,
as a Go one builds with -cover: while the coverage command runs,
ITOS_CC_JACOCO_AGENT names the JaCoCo runtime agent jar
(org.jacoco.agent-<version>-runtime.jar) in the Gradle or Maven cache, at
the version the build names where it names one, and ITOS_CC_JACOCO_DESTDIR
a directory of the run's own, and the harness starts the JVMs its tests
run with
  -javaagent:$ITOS_CC_JACOCO_AGENT=destfile=$ITOS_CC_JACOCO_DESTDIR/<name>.exec
a <name> of each JVM's own, or append=true when JVMs share one. Afterwards
jacococli (org.jacoco.cli-<version>-nodeps.jar, from the same caches, run
with JAVA_HOME's java) merges what they wrote and reports it against the
module's class files (build/classes/kotlin/main and java/main, or
target/classes) beside the JaCoCo or Kover report; while the agent is
named, Gradle's test task runs with --rerun. Neither jar is downloaded:
without the agent ITOS_CC_JACOCO_AGENT is unset, which a harness can tell,
and .exec files with no jacococli to report them are coverage.tool-missing
with --fail-uncovered, where Kotlin has mutants to judge, and otherwise a
log line. Kover's own agent is not named: a Kover project's harness uses
JaCoCo's too. In Dart, --all-tests runs flutter test --no-pub with no path,
the test files under the package's test/ alone, never integration_test/ or
test_driver/, and the processes a test starts show up in no coverage.
Every test and coverage command itos-cc composes runs with
PYTHONDONTWRITEBYTECODE=1, and every pytest one with -p no:cacheprovider,
so neither bytecode nor a .pytest_cache is written into the project; a
--test-command or --coverage-command runs exactly as given.

Results are cached in .metrics/mutate/<file>.json, which is meant to be
committed: later runs reuse killed mutants of unchanged functions, and the
survivors itos-cc.yaml excepts, and retry only other survivors and changed
functions. Each snapshot also records the SHA-256
of every test file that reaches its file: in TypeScript, Python, Kotlin and
Dart the tests whose imports reach it, directly or through other modules, as
vitest related and jest --findRelatedTests select them, and through the
test-support files they import (a helper under tests/ or __tests__, a Kotlin
helper in src/test, a Dart one under test/), which are among them too, and
in Python through every conftest.py in a test's directory and those above
it, up to its build root; in Dart only those its own tests run, the test
files under its package's test/ and the support files on their way, so a
change under integration_test/ or test_driver/ makes nothing stale;
in Go its package's tests and those of the packages that import it, one hop. When one of them
is added, changed, or removed, every mutant of the file runs again. Each outcome records the
scope of the tests that decided it: "own" (the file's own tests, left out of
the file), "all-tests", or the --test-command line; a reused outcome keeps
the scope it was decided with, and mutation sample re-runs it there. A
TypeScript file whose project runs its test script without Vitest or Jest
has no narrower own tests than that script, the whole suite, so
its outcomes are recorded as "all-tests", with the whole suite's evidence,
and any test change makes them stale; one recorded before as "own" reads
stale once. Vitest runs a project whose package.json declares vitest, or
where a vitest resolves from its node_modules/.bin and its test script
names vitest, vp or vite or a vite.config.* or vitest.config.* sits beside
its package.json; Jest runs one that declares jest. A summary comment is kept at
the end of each source file; it lists each survivor, and each one itos-cc.yaml
excepts apart, with its reason.

For Go outcomes recorded with --all-tests or --test-command, freshness also
depends on every _test.go file beneath the source's nearest go.mod (nested
modules excluded, build-tagged tests included) and on files matched by
mutation.tests.support. Name feature files and other command inputs there;
inputs outside those files cannot be inferred. These checks use saved hashes
and never run a test or list command. Reusing a broad-scope result keeps its
scope and evidence; a partial run does not refresh evidence for functions it
did not judge. Legacy broad-scope Go results without this evidence rerun once.
A Dart outcome recorded with --all-tests depends on the test files under
its package's test/ alone, those flutter test runs with no path, and one
recorded with --test-command on every Dart test file beneath its nearest
pubspec.yaml.

--since REF judges only the functions the commits since REF changed, as a
gate on a branch's own work: git diff REF...HEAD, committed changes only.
Paths narrow it to the files under them, and the sources itos-cc.yaml's
exclude names are dropped before the range is, so a gate's command stays
the same. Functions not judged neither run nor change in the snapshot, and only those judged count in the summary, the
problems, and the exit code. A file with no function judged is left as it
was: neither its snapshot nor its summary comment is written. Renames are
followed: a move is no change, and a renamed file's snapshot moves with it.

A snapshot whose source file is gone, nothing at its path in the working
tree, is an orphan, unless git shows the file renamed to a file that
exists: in the --since range, or in the working tree against HEAD, staged
as git mv leaves it or added. Such a snapshot is kept for the --since run
over the new path, which moves it. A file deleted, renamed to a path
where nothing is, or not tracked by git is an orphan, and so is one moved
with a plain mv: git sees a deletion and an untracked file, so stage a
move (git mv, or git add) before a run. Outside a git repository, or when
git fails, no snapshot can be judged, and none is removed. A file outside the project
root, whose snapshot is named by its absolute path, is the file the
snapshot records, never the path that name maps to under the root: its
snapshot is no orphan while it exists. Once it has written what it
judged, a run removes the orphans under its paths, the working directory
when it is given none, and every directory that leaves empty under
.metrics/mutate; it keeps every snapshot whose file exists, even a new
file's at an old path, whose functions go stale by hash. It names each one,
a line "snapshot <snapshot> was an orphan, and is removed: its source
<file> is gone", and in --json under "orphans". The snapshot of a source
that itos-cc.yaml's exclude leaves out is an orphan too, though its file
is there, named "…: its source <file> is excluded". A counted run writes nothing, so it
removes nothing.

A Go file only another system builds is not judged here: its name's
_GOOS or _GOARCH suffix, or its //go:build line, rules out the GOOS and
GOARCH go env reports, whatever its other tags. The run leaves it out,
says "itos-cc: not built on <goos>/<goarch>, so <file> is not judged here"
on stderr, lists it under "not_built" in --json, and fails nothing for
it, --fail-uncovered included. Its snapshot, which a run on a system that
builds it writes, is left as it is, and is no orphan; a counted run
selects none of its sites. A file only a tag of the project's own leaves
out, such as integration or ignore, is judged as before, and so is a
branch on runtime.GOOS: its lines no test here executes are uncovered.

--count N judges at most N mutation sites freshly, as a bounded check
that needs no cache: the cache neither saves a trial nor is written. It
resolves the Git repository and its HEAD commit once and judges that
commit's tracked files, frozen in a private copy under the checkout's git
directory (git rev-parse --git-path itos, a linked worktree's own
included), so staged, unstaged and untracked changes play no part. It
warns on stderr when such a change goes unjudged, naming the commit and
each source, test or itos-cc.yaml whose working tree differs from it,
staged, unstaged, or untracked and not ignored, and --json lists them as
the sampling's "uncommitted"; the warning changes neither the judgment nor
the exit code. Tools, dependencies and the environment are used as
installed, from the live project, and nothing is installed or
downloaded: TypeScript's node_modules and Python's .venv or venv (else the
virtualenv VIRTUAL_ENV names) at its build roots, the .fvm/flutter_sdk FVM
links beside a committed .fvmrc, and the Go module, Gradle, Maven and pub
caches, with Go commands offline (GOPROXY=off), Gradle --offline and Maven
-o. Each Dart package is readied once with flutter pub get --offline
--enforce-lockfile, resolving the commit's pubspec.lock from the pub cache,
and its tests run with --no-pub. A dependency missing offline fails the run, as
count.preparation-failed naming the stage. Only a language with something
to judge is measured: one with an eligible site in the selection, and Go,
whose admitted functions strict Go coverage judges, so a file of another
language with no site needs no tool. The sites of the selection,
paths and --since REF's changed functions, are ranked by SHA-256 over
--seed TEXT (the HEAD commit's id unless given) and each site's identity,
and the first N are selected across every file, function and worker.
Coverage, listed reach and a clean baseline of each selected file's own
tests are then measured on the frozen copy; any command failing fails the
run before any mutant. A selected site no test reaches is reported
uncovered, never run and never redrawn. Every other selected site runs its
own tests once, even when the cache holds a kill for it, and one that
survives them runs the listed tests that reach its line, after their
selection's clean baseline, as a complete run does: both stages are one
trial. A site of a Python, Kotlin or Dart file no own test reaches, whose own
tests run nothing, needs no clean baseline of them: listed tests that
reach its line judge it alone, as in a complete run. A selection whose
baseline fails is tests.selection-failed, and a
site that needs it is blocked, with no outcome. Exceptions of the functions
with a selected site apply as in a complete run: a valid one excepts its
survivor, which is still drawn and run, and a stale one fails, as
mutation.exception-stale. --count bounds mutant trials only, not discovery,
coverage, listing, baselines or total time. A counted run writes no
snapshot, summary comment or coverage cache, and its pass proves only the
judgments it reports, never a complete result: mutation check reports the
cache as it was. --fail-uncovered keeps its meaning, strict Go included,
for every admitted function, and =lines narrows it to the changed lines
as in a complete run; a counted run still proves only Go's lines
strictly: strict Python, TypeScript, Kotlin and Dart lines are not yet
proven by it. A range with no site is not applicable, which
is not a pass of a range whose tests or measurement failed. --count runs
on Linux and macOS, where every command it starts runs in a process group
it owns (a command that detaches from its group or session is not
followed); Windows is #29. It refuses --changed, --no-coverage, existing
or raw coverage, --test-command and --mutate-all, and --seed needs it. With --all-tests the own stage runs the whole suite, and a
survivor still runs the listed tests that reach it.

SIGINT or SIGTERM interrupts a run, complete or counted, as it does every
command: no command starts after it, the judgment then running is
cancelled with no outcome, and every process group the run owns is killed
and joined within one shared five-second deadline before its copies are
removed, a second interrupt waiting for that, then ending the run at once.
It reports interrupted, exit 130. A complete run keeps what it finished as
a --fail-fast stop does, each file with its "state" and "work", and a file
it cut short gets no summary comment; a counted run's report, completion
"interrupted", keeps the judgments completed before it. An owned group
whose cleanup does not finish is mutation.cleanup-failed, exit 100, beside
the run's other problems, one for each group.

--fail-fast stops a complete run at the first actionable failure it
observes, in the order judgments finish, for a quick fix-and-retry loop:
an unexcepted survivor, once the listed tests that reach it failed to kill
it too; with --fail-uncovered, an uncovered mutant or a strict Go, Python,
TypeScript, Kotlin or Dart coverage finding; an exception that no longer holds; a file whose tests fail before
any mutant; or a selection of listed tests that fails without any mutant.
It reports that failure under the rule a run without it reports, and exits
as that rule says. Killed, timed-out, validly excepted and listed-killed
mutants never stop it. A failure planning or coverage already shows, such
as a stale exception or an uncovered mutant with --fail-uncovered, stops it
before any baseline or mutant runs; listing and coverage keep their scope,
so it bounds no total time. At the stop no further command starts, each
judgment still running is cancelled, with no outcome (never killed or timed
out), and every process the run started is stopped and joined within one
shared five-second deadline from the stop, before its worker copies are
removed. A file whose every selected mutant was decided is written as
without --fail-fast. A file the stop cut short, or whose mutant a failing
selection of listed tests blocked, gets no summary comment, its source
left as it was, but its snapshot keeps every judgment the run completed or
reused, with the scope and the freshness evidence that decided it; a
cancelled, blocked or unattempted mutant gets no outcome, and keeps what
the snapshot recorded for it that still holds, if anything, so a stopped
--mutate-all rerun never drops a valid result. mutation check still fails
a function with a mutant no valid result records, and a later run reuses
what was kept while its inputs hold and runs only the rest. A file whose
baseline failed, or which the run judged nothing anew in, is left as it
was. With --json, "stop" says whether the run stopped early and at which
rule and subject, "work" counts the selected mutants completed (run,
reused or measured uncovered), cancelled, unattempted and blocked,
disjointly, and each file has its "state" and "work", the run's own work
on it, with "ran" and "reused" counting the mutants it ran and reused of
it in every state, blocked included, its completed work less any measured
uncovered, its "cache", whether its snapshot as the run leaves it holds a
valid result for every mutant of its functions judged, its "baseline" as
it ran ("not-run" when it never did), and every mutant of its functions
judged with its "state", an outcome only when completed. --fail-fast runs on Linux and macOS, where every command it
starts runs in a process group it owns (a command that detaches from its
group or session is not followed); on Windows it is fail-fast.platform,
before any command runs (#29). It refuses --count, whose runs stay
aggregate. Without it, scheduling, output, snapshots and comments are as
they always were.

--fail-uncovered makes each uncovered mutant a failure, listed like a
survivor. For Go, Python, TypeScript, Kotlin and Dart it also requires fresh
measured executable coverage for every judged function, including functions
with no mutation sites, and fails, as mutation.uncovered-statement, each
uncovered positive-weight Go coverage block, each line a Python, TypeScript
or Dart LCOV report names executable that no test executed, and each
line of a Kotlin JaCoCo or Kover XML report none of whose instructions ran
(a line with some run, such as one a test took one branch of, is covered):
coverage.py's over the tests that reach the file, Vitest's v8 or Jest's
over its related tests, or c8's over the test script of a package with
neither, Gradle's jacocoTestReport or koverXmlReport, or Maven's
jacoco:report, over the test classes of the file's module that reach it,
and flutter test --coverage's over the test files under the package's test/
that reach it, the whole suite with --all-tests or --test-command. A
function's lines run from its body's first statement to its end: a Python
def line, or a TypeScript arrow function's declaration, runs at import, and
a Kotlin declaration's line holds the bridge that fills in default
arguments. A Dart function's lines run from its signature line, which the
Dart VM counts at each call, and a line any of whose code ran is covered. A
Python file no test reaches has its executable lines listed by coverage.py's
own analysis, a TypeScript file no test loads by Vitest's v8 provider run
with no test, and a Kotlin file no test reaches by the report of another
file of its module, which lists every class the module compiled, all of
them uncovered; under Jest or c8 such a file, a Kotlin one whose module
measured no other file, a Dart library no test loads, which flutter test's
report never names, and any file a project's own coverage script
measured (--all-tests with one), has no evidence, which is
mutation.coverage-missing. A scope the fingerprints cannot prove is
mutation.coverage-unsupported for every judged function of its root, even
with fresh evidence: an active Go workspace or a local replacement outside
the module; an npm or yarn workspace, a workspace: dependency, a file: or
link: dependency or pnpm override outside the package root, a pnpm
workspace's own root package, or a member depending on another by name;
a pnpm-workspace.yaml with no packages is settings, and a member's
evidence holds its workspace root's config, .npmrc, .pnpmfile.cjs and
patches; a Python path or editable dependency outside the build root
(pyproject.toml, requirements files, the root's virtualenv); a Gradle
includeBuild, or a Maven module, subproject or parent relativePath, outside
the build root; a pub path dependency or override, in a pubspec.yaml or
pubspec_overrides.yaml, outside the package root, or a pub workspace's root
for a member. They are found in the root's own files, and nothing outside
the project root is read.
Strict runs reject --no-coverage,
--coverage-report, --use-existing-coverage, and --coverage-command. A
matching independent cache can avoid remeasurement; otherwise built-in or
listed coverage must measure successfully. Empty or comment-only function
bodies have no executable coverage obligation. For the mutants of
TypeScript, Python, Kotlin and Dart it fails closed as Go does: a build root with
mutants to judge whose coverage tool is missing, or whose coverage command
fails or writes no report, is coverage.tool-missing or
coverage.measured-nothing, naming its language and dir, and the run stops
before any mutant runs. Without --fail-uncovered such a file runs every
mutant, as it always did, and so it does with --coverage-report,
--use-existing-coverage or --coverage-command. With --since, only the judged
functions' uncovered mutants and executable blocks count.

--fail-uncovered=lines, which needs --since, judges only where the range
changed: an executable Go coverage block or Python, TypeScript, Kotlin or
Dart line counts when it overlaps a line the commits since REF added or
changed, the new side of each hunk of git diff --unified=0 REF...HEAD, a
renamed file's under its new path, and an uncovered mutant fails only on
such a line. A judged function with such a line still needs complete
evidence, as mutation.coverage-missing says, as its changed lines cannot
be judged without it; a function with none, such as one the range only
deleted lines from, is judged for survivors and exceptions alone.
Survivors and exceptions are judged as before. Evidence is measured and
recorded for whole functions, so a later --fail-uncovered check of the
same snapshot judges them whole. --fail-uncovered alone is
--fail-uncovered=functions, whole functions. =lines holds in --count and
--fail-fast runs too; without --since it is flags.conflict.

A survivor that itos-cc.yaml excepts (see mutation except) fails nothing: it
is counted excepted, not survived, and is reused without running, as a kill
is, while its function and the tests that import its file are unchanged.
An exception no longer holds, and fails as mutation.exception-stale, when
its function changed (its mutant is then judged as if it had none), when
its function or its site is gone, or when the mutant, run again after its
tests changed, is killed. An exception never excuses an uncovered mutant.
With --since, only the judged functions' exceptions count. --mutate-all
runs excepted mutants too.

An entry for a file the run does not select is judged too by a run of the
whole project (no paths, no --changed, no --since), and by a --since run
whose range deleted or renamed its file. When exactly one source selected
holds a function of the entry's name and hash, the entry has moved: it
still excepts the mutant at its site there, and fails as stale with why
moved and new_file, the file to name in the entry. Otherwise it is gone.
A run never writes itos-cc.yaml. An itos-cc.yaml that cannot be read is
config.invalid, and nothing runs.

Tests that only run the built program, such as an end-to-end suite that a
single test function runs, can be listed in itos-cc.yaml under
mutation.tests, so each mutant runs only those that reach its line:
  mutation:
    tests:
      list: <command printing a test a line: its ID, then a tab and its file, or not, then a tab and its fingerprint, or not>
      run: <command running the tests {pattern} selects>
      ids_pattern: <the pattern, with {ids}>
      join: {each: <each ID, with {id}>, sep: <between them>}
      whole: <command running every test; optional, else run with every ID>
      support: [<globs of files every test depends on, optional>]
Commands run through the platform shell at the project root. When a mutant
has to run, the list command runs once, then every listed test, for
coverage, with ITOS_CC_TEST_COVERDIR set to a directory of the run's own:
a harness that gives the processes each test starts
GOCOVERDIR=<that directory>/<test ID>, COVERAGE_FILE=<that
directory>/<test ID>/.coverage to Python ones, NODE_V8_COVERAGE=<that
directory>/<test ID> to Node ones, or a JaCoCo destfile under <that
directory>/<test ID>/ to JVMs, splits the coverage by test in one run;
otherwise each test runs alone, every process it starts with a
GOCOVERDIR, a coverage.py data file, a NODE_V8_COVERAGE and an
ITOS_CC_JACOCO_DESTDIR of its own. The run of every test sets
ITOS_CC_TEST_COVERDIR without GOCOVERDIR: a harness builds its binary
with -cover only when it gives the processes GOCOVERDIR, or gives each
test's processes GOCOVERDIR=<that directory>/<test ID> itself; otherwise
a -cover binary warns on stderr that it wrote no data. A coverage run
that fails, of every test or of one alone, is tests.coverage-failed:
nothing is judged and no snapshot is written. A
mutant runs its file's own tests first and, only if it survives them, the
listed tests that reach its line, in one run: its outcome then has scope
"listed" and records their IDs. A line a listed test reaches is never
uncovered. The first time a mutant needs a selection of listed tests,
that selection runs once without any mutant, in the mutant's worker's
copy: its time, output and memory bound every mutant run of it, as a
file's baseline bounds its mutants'. A selection that fails without a mutant is
tests.selection-failed: no mutant that would run it is judged, and the
files holding them keep their snapshots, as with a failing baseline
("baseline": "failed" in --json). Such
an outcome holds while the files its tests are defined
in, and the support files, are as it recorded them; mutation check calls
its function stale when one changed. A test listed with a fingerprint, a
third column the list command computes from the test's own text, dates
the outcomes resting on it by that fingerprint instead of its file: they
hold while the list prints the test with it, whatever else its file holds
and whichever file names it. The list command then runs, once a command,
only when such a test's file changed. A list command that fails is
tests.list-failed: nothing is
judged and no snapshot is written. Listed tests are not run with
--no-coverage, --all-tests, --test-command, --use-existing-coverage,
--coverage-command, or --coverage-report.`,
		JSON: `"files": [{"file", "killed", "survived", "excepted", "uncovered", "ran",
   "reused", "baseline": "passed"|"failed"|"not-run", whether this run
   ran the file's own tests without any mutant: "passed" only when they
   ran and passed in this run, for it or for another file sharing its
   command, as a Go package's files do; "not-run" when none ran, as for a
   file whose mutants were all reused, which said "passed" before, or a
   Python, Kotlin or Dart file no test reaches (a script that checked for
   "passed" should accept "not-run", or check for "failed"), "mutants": [{"line", "column",
   "function", "original", "replacement",
   "outcome": "killed"|"survived"|"timeout"|"over-limit"|"uncovered",
   with "over-limit" "limit": "output"|"memory", "reused",
   "scope": "own"|"all-tests"|"listed"|"<test command>", for an
   excepted survivor "excepted": "its reason", where this run's coverage
   executed its line "coverage": ["in-process", "integration"], either or
   both, and with scope "listed" "tests": ["<test ID>"]}], and with
   --since
   "judged": ["namespace#name"], and with --fail-fast "state":
   "completed"|"stopped"|"unattempted"|"blocked", "work",
   "cache": "complete"|"incomplete", every
   mutant "state":
   "completed"|"cancelled"|"unattempted"|"blocked", and "outcome" and
   "scope" for completed ones only}]; with --fail-fast, "stop":
   {"stopped", "rule", "subject"}, the problem's rule and subject keys, and
   "work": {"completed", "cancelled", "unattempted", "blocked"};
   when it removed any, "orphans": [{"file", "snapshot", "why"}], each
   snapshot whose source file was gone or excluded, "snapshot" its path
   from the project root, "why" "gone"|"excluded"; when it left any out,
   "not_built": [{"file", "platform": "<goos>/<goarch>"}], each Go file
   only another system builds, with --count too, its "file" from the
   repository's top; with
   --count, instead of "files" and "orphans":
   "sampling": {"budget", "eligible", "selected", "executed", "omitted",
   "seed", "algorithm", "commit", "since", "since_base",
   "assurance": "sampled"|"not-applicable",
   "completion": "completed"|"stopped"|"interrupted"|"not-applicable",
   "stop", "bounds", "uncommitted": ["<path>"]},
   "selected": [{"identity", "file", "line", "column", "function",
   "original", "replacement",
   "state": "judged"|"uncovered"|"blocked"|"failed"|"cancelled"|"unattempted",
   "outcome" (judged and uncovered sites only), with "over-limit"
   "limit": "output"|"memory", "scope", with scope
   "listed" "tests": ["<test ID>"], for an excepted survivor "excepted":
   "its reason", "reason", and the stages its trial ran, "stages":
   [{"name": "own"|"listed-baseline"|"listed",
   "state": "complete"|"failed"|"aborted", "outcome", "tests", "error"}]}],
   "subjects": {"judged": [{"file", "function"}], "omitted": [...]},
   "stages": [{"name", "state": "complete"|"failed"|"skipped"|"aborted", "error"}]`,
		Rules: []string{
			"mutation.survived         a mutant survived: file, line, column, function, original, replacement; with --count identity, which the message names and the fix's mutation except takes",
			"mutation.uncovered        with --fail-uncovered, no test executes a mutant: file, line, column, function, original, replacement",
			"mutation.uncovered-statement with --fail-uncovered, a measured executable Go coverage block or Python, TypeScript, Kotlin or Dart line is uncovered: file, function, line",
			"mutation.coverage-missing  with --fail-uncovered, a Go, Python, TypeScript, Kotlin or Dart function lacks complete measured coverage evidence: file, function, line",
			"mutation.coverage-unsupported with --fail-uncovered, strict coverage reaches beyond the inventoried root: a Go workspace or outside replacement, an npm or yarn workspace, a pnpm workspace root package or a member depending on another, or a local dependency, pnpm override, includeBuild, Maven module or pub path dependency or override outside it: file, function, line",
			"coverage.tool-missing     with --fail-uncovered, a language's coverage tool, or its collector of the processes tests start (a coverage.py that does not start in them, no c8, no jacococli for the .exec files a JVM wrote), is missing where it has mutants to judge: language, dir",
			"coverage.measured-nothing with --fail-uncovered, a language's coverage command failed or wrote no report where it has mutants to judge: language, dir",
			"mutation.exception-stale  an exception in itos-cc.yaml no longer holds: file, function, line (none when the function or its file is gone, or its file excluded), column, original, replacement, why: killed|changed|gone|moved|excluded, and with moved new_file",
			"mutation.baseline-failed  the tests fail before any mutant: file, output, the head and tail of what they printed",
			"config.invalid            itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"tests.list-failed         the list command of mutation.tests failed: command, exit_code",
			"tests.coverage-failed     the listed tests' coverage run failed without any mutant: command, exit_code",
			"tests.selection-failed    a selection of listed tests fails without any mutant: ids, command, exit_code",
			"since.bad-ref             --since names no commit: ref",
			"since.no-git              --since outside a git repository",
			"flags.conflict            --since with --changed: flag",
			"flags.conflict            --fail-uncovered with --no-coverage, or strict Go, Python, TypeScript, Kotlin or Dart coverage with raw coverage flags: flag",
			"flags.conflict            --fail-uncovered=lines without --since: flag",
			"flags.conflict            --seed without --count, or --count with a flag it refuses, --fail-fast included: flag",
			"count.platform            --count on a platform other than Linux and macOS, that is Windows (#29): platform",
			"fail-fast.platform        --fail-fast on a platform other than Linux and macOS, that is Windows (#29), before any command runs: platform",
			"count.no-git              --count outside a Git repository, or before its first commit",
			"count.unsupported-scope   --count over committed content it cannot judge, such as a symlink or a submodule",
			"count.preparation-failed  with --count, a runtime Git, tool, listing, coverage, conversion or baseline step failed before any mutant, or the private scratch directory is unusable (stage scratch): stage",
			"count.trial-failed        with --count, a selected mutant's trial could not run: file, line, column, function, original, replacement, identity",
			"mutation.cleanup-failed   an owned process group's cleanup did not finish by its deadline, its judgment left with no outcome: process_group, command",
			"interrupted               SIGINT or SIGTERM stopped the run, whose report keeps what it finished",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "every mutant that ran was killed"},
			{Code: 1, Meaning: "a mutant survived, a mutant is uncovered with --fail-uncovered or coverage measured nothing where it has mutants to judge, an exception is stale, a file's tests fail before any mutant, the list command of mutation.tests failed, a selection of listed tests fails without any mutant, or with --count a preparation step failed or a selected mutant is not judged"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag or path, a --since ref that is no commit, --since with --changed, --fail-uncovered=lines without --since, a --count below 1, --seed without --count, committed content --count cannot judge, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "--changed or --since outside a git repository; with --fail-uncovered, a coverage tool missing where it has mutants to judge; --count outside a Git repository with a commit, on Windows, or with a required tool missing; --fail-fast on Windows"},
		},
		Examples: []string{
			"itos-cc mutation run --changed",
			"itos-cc mutation run --since origin/main --fail-uncovered  # a branch's own commits, as a gate",
			"itos-cc mutation run --since origin/main --fail-uncovered=lines  # coverage of the changed lines alone",
			"itos-cc mutation run --all-tests --json                    # nightly",
			"itos-cc mutation run --count 20 --since origin/main        # a bounded fresh check of committed work (Linux and macOS)",
			"itos-cc mutation run --changed --fail-fast                 # stop at the first survivor while fixing (Linux and macOS)",
		},
	}
}
