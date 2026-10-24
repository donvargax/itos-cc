Feature: Mutation testing
  "Would the tests notice if this code were wrong?" itos-cc changes one
  operator, boolean, or 0/1 at a time inside each function and runs the tests
  that cover the file. A mutant is killed when the tests fail or time out,
  survives when they pass, and is uncovered when no test executes its line.

  Rule: Mutation sites

    @ID-MUT-01
    Scenario: TypeScript sites
      Given the file "x.ts":
        """
        const LIMIT = 1 + 2;
        function f(xs: Array<number>, a: number): boolean {
          // a < b in a comment
          const s = "x > y";
          if (a > 0 && !done) return a === 1;
          return -a <= 10 || true;
        }
        """
      When I run "itos-cc mutation list x.ts"
      Then the sites are:
        | line | original | replacement |
        | 5    | >        | >=          |
        | 5    | 0        | 1           |
        | 5    | &&       | \|\|        |
        | 5    | !        | (deleted)   |
        | 5    | ===      | !==         |
        | 5    | 1        | 0           |
        | 6    | -        | (deleted)   |
        | 6    | <=       | <           |
        | 6    | \|\|     | &&          |
        | 6    | true     | false       |
      And code outside functions, comments, strings, and type arguments hold no sites

    @ID-MUT-02
    Scenario: Nullish coalescing and optional chaining
      Given the file "x.ts":
        """
        export function place(a, b) {
          return a ?? b?.c ?? b?.[0];
        }
        """
      When I run "itos-cc mutation list x.ts"
      Then the sites are:
        | line | original | replacement |
        | 2    | ??       | \|\|        |
        | 2    | ?.       | .           |
        | 2    | ??       | \|\|        |
        | 2    | ?.       | (deleted)   |
        | 2    | 0        | 1           |
      And a?.() becomes a(), as a?.[0] becomes a[0]
      And .js, .jsx, .mjs, and .cjs files have the same sites

    @ID-MUT-03
    Scenario: Python sites
      Given the file "x.py":
        ```
        LIMIT = 1 + 2

        def f(a, b):
            """a < b"""
            if a >= 0 and not b:
                return a * 2 == 1
            return -a < b or False
        ```
      When I run "itos-cc mutation list x.py"
      Then the sites are:
        | line | original | replacement |
        | 5    | >=       | >           |
        | 5    | 0        | 1           |
        | 5    | and      | or          |
        | 5    | not      | (deleted)   |
        | 6    | *        | /           |
        | 6    | ==       | !=          |
        | 6    | 1        | 0           |
        | 7    | -        | (deleted)   |
        | 7    | <        | <=          |
        | 7    | or       | and         |
        | 7    | False    | True        |

    @ID-MUT-04
    Scenario: Go sites
      Given the file "x.go":
        """
        package p

        var limit = 1 + 2

        func f(a int, xs []int) bool {
        	var m map[string][]int
        	_ = m
        	if a != 0 || !ok(xs) {
        		return a-1 > len(xs)
        	}
        	return true
        }
        """
      When I run "itos-cc mutation list x.go"
      Then the sites are:
        | line | original | replacement |
        | 8    | !=       | ==          |
        | 8    | 0        | 1           |
        | 8    | \|\|     | &&          |
        | 8    | !        | (deleted)   |
        | 9    | -        | +           |
        | 9    | 1        | 0           |
        | 9    | >        | >=          |
        | 11   | true     | false       |

    @ID-MUT-05
    Scenario: Kotlin sites
      Given the file "x.kt":
        """
        val limit = 1 + 2

        fun f(a: Int, xs: List<Int>): Boolean {
            if (a == 0 && !xs.isEmpty()) return false
            return a * 2 >= xs.size
        }
        """
      When I run "itos-cc mutation list x.kt"
      Then the sites are:
        | line | original | replacement |
        | 4    | ==       | !=          |
        | 4    | 0        | 1           |
        | 4    | &&       | \|\|        |
        | 4    | !        | (deleted)   |
        | 4    | false    | true        |
        | 5    | *        | /           |
        | 5    | >=       | >           |

    # slice-29, dart-support: a Dart mutant that does not compile still
    # costs a flutter test start, so Dart's sites are the swaps that compile
    # whatever their operands' types. Not * and /, as int / int is a double;
    # not ?. to ., as the receiver is nullable; not the postfix null
    # assertion x!. a ?? b becomes b: the left operand and ?? are deleted,
    # since TypeScript's ?? to || does not compile in Dart. + to - fails
    # only on a String or a List; such a mutant counts as killed, as a Go
    # or Kotlin mutant that fails to build does (q-83; a separate outcome
    # for every compiled language waits on the idea mutant-not-compiled).
    @slice-29 @ID-MUT-262
    Scenario: Dart sites
      Given the file "x.dart":
        """
        const limit = 1 + 2;

        bool f(int a, List<int> xs, String? name) {
          // a < b in a comment
          final s = 'x > y';
          if (a == 0 && !xs.isEmpty) return false;
          final n = name ?? 'none';
          return a * 2 >= xs.length + 1 || -a < n.length && name!.isNotEmpty;
        }
        """
      When I run "itos-cc mutation list x.dart"
      Then the sites are:
        | line | original | replacement |
        | 6    | ==       | !=          |
        | 6    | 0        | 1           |
        | 6    | &&       | \|\|        |
        | 6    | !        | (deleted)   |
        | 6    | false    | true        |
        | 7    | name ??  | (deleted)   |
        | 8    | >=       | >           |
        | 8    | +        | -           |
        | 8    | 1        | 0           |
        | 8    | \|\|     | &&          |
        | 8    | -        | (deleted)   |
        | 8    | <        | <=          |
        | 8    | &&       | \|\|        |
      And code outside functions, comments, strings, and type arguments hold no sites

    @ID-MUT-06
    Scenario: Scanning prints sites without running tests
      When I run "itos-cc mutation list src/board.py"
      Then each site is printed as "file:line:column `original` → `replacement` in namespace#name"
      And no tests are run
      And the exit code is 0

    # mutation list listed sites function by function, so the sites of an
    # inline callback (app.get("/u", (req, res) => …)) came after every site
    # of the function around it, while mutation run --json lists a file's
    # mutants by line and column (ID-MUT-52). Both list in line and column
    # order (the coordinator's call, 2026-10-07).
    @mutation-list-line-order @ID-MUT-101
    Scenario: Sites are listed in line order, inline callbacks included
      Given src/app.ts has a function with a site before, inside and after an inline callback
      When I run "itos-cc mutation list src/app.ts"
      Then its sites are listed in line and column order
      And in the order "itos-cc mutation run --json src/app.ts" lists the file's "mutants"

  Rule: Running mutants

    # Binary-only for good: which command runs a project's tests is the
    # toolchain's question, held against the real tools, as COV-05's is.
    # The no-pytest row predated own-tests-python (83ae772): a file's own
    # tests run as their modules, and discover is the --all-tests form. The
    # coordinator settled the row on 2026-10-13, and each row lists words
    # the command holds, in order.
    @ID-MUT-07 @binary-only
    Scenario Outline: Each file runs its narrowest test command
      Given a source file in a <language> project with <setup>
      When its mutants run
      Then the test command is <command>

      Examples:
        | language   | setup             | command                                                          |
        | Go         | go.mod            | go test -count=1 -failfast on the file's package                 |
        | TypeScript | vitest            | the installed vitest related --run --bail=1 on the file          |
        | TypeScript | jest              | the installed jest --bail --findRelatedTests on the file         |
        | TypeScript | neither installed | <pm> run test, the package manager the project declares          |
        | Python     | pytest importable | python -m pytest -q -x -p no:cacheprovider                       |
        | Python     | no pytest         | python -m unittest -f <the file's test modules>                  |
        | Kotlin     | Gradle            | gradle test --fail-fast for the module, via ./gradlew if present |
        | Kotlin     | Maven             | mvn -q test                                                      |

      # slice-35, dart-support: flutter test has no fail-fast flag; the
      # files are the test files that reach the source (slice-34), and an
      # FVM-pinned SDK is preferred as coverage prefers it (slice-32).
      @slice-35
      Examples:
        | language   | setup                        | command                                                     |
        | Dart       | a resolved Flutter package   | flutter test --no-pub on the test files that reach the file |
        | Dart       | .fvmrc and .fvm/flutter_sdk  | .fvm/flutter_sdk/bin/flutter test --no-pub                  |

    # Binary-only for good: the command coverage runs is the real tools',
    # as COV-03's is.
    @ID-MUT-08 @binary-only
    Scenario: By default a file is measured and mutated by its own tests
      Given a/a.go is tested only by e2e/e2e_test.go, which imports package a
      When I run "itos-cc mutation run a/a.go"
      Then coverage runs "go test -count=1 -covermode=set -coverprofile=... example.com/m/a"
      And its mutants are uncovered and none runs
      # coverage comes from the tests that kill mutants, so a line only
      # other tests reach is uncovered, never a false survivor

    # Binary-only for good: the whole suite's commands are the real tools',
    # as COV-04's are.
    @ID-MUT-09 @binary-only
    Scenario: The whole suite, as a nightly job
      When I run "itos-cc mutation run --all-tests a/a.go"
      Then coverage and every mutant run the whole suite of its build root: go test ./..., vitest run, jest, or gradle test
      And the e2e test kills them
      # tests that only run the built binary are not in coverage; with
      # --no-coverage too, every mutant runs and they can kill it
      And a later run without --all-tests reuses those kills for functions that have not changed while the Go module tests and support files remain unchanged

    # Binary-only for good: the platform shell runs it, as COV-17's.
    @ID-MUT-10 @binary-only
    Scenario: A custom test command
      When I run "itos-cc mutation run --test-command 'make test' src/board.go"
      Then "make test" runs through the platform shell from the file's build root

    # Binary-only for good: the worker copies and their links are real
    # folders the tests run in.
    @ID-MUT-11 @binary-only
    Scenario: The real tree is never modified while tests run
      When I run "itos-cc mutation run src/board.ts"
      Then each worker runs mutants in its own private copy of the project
      And dependency directories such as node_modules and .venv are linked, not copied
      And the source files in the working tree are unchanged while tests run

    # Binary-only for good: PYTHONPATH is what the real interpreter reads
    # in a worker's real copy.
    @ID-MUT-12 @binary-only
    Scenario: Python tests import the worker's copy
      Given a Python project installed in editable mode
      When its mutants run
      Then PYTHONPATH puts the worker's copy of "." and "src" first

    # Binary-only for good: where the baseline runs is a worker's real copy.
    @ID-MUT-13 @binary-only
    Scenario: The baseline proves the copy works
      When mutants are about to run for a test command
      Then that command first runs unmutated inside a worker's copy
      And its duration sets the mutant timeout

    @ID-MUT-14
    Scenario: Timeouts
      Given the baseline took 3 seconds
      When I run "itos-cc mutation run --timeout-factor 10"
      Then a mutant times out after 35 seconds
      And a timed-out mutant counts as killed

    @ID-MUT-15
    Scenario: Fast suites still get the allowance
      Given the baseline took 50 milliseconds
      When mutants run with the default timeout factor of 10
      Then a mutant times out after 5.5 seconds

    # timeout-allowance: a mutant timed out after its baseline's time times
    # the timeout factor, at least 2 seconds. A mutant's run builds the
    # mutated code, which the baseline may have found built already, so on
    # a slow runner (windows-latest, CI runs 37718895582 and 37721105456) a
    # mutant that should survive timed out and counted as killed, hiding a
    # survivor. Decided with the person on 2026-10-07 (q-23): a timeout is
    # the baseline's time times the factor plus a fixed 5 seconds, as PIT and
    # Stryker add a constant, and the 2-second minimum goes. It holds for a
    # file's tests, a --test-command or --all-tests run, each selection of
    # listed tests (ID-MUT-135) and mutation sample alike. ID-MUT-14, 15 and
    # 135 said the old rule and change with the fix.
    @timeout-allowance @ID-MUT-149
    Scenario: A mutant's timeout leaves a fixed allowance beyond its baseline
      Given the baseline took 50 milliseconds
      When mutants run with the default timeout factor of 10
      Then a mutant times out after 5.5 seconds: ten times the baseline, plus 5 seconds

    @ID-MUT-16
    Scenario: A failing baseline
      Given the tests of src/board.py fail without any mutation
      When I run "itos-cc mutation run src/board.py"
      Then stdout says "src/board.py: baseline tests fail; snapshot not updated"
      And the test output follows whole, as it is under 40 lines
      And the snapshot of src/board.py is not changed
      And the problem is "mutation.baseline-failed", with file
      And the exit code is 1

    # slice-24, issue #47: a failing baseline printed only the last 20 lines
    # of its output (Tail in presentation.go, used by run_ui.go and
    # sample_ui.go). Vitest prints the cause first and the bulk last, so the
    # cause was cut off, and with --json the output appeared nowhere:
    # mutation.baseline-failed carried only file (#47). Decided in the item:
    # keep the head and the tail of the output, shared with the baseline's
    # report. Coordinator's calls, 2026-10-21: the report gives the first 20
    # lines and the last 20, and between them one line, "… 60 lines left out
    # …", when there are more than 40; a run keeps at most 64 KiB at either
    # end, whatever a line's length, so a baseline's output holds bounded
    # memory too. --json's mutation.baseline-failed gains "output", the same
    # text (a key added). mutation sample's report is the same. A failing
    # selection of listed tests is unchanged. MUT-16's sentence "the last 20
    # lines of the test output follow" becomes "the test output follows
    # whole, as it is under 40 lines" in the feat, its step with it. Both
    # drivers run it, as it needs no real tool. Before slice-24 only the
    # last 20 lines follow, failing at the Then.
    @slice-24 @ID-MUT-303
    Scenario: A failing baseline's report keeps the head and the tail of its output
      Given the tests of src/board.py fail without any mutation, printing 100 numbered lines, the first naming the cause
      When I run "itos-cc mutation run src/board.py"
      Then stdout says "src/board.py: baseline tests fail; snapshot not updated"
      And lines 1 to 20 follow it, then "… 60 lines left out …", then lines 81 to 100
      And the problem is "mutation.baseline-failed", with file "src/board.py"
      And its "output" in --json is those same lines
      And "itos-cc mutation sample src/board.py" reports the failing baseline with the same lines

    # Binary-only for good: mutants running at once are real processes.
    @ID-MUT-17 @binary-only
    Scenario: Parallel workers
      When I run "itos-cc mutation run --workers 4"
      Then up to 4 mutants run at the same time
      And the default is half the CPUs, at least 1

    # slice-13 decisions (coordinator, 2026-10-21: the spec drafter's
    # recommendations, taken as routine calls inside the issue's scope):
    # 1. Every Worker.Run starts from the copy as it was made. Before each command runs in an
    #    existing copy (a file's baseline, a listed selection's baseline, a mutant, a counted
    #    trial; every WorkPurpose), the worker puts the copy back as it was right after
    #    copying and Ready. It removes what earlier runs added, puts back what they changed,
    #    and makes again what they removed. The worker restores rather than copying afresh,
    #    which would cost a full copy, and for Dart a flutter pub get --offline, per mutant.
    # 2. "As it was made" is a record the worker takes when it makes the copy: each file,
    #    folder and symbolic link the copy holds once copyPackages and Ready have run, with
    #    each file's size, mode and modification time and each link's target. The record is
    #    whatever the copy rules produced, so worker-copies-skip-ignored (q-69), which depends
    #    on this slice, only changes copyTree and needs no change here.
    # 3. Order inside boundWorker.Run (workers.go:94-124): restore first, then write the
    #    mutant (withMutant), then run. The restore never runs between writing a mutant and
    #    its run. The copy made by a run's first use is already as made and needs no restore.
    # 4. What the restore leaves alone (q-A). It keeps the folders the copy leaves out, as the
    #    runs leave them. These are skipDirs, build output (project.IsBuildOutputWithIgnore
    #    with no git question, since a copy has no .git) and dartLeftOut's build/ and
    #    coverage/, so Gradle, Cargo, Maven and Flutter's incremental build output and
    #    .dart_tool (Ready's resolution, which slice-35 put in every copy) persist as they do
    #    today. It also keeps the linked dependency folders (linkDirs), which are the
    #    project's own and shared, and never walks into them: a link that a run removed is
    #    made again. .pytest_cache needs no exception, since every pytest command itos-cc
    #    composes runs with -p no:cacheprovider (ID-MUT-07). A --test-command that writes one
    #    writes it in a left-out folder, as today.
    # 5. What a changed file is (q-B). A file whose size, mode or modification time differs
    #    from the record, as rsync's quick check decides. The worker copies the file again
    #    from the project, as the copy was made from it in the first place. Keeping a second
    #    pristine tree per pool is not worth its disk. The mutated file's own write and put-
    #    back counts as a change like any other, or the record follows it. The agent chooses.
    # 6. A restore that fails (q-C), such as a folder a test made unreadable, discards the
    #    copy and copies afresh with Ready, as copyOf already does when Ready fails
    #    (worker.go:69). If that fails too, it is WorkerFailed, never a judgment.
    # 7. The restore is not timed. It happens before the run's timeout context starts
    #    (worker.go:190-195) and stays out of CommandResult.Elapsed. A baseline's time, and so
    #    every timeout derived from it (ID-MUT-14, 135, 149), is the command's alone. A done
    #    context stops the restore as WorkerCancelled.
    # 8. Only the copy the command runs in is restored. A worker with copies of several trees
    #    restores each when a command next runs there.
    # 9. The port says it: Worker's doc (internal/slices/mutation/domain/workers.go:25-30)
    #    says each run starts from the copy as it was made. README's "Parallel workers" bullet
    #    says it in a sentence, naming what stays (build output, caches, .dart_tool, the
    #    linked dependencies).
    # 10. Out of scope. A coverage run, which runs in the project's own folder, leaving a file
    #    in the live tree that every copy then carries is q-69's, which slice-43 settles: a
    #    copy leaves out what git ignores (MUT-325). The scenario therefore runs with
    #    --no-coverage. A test that writes into a
    #    linked node_modules writes into the project's, as today.
    # Tests besides the scenario: an adapter test in worktrees that holds it on every OS's go
    # test with no tools. It reuses worker_test.go's approach of running the test binary
    # itself as the command (TestCommandRunnerFixtureHelper, a new mode): the helper exits 1
    # when left-behind exists, when kept.txt is missing or when source.txt is not original\n,
    # then writes left-behind, removes kept.txt and rewrites source.txt. A baseline then two
    # mutants on one worker must all pass. The test must also cover a left-out folder (build/
    # beside a package.json) and a link, which keep what the run wrote, and the restore's time
    # staying out of Elapsed. Today it fails at the second run. The shared WorkersContract
    # (domain/porttest) gets no clause, because the fake runs nothing and holds no copy.
    # Owns: internal/slices/mutation/adapters/worktrees/ (worker.go, workers.go, a new file
    # for the record and restore, their tests), the Worker doc in domain/workers.go, the
    # scenario rows below in features/mutate.feature, features/mutate_pristine.go (new), one
    # registration line in features/mutate.go, and README's "Parallel workers" bullet.
    # Must not touch: internal/processes (bug-5's), worktrees/dart.go's rules (slice-35's;
    # read dartLeftOut and pubWorkspace as they are), coverage/, the fake Workers in
    # domain/porttest, and features/steps.go (the existing Tests declaration with language go
    # is enough).
    # slice-13, issue #35: a worker made its copy of the project once a run
    # and kept it for its baseline and every mutant, putting back only the
    # mutated file, so whatever one run of the tests wrote, rewrote or
    # removed there was still so for every later run of that worker. One
    # run that left a failing state behind (rapid's failfile, in issue #35)
    # killed every later mutant of the worker, whatever it changed, and
    # mutation sample reported the kills as mismatches. Decided by the
    # coordinator on 2026-10-21: each run in a worker, the baseline's, a
    # selection's and each mutant's, starts from the copy as it was made.
    # What the copy leaves out, build output, caches and .dart_tool, and
    # the dependency folders it links stay as the runs leave them.
    # --no-coverage keeps every run in the copy: a coverage run, in the
    # project's own folder, leaving a file there is q-69's, which slice-43
    # settles (MUT-325). --workers 1 has one copy hold every run,
    # and the test's own check decides each outcome, so no timing does. No
    # test checks a result, so every mutant survives. Before slice-13 the
    # baseline passed and left its mark, and every mutant was killed, exit
    # 0, failing at the first Then. Binary-only for good: the copy, and what
    # a test run leaves in it, are the real Workers'; the fake runs each
    # mutant as declared and holds no copy. The adapter's own test holds the
    # same on every system's go test.
    @slice-13 @ID-MUT-267 @binary-only
    Scenario Outline: What a test run <changes> is back as copied for the next mutant
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        func B(x int) bool { return x > 1 }
        func C(x int) bool { return x > 2 }
        """
      And testdata/ is ignored by git and holds golden.txt, "committed", and keep.txt, both committed
      And its test, which checks no result, fails when <found>, and then <changes>
      When I run "itos-cc mutation run --no-coverage --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1
      And the project's own testdata folder is as committed

      Examples:
        | changes                       | found                                |
        | writes testdata/left-behind   | testdata/left-behind is there        |
        | rewrites testdata/golden.txt  | testdata/golden.txt is not committed |
        | removes testdata/keep.txt     | testdata/keep.txt is gone            |

    # slice-43, q-69 and q-92 (the person, 2026-10-24): a worker's copy
    # carried every file of the tree but the folders it skips or links,
    # ignored ones too, so a file a coverage run left in the project (it
    # runs in the project's own folder) reached every copy and failed its
    # baseline, and stray build output and tools went into every copy (80
    # MB a worker in itos-cc's own tree). Decided:
    # 1. Ignored is what the committed .gitignore files ignore and git does
    #    not track: git ls-files -z --others --ignored
    #    --exclude-per-directory=.gitignore --directory, once per tree when a
    #    pool opens, so copies are the same on every machine (no
    #    info/exclude, no global excludes). A tracked file in an ignored
    #    folder stays in the copy (MUT-267's keep.txt).
    # 2. mutation.copy in itos-cc.yaml lists doublestar globs from the
    #    project root, checked as exclude's are and refused as
    #    config.invalid keyed mutation.copy[i], of paths a copy would leave
    #    out, ignored or build output (a TypeScript dist/ beside its
    #    package.json, left out before this slice too), or an untracked
    #    nested checkout (decision 10), whose files it brings, which it
    #    copies after the links. Never .git, .dart_tool or a linked folder.
    # 3. A glob matching a folder brings all it holds: gen and gen/** alike.
    # 4. The paths it brings are recorded and restored before each run like
    #    any copied file (slice-13), a path inside a build-output folder
    #    included.
    # 5. The linked folders (node_modules, .venv, venv, vendor) stay linked.
    # 6. A tree in no git repository, or where git fails, is copied as
    #    before this slice, with no note.
    # 7. A baseline that passes in the project but fails in the copy (MUT-284)
    #    names the ignored paths and the nested checkouts the copy left out,
    #    folded at folders, the first five then "and N more", and its fix
    #    names mutation.copy with the first path as its example. --json
    #    keeps its fields.
    # 8. Generated Dart stays in the copy even when ignored, recognised as
    #    discovery recognises it, keeping q-77's promise; an adapter test
    #    holds it, as the scenarios need no Flutter.
    # 9. Counted runs are unchanged: their frozen copy already holds the
    #    commit's tracked files alone. The pool asks git nothing for them.
    # 10. A folder holding its own .git, a folder or a file as a linked
    #    worktree has, that git does not track is another repository and
    #    is never copied, so a nested checkout such as .claude/worktrees/
    #    never reaches a copy (git ls-files --others lists it as one entry;
    #    itos's evidence records refuse nested checkouts for the same
    #    reason). A submodule, which the index tracks as a gitlink, is
    #    copied with its files, as before this slice, never its .git
    #    (coordinator's correction, 2026-10-24: decision 10 as first written
    #    dropped submodules too, whose folder holds a .git file).
    # It changes what a copy holds, so the feat carries an Upgrading
    # footer: a project whose tests read an ignored file names it in
    # mutation.copy. Binary-only for good but the refusal: copies are the
    # real Workers'. An adapter test in worktrees holds decisions 1, 3, 4,
    # 6, 8 and 10 on every system's go test. Before slice-43, MUT-325's
    # first row's baseline failed in the copy, MUT-326's copy held
    # tmp/stray.log, MUT-328 reported no ignored path, MUT-330 copied
    # nested/ and MUT-331 refused nothing; MUT-325's second row, MUT-327,
    # MUT-329 and MUT-332 are guardrails that pass today.
    @slice-43 @ID-MUT-325 @binary-only
    Scenario Outline: A file the coverage run leaves in the project never reaches a worker's copy
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        func B(x int) bool { return x > 1 }
        func C(x int) bool { return x > 2 }
        """
      And testdata/ is ignored by git and holds golden.txt, "committed", and keep.txt, both committed
      And its test, which checks no result, fails when <found>, and then writes testdata/left-behind
      When I run "itos-cc mutation run --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1

      Examples:
        | found                         |
        | testdata/left-behind is there |
        | testdata/keep.txt is gone     |

    @slice-43 @ID-MUT-326 @binary-only
    Scenario: A worker's copy still links node_modules and leaves out a stray ignored file
      Given a TypeScript package, tested with Vitest, whose .gitignore ignores node_modules/ and tmp/, with tmp/stray.log, and whose tests pass in the project
      When I run "itos-cc mutation run --workers 1 --json" for its source
      Then its baseline passed, and every mutant of it was judged
      And the worker's copy held node_modules as a link to the project's, and no tmp/stray.log

    @slice-43 @ID-MUT-327 @binary-only
    Scenario Outline: An ignored path itos-cc.yaml names is copied
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        """
      And gen/ is ignored by git and holds table.txt, which the project generated and never committed
      And its test, which checks no result, fails when gen/table.txt is missing
      And itos-cc.yaml holds '<yaml>'
      When I run "itos-cc mutation run --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1

      Examples:
        | yaml                       |
        | mutation: {copy: [gen/**]} |
        | mutation: {copy: [gen]}    |

    @slice-43 @ID-MUT-328 @binary-only
    Scenario: A baseline that fails for a missing ignored file names it and the key
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        """
      And gen/ is ignored by git and holds table.txt, which the project generated and never committed
      And its test, which checks no result, fails when gen/table.txt is missing
      When I run "itos-cc mutation run --workers 1 --json leak.go"
      Then the problem is "mutation.baseline-failed", its message naming gen/ as ignored by git and left out of the worker's copy, and its fix naming mutation.copy in itos-cc.yaml
      And the exit code is 1

    @slice-43 @ID-MUT-329 @binary-only
    Scenario: A project in no git repository is copied whole, as before
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        """
      And the module is in no git repository and holds gen/table.txt
      And its test, which checks no result, fails when gen/table.txt is missing
      When I run "itos-cc mutation run --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1

    @slice-43 @ID-MUT-330 @binary-only
    Scenario: A nested checkout never reaches a worker's copy
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        """
      And nested/ is a git repository of its own, holding marker.txt, which git in the module neither tracks nor ignores
      And its test, which checks no result, fails when nested/marker.txt is there
      When I run "itos-cc mutation run --no-coverage --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1

    @slice-43 @ID-MUT-332 @binary-only
    Scenario: A git submodule's files reach a worker's copy
      Given the Go module "example.com/leak" with the file "leak.go":
        """
        package leak

        func A(x int) bool { return x > 0 }
        """
      And sub/ is a git submodule of the module, holding data.txt
      And its test, which checks no result, fails when sub/data.txt is missing
      When I run "itos-cc mutation run --no-coverage --workers 1 --no-annotate --json leak.go"
      Then leak.go's baseline passed and every mutant of leak.go survived
      And the exit code is 1

    @slice-43 @ID-MUT-331
    Scenario Outline: A mutation.copy glob that is no pattern is a config error
      Given itos-cc.yaml holds '<yaml>'
      When I run "itos-cc <command>"
      Then the problem is "config.invalid", with key "<key>"
      And the exit code is 80

      Examples:
        | yaml                                  | command      | key              |
        | mutation: {copy: ["gen/[x"]}          | mutation run | mutation.copy[0] |
        | mutation: {copy: [gen/**, "../x/**"]} | mutation run | mutation.copy[1] |
        | mutation: {copy: ["/abs/**"]}         | crap         | mutation.copy[0] |
        | mutation: {copy: [""]}                | mutation run | mutation.copy[0] |

    # slice-19, issue #44: a worker copied the package's own root alone,
    # the nearest package.json, with node_modules linked. Whatever the
    # package read above its folder was missing in the copy. In a pnpm
    # workspace that is common: a tsconfig.json that extends the
    # workspace's ../../tsconfig.base.json. Vite could not load it in the
    # copy, every test failed with TSCONFIG_ERROR, and every baseline
    # failed, so no mutant was judged. The same tests passed in the tree,
    # as itos-cc's own coverage run there showed. Decided in the item:
    # when the package belongs to a workspace, a worker copies the
    # workspace's root, its dependency folders linked, and runs in the
    # package's place in that copy, as a pub workspace member's copy holds
    # its workspace (MUT-265); the cost of copying every member once per
    # worker is accepted. Coordinator's calls, 2026-10-21: a workspace is a
    # pnpm-workspace.yaml with a packages key or a package.json with a
    # workspaces field (npm and yarn), above the package and within the
    # project root, whose globs match it; a workspace root above the
    # project root is not copied. The whole root is copied, less what a
    # copy always leaves out, and two members of one workspace share one
    # copy per worker. A tsconfig extends chain outside any workspace is
    # not followed: MUT-284's message covers it. The workspace reader is
    # the worker's own, not slice-18's, so the two items stay apart.
    # Binary-only for good: the copy and Vite's reading of the tsconfig
    # are the real Workers' and the real Vitest's, and the fake Workers
    # hold no copy (as MUT-267). Before slice-19 every baseline fails in
    # the copy with TSCONFIG_ERROR, failing at the first Then.
    @slice-19 @ID-MUT-283 @binary-only
    Scenario Outline: A member of a <workspace> whose tsconfig extends one at the workspace root is mutated in a copy of its workspace
      Given a <workspace> whose root holds tsconfig.base.json, with the member packages/shapes, tested with Vitest, whose tsconfig.json extends ../../tsconfig.base.json and whose tests pass in the project
      When I run "itos-cc mutation run --workers 1 --json packages/shapes/src/area.ts"
      Then packages/shapes/src/area.ts's baseline passed, and every mutant of it was judged
      And the exit code is 0

      Examples:
        | workspace                                                |
        | pnpm workspace listing packages/* in pnpm-workspace.yaml |
        | npm workspace listing packages/* in its package.json     |

    # slice-19, issue #44: when a copy still cannot work, a baseline that
    # fails in the copy but passes in the project said "Make its tests
    # pass", which points at the wrong fix. Decided in the item: say that
    # the tests fail only in the worker's copy when they pass in the tree.
    # Coordinator's calls, 2026-10-21: after a file's baseline fails in a
    # copy, itos-cc runs the same command once, unmutated, in the
    # project's own folder, as coverage runs there; when it passes there,
    # the problem stays "mutation.baseline-failed" and the exit code stays
    # 1, the message saying its tests pass in the project but fail in the
    # worker's copy of <root>, and the fix that the copy holds <root> alone
    # and its tests read something outside it. When they fail there too,
    # the message is as before. mutation sample says the same. The fixture
    # is a package in no workspace whose tsconfig.json extends the folder
    # above it, which slice-19 still does not copy. Binary-only for good,
    # as MUT-283. Before slice-19 the message says only that its tests
    # fail, failing at the Then; the But holds today.
    @slice-19 @ID-MUT-284 @binary-only
    Scenario: A baseline that fails only in the worker's copy says so
      Given a TypeScript package in no workspace, tested with Vitest, whose tsconfig.json extends ../tsconfig.base.json in the folder above it, and whose tests pass in the project
      When I run "itos-cc mutation run --json" for its source
      Then the problem is "mutation.baseline-failed", its message saying the tests pass in the project but fail in the worker's copy of the package
      And the exit code is 1
      But when its tests fail in the project too, the message is "its tests fail before any mutant, so none was judged", as before

  # slice-24, issues #54 and #47. A mutant whose test loops writing output
  # filled unbounded buffers: the worker's (worktrees/worker.go runIn) and
  # the supervisor's (processes/proc_owned.go readOwnedCommandOutput). A
  # test that kept the output in memory grew until the machine died. On a
  # GitHub-hosted runner that took the runner down before the mutant's
  # timeout fired: the job ended with a shutdown signal, a counted run
  # reported count.interrupted, the mutants in flight were lost and every
  # site after them went unjudged (#54). A gate can never pass a range
  # holding such a mutant.
  # Decided in the item: a mutant's output and memory are both capped by
  # default. Each cap is sized from the baseline as the timeout is
  # (q-23): the baseline's output size or peak memory times a factor, plus
  # a fixed allowance, so a cap never sits below what the unmutated tests
  # need. Where a platform cannot measure memory there is no memory
  # cap, rather than a guessed one. Going over a cap kills the
  # mutant, as a timeout does: a runaway is the suite catching the mutant,
  # as PIT and Stryker count it. Its outcome is its own, over-limit,
  # beside timeout, in the snapshot and --json, so such kills can be listed
  # and reviewed.
  # Decided by the person on 2026-10-21: the defaults, flags only (no
  # itos-cc.yaml key) and no memory cap on macOS and Windows for now, said
  # once on stderr, with ideas for a Windows Job Object and a macOS reader;
  # --json does not list which caps held. Coordinator's calls, 2026-10-21,
  # for the rest, memory read as the group's summed RSS every 50 ms, not
  # an rlimit (RLIMIT_AS breaks Go and the JVM, and caps each process, not
  # the group):
  # - The output cap is the bytes the baseline wrote, stdout and stderr
  #   together, times --output-factor, 10 by default, plus 16 MiB. The
  #   memory cap is the baseline's peak memory times --memory-factor, 4 by
  #   default, plus 256 MiB. Both flags sit beside --timeout-factor on
  #   mutation run (counted runs included) and mutation sample; no
  #   itos-cc.yaml key, as the timeout has none.
  # - Memory is the resident memory of the command's whole owned process
  #   group, summed over its members, sampled while it runs; its peak is
  #   the baseline's. Linux measures it from /proc, as the supervisor
  #   already reads the group there. macOS and Windows measure nothing yet,
  #   so they cap no memory, and the run says so once on stderr.
  # - Each cap comes from the baseline whose time sets the timeout: the
  #   file's own, a selection of listed tests' (MUT-135), a sample's or a
  #   counted trial's. A baseline has no cap, as it has no timeout.
  #  
  # - Over a cap, the group is killed and joined as at a mutant's own
  #   timeout (ADR-0021): a local cleanup budget, never the shared one. The
  #   outcome is "over-limit", with "limit" "output" or "memory" in --json
  #   and the snapshot (a key added, left out for every other outcome, so
  #   older snapshots read and write as before). Its progress line says
  #   which cap and its size.
  # - Everywhere a timeout counts as killed, over-limit does: the file's
  #   and the unit's killed counts, mutation check, the reuse of a kill in
  #   an unchanged function, sample's agreement (MUT-74), an exception gone
  #   stale as killed, and --fail-fast, where a kill is not actionable.
  #  
  # Both drivers run MUT-300 and MUT-302. MUT-301 too, its outcome
  # differing by system: the platform is a value the Workers binding holds,
  # and the fake measures memory where the real supervisor does.
  Rule: A mutant's output and memory are capped, sized from its baseline

    # The flooding fixture writes in 64 KiB pieces with a millisecond's
    # pause, about 64 MB a second, so today's product, which buffers it
    # all until the timeout, stays under half a gigabyte on CI.
    # --timeout-factor 1 keeps that red run short; every judged mutant's
    # tests end in under a second. The fixture sets GOFLAGS=-v: without
    # -v, go test holds a package's test output until its test ends, so
    # there the memory cap, not the output cap, stops such a test.
    # Before slice-24 the endless printer runs to its timeout, outcome
    # "timeout", failing at the first Then.
    @slice-24 @ID-MUT-300
    Scenario: A mutant whose tests write output without end is killed over its output cap
      Given a Go package whose test prints 2 MiB with nothing mutated, where one mutant makes it print 30 MiB and pass, and another makes it print without end
      When I run "itos-cc mutation run --no-coverage --workers 2 --timeout-factor 1 --json" for its file
      Then the endless printer's outcome is "over-limit", with "limit" "output", and it is counted in "killed"
      And it ended before its timeout, and every other mutant was judged as its tests decide
      And the 30 MiB printer survived, under ten times the baseline's 2 MiB plus 16 MiB
      And the exit code is 1
      But with "--output-factor 1" the 30 MiB printer is "over-limit" too, and the exit code is 0

    # The hoarding fixture grows by 64 MiB every 20 ms up to 2 GiB, then
    # fails its test, so no system holds more for long, and on a system
    # with no memory cap the mutant is killed by its own test.
    # --memory-factor 1 keeps the cap near the baseline's peak plus
    # 256 MiB, well under 2 GiB, whatever the toolchain's own peak.
    # Before slice-24 the run refuses --memory-factor as flags.unknown,
    # exit 80, failing at the first Then on every system; without the flag
    # the hoarder was "killed" by its own test, and nothing was said.
    @slice-24 @ID-MUT-301
    Scenario: A mutant whose tests hold ever more memory is killed over its memory cap where memory is measured
      Given a Go package whose test holds little memory with nothing mutated, where one mutant makes it hold ever more, up to 2 GiB, before it fails
      When I run "itos-cc mutation run --no-coverage --workers 1 --memory-factor 1 --json" for its file
      Then on Linux the hoarder's outcome is "over-limit", with "limit" "memory", before it holds 2 GiB
      And every other mutant was judged as its tests decide
      But on macOS and Windows its outcome is "killed", and stderr says once that no memory cap holds on this system, as itos-cc cannot measure a test run's memory there

    # The snapshot is written as raw JSON (the lessons in AGENTS.md): a
    # run that timed the mutant out records it, and the fixture then turns
    # its outcome into "over-limit" with "limit" "output".
    # Before slice-24 run does not reuse an outcome it does not know, so
    # it runs the mutant again and records "timeout", failing at the Then.
    @slice-24 @ID-MUT-302
    Scenario: A recorded over-limit kill is a kill to run, check and sample
      Given the snapshot of src/board.go records the mutant at line 7 as "over-limit", with "limit" "output", in a function unchanged since
      When I run "itos-cc mutation run --json" for its file
      Then that mutant is "over-limit", with "limit" "output", reused, and counted in "killed"
      And "itos-cc mutation check src/board.go" counts it killed and exits 0
      And when it now runs past its timeout, "itos-cc mutation sample --count 20 src/board.go" reports no problem and exits 0

  Rule: Coverage decides which mutants run

    @ID-MUT-18
    Scenario: Mutants on lines no test executes are not run
      Given coverage shows line 12 of src/board.ts is never executed
      When I run "itos-cc mutation run src/board.ts"
      Then the mutants on line 12 are reported as uncovered without running

    @ID-MUT-19
    Scenario: A file the tests never load is entirely uncovered
      Given coverage measured other TypeScript files but never src/unused.ts
      When I run "itos-cc mutation run src/unused.ts"
      Then every mutant in src/unused.ts is uncovered

    # A language counted as measured only from the files of the run, and
    # coverage drops report entries that match no source, so a file no test
    # loads, mutated alone (or as the only file --since selects), printed
    # "no coverage …; running every mutant" though coverage ran for its
    # language; under --fail-uncovered its mutants then failed as survivors.
    # Seen while building slice-2, not yet reproduced with a TypeScript or
    # LCOV fixture: reproduce it first. A coverage command that ran for the
    # file's language measures that language, even when its report names no
    # file of the run (the coordinator's call, 2026-10-07). Go fixtures do
    # not show it, since go test reports a package without tests at 0%.
    @unloaded-file-uncovered @ID-MUT-105
    Scenario: A file no test loads, mutated alone, is entirely uncovered
      Given a TypeScript project whose tests import src/board.ts but never src/unused.ts
      When I run "itos-cc mutation run src/unused.ts"
      Then every mutant in src/unused.ts is uncovered, and none runs
      And stderr does not say "no coverage"
      And "itos-cc mutation run --fail-uncovered src/unused.ts" lists them as uncovered and exits 1

    # Binary-only for good: Gradle and Maven are left off the real PATH.
    @ID-MUT-20 @binary-only
    Scenario: No coverage for the language at all
      Given coverage measured no Kotlin file
      When I run "itos-cc mutation run src/Board.kt"
      Then stderr says "itos-cc: no coverage for src/Board.kt; running every mutant"

    @ID-MUT-21
    Scenario: Skipping coverage
      When I run "itos-cc mutation run --no-coverage"
      Then every mutant runs regardless of coverage

    @ID-MUT-22
    Scenario: Coverage is only measured when a mutant has to run
      Given every mutant can be reused from the previous snapshot
      When I run "itos-cc mutation run"
      Then no coverage or test command runs
      And stderr says "itos-cc: every mutant reused from .metrics/mutate, none to run"

    # slice-48, issue #49 (2026-10-24, a coordinator's call inside the
    # item): a run whose every mutant was reused printed "itos-cc: no
    # mutations to test" above per-file summaries listing them, which reads
    # as an empty selection. It now says "itos-cc: every mutant reused from
    # .metrics/mutate, none to run". A selection with no mutant at all keeps
    # "no mutations to test". The feat changes MUT-22's line to the new
    # wording, saying so. Before this item the old line was printed, failing
    # at the stderr step.
    @slice-48 @ID-MUT-336
    Scenario: A run whose every mutant is reused says so
      Given every mutant can be reused from the previous snapshot
      When I run "itos-cc mutation run"
      Then stderr says "itos-cc: every mutant reused from .metrics/mutate, none to run"
      And stderr does not say "no mutations to test"

    # The lines the mutation commands write to stderr themselves (the
    # baseline, coverage, each mutant's outcome) began "mutate:", a command
    # that is gone since the mutation group. They begin "itos-cc:", as every
    # other line itos-cc writes to stderr does (the coordinator's call,
    # 2026-10-07). The output of the test and coverage commands themselves is
    # passed on as it is. ID-MUT-20 and ID-MUT-22 said the old text and changed
    # with the fix.
    @mutation-progress-prefix @ID-MUT-100
    Scenario: Progress lines begin itos-cc:
      Given src/board.ts has a mutant that is killed and one on a line no test executes
      When I run "itos-cc mutation run src/board.ts"
      Then the lines itos-cc writes to stderr about the baseline and each mutant begin "itos-cc: "
      And no line of stderr begins "mutate: "

  # Issue #14: Go records coverage only for code that runs inside the test
  # process, so a project tested end to end, by tests that run its built
  # binary, had every line those tests reach uncovered, and --fail-uncovered
  # failed nearly everything (itos, whose specification runs the built itos,
  # is that project). Go has had integration coverage since 1.20: a binary
  # built with go build -cover writes its coverage into the directory
  # GOCOVERDIR names, and go tool covdata textfmt turns it into an ordinary
  # profile. Decided with the person on 2026-10-07 (the issue's part 1; its
  # part 2, coverage per test, stays the idea test-attribution):
  # - When itos-cc runs a Go coverage command, it sets GOCOVERDIR to a fresh
  #   directory of the run's own under .metrics/coverage/, and after the run
  #   merges whatever data was written there into the coverage, beside go
  #   test's profile. A project opts in by having its test harness build the
  #   binary under test with -cover when GOCOVERDIR is set; itos-cc knows
  #   nothing of the harness. A project that writes nothing there behaves as
  #   before.
  # - Which tests run is unchanged: by default a file's own tests
  #   (ID-MUT-08), so an end-to-end test in another package counts under
  #   --all-tests, as its kills already do (ID-MUT-09).
  # - --json says which coverage reached each covered mutant: "coverage",
  #   the sources whose data covers its line, "in-process", "integration"
  #   or both (the coordinator's call: per mutant, the finest the issue
  #   allows, so a gate can tell).
  # - Coverage is shared, so crap's coverage counts integration data too.
  # - Found while building it: go test -cover sets each test binary's
  #   GOCOVERDIR to a directory of its own, and reads only the test binary's
  #   own data there, so a binary a test builds wrote where nothing read it.
  #   itos-cc runs each test binary itself (go test -exec), with GOCOVERDIR
  #   the run's directory; go test's own profile is unchanged.
  #
  # Binary-only for good: the coverage of a process a test starts is the
  # real binary's, written by the real go build -cover.
  Rule: Coverage from tests that run the built binary

    @integration-coverage @ID-MUT-117 @binary-only
    Scenario: Lines a test reaches through the built binary are covered
      Given a Go module whose only test builds its binary with go build -cover when GOCOVERDIR is set, and runs it
      And the test checks one branch of the binary's output and not another
      When I run "itos-cc mutation run --all-tests --fail-uncovered"
      Then the lines the binary ran are covered, and their mutants run
      And a mutant the test notices is killed, and one it does not notice survives
      And only the mutants on lines the binary never ran are uncovered

    @integration-coverage @ID-MUT-118 @binary-only
    Scenario: A project whose binary writes no coverage behaves as before
      Given a Go module whose only test builds its binary without -cover and runs it
      When I run "itos-cc mutation run --all-tests --fail-uncovered"
      Then every mutant the binary's code holds is uncovered, as before
      And the exit code is 1

    @integration-coverage @ID-MUT-119 @binary-only
    Scenario: Which coverage reached each mutant, as JSON
      Given the module of ID-MUT-117, with one function also called by an in-process test
      When I run "itos-cc mutation run --all-tests --json"
      Then each covered mutant has "coverage", listing "in-process", "integration" or both, as the data that covers its line
      And an uncovered mutant has no "coverage"

    @integration-coverage @ID-MUT-120 @binary-only
    Scenario: The binary's coverage data stays with the run
      When the coverage of ID-MUT-117 runs
      Then GOCOVERDIR names a directory under the run's own run-* directory in .metrics/coverage/
      And nothing of it is left once the run ends

  # Issue #14, part 2: a project tested end to end (itos: about 505 godog
  # scenarios, one go test run of about 3 minutes) cannot afford a whole
  # suite per mutant (--all-tests). Coverage per test lets each mutant run
  # only the tests that reach its line. Decided with the person on
  # 2026-10-07 (q-17, q-18, q-20); freshness of such kills (q-19) is the
  # next slice, listed-tests-freshness.
  # - itos-cc.yaml names the tests under mutation.tests, as itos's adapters
  #   do, sharing only IDs and paths: list, a command printing one test per
  #   line, its ID and, after a tab, optionally the file that defines it;
  #   run, a command with {pattern}; ids_pattern, with {ids}; and join, with
  #   each ({id}) and sep. Optional whole, a command running every listed
  #   test, else run with every ID. Without mutation.tests nothing changes.
  # - Per-test coverage takes one run: itos-cc sets ITOS_CC_TEST_COVERDIR to
  #   a directory of the run's own, and a harness that knows it gives the
  #   processes each test starts GOCOVERDIR=<dir>/<test-id>. When nothing is
  #   written there, itos-cc runs the run command once per test, each with
  #   GOCOVERDIR of its own, as integration-coverage passes it (the
  #   coordinator's names: the variable and the per-test fallback).
  # - A mutant runs its file's own tests first; only if it survives, the
  #   listed tests whose coverage reaches its line, as one run. It is
  #   uncovered only when neither covers its line.
  # - An outcome a listed run decided records scope "listed" (q-15) and the
  #   IDs of the tests that cover its line; mutation sample re-runs it with
  #   those tests. --all-tests still runs the whole suite instead.
  # - A list command that fails is "tests.list-failed", exit 1, and nothing
  #   is judged (the coordinator's call, as a failing baseline is).
  Rule: Mutants run only the listed tests that reach them

    @listed-tests @ID-MUT-121
    Scenario: Listed tests come from itos-cc.yaml
      Given itos-cc.yaml names mutation.tests with list, run, ids_pattern and join
      And the list command prints "ID-A-01<tab>a.feature" and "ID-A-02"
      When I run "itos-cc mutation run"
      Then the list command runs once
      And the listed tests are ID-A-01, defined in a.feature, and ID-A-02, with no file

    @listed-tests @ID-MUT-122
    Scenario: Per-test coverage takes one run when the harness splits it
      Given a Go module whose harness, when ITOS_CC_TEST_COVERDIR is set, gives each test's binary GOCOVERDIR=<dir>/<test-id>
      And its tests ID-A-01 and ID-A-02 reach different lines of the binary
      When I run "itos-cc mutation run"
      Then the listed tests run once for coverage
      And each line the binary ran is covered by the IDs of the tests that reached it

    @listed-tests @ID-MUT-123
    Scenario: Without the harness's help, coverage takes one run per test
      Given the module of ID-MUT-122, whose harness ignores ITOS_CC_TEST_COVERDIR
      When I run "itos-cc mutation run"
      Then the run command runs once per listed test for coverage, each selecting that test alone
      And each line is covered by the same IDs as in ID-MUT-122

    @listed-tests @ID-MUT-124
    Scenario: A mutant runs its own tests first, and the covering tests only if it survives
      Given a mutant its file's own tests kill, and one only ID-A-02 kills
      When I run "itos-cc mutation run"
      Then the first is killed without any listed test running for it
      And the second runs its own tests, survives them, then runs the run command selecting ID-A-02 alone, and is killed

    @listed-tests @ID-MUT-125
    Scenario: Uncovered means neither the own tests nor a listed test reach the line
      Given a line only ID-A-01 reaches, and a line no test reaches
      When I run "itos-cc mutation run --fail-uncovered"
      Then the first line's mutants run and are not uncovered
      And only the second line's mutants are uncovered

    @listed-tests @ID-MUT-126
    Scenario: An outcome decided by listed tests records them
      Given a mutant only ID-A-02 kills
      When I run "itos-cc mutation run --json"
      Then its snapshot entry records scope "listed" and tests ["ID-A-02"]
      And its mutant in --json has "tests": ["ID-A-02"]

    @listed-tests @ID-MUT-127
    Scenario: mutation sample re-runs a listed outcome with its tests
      Given a fresh outcome recorded with scope "listed" and tests ["ID-A-02"]
      When I run "itos-cc mutation sample --count 100"
      Then that mutant runs the run command selecting ID-A-02, after its own tests survive it
      And no "mutation.mismatch" is reported

    @listed-tests @ID-MUT-128
    Scenario: Without mutation.tests nothing changes
      Given itos-cc.yaml has no mutation.tests
      When I run "itos-cc mutation run"
      Then no list command runs, and ITOS_CC_TEST_COVERDIR is not set
      And the outcomes and the snapshot are as before

    @listed-tests @ID-MUT-129
    Scenario: A list command that fails judges nothing
      Given the list command exits 1
      When I run "itos-cc mutation run"
      Then the problem is "tests.list-failed", with its exit code
      And no mutant runs, no snapshot is written, and the exit code is 1

    # slice-15 decisions (coordinator, 2026-10-21: the spec drafter's
    # recommendations, taken as routine calls inside the issue's scope):
    # 1. The rule. tests.coverage-failed: the listed tests' coverage run failed without any
    #    mutant. Two cases count: - the run of every listed test (whole, else run with every
    #    ID) exits non-zero; - in the per-test fallback, one test's run alone exits non-zero.
    # Nothing is judged, no mutant runs, no snapshot is written and no .metrics/coverage cache
    # is kept, as with a cancelled measurement. This holds with and without --fail-uncovered,
    # and with --fail-fast. The run reports it as its only problem; coverage.cleanup-failed
    # for a coverage command's owned group, and interrupted, still stand beside it.
    # 2. Exit code 1. It is the "a check said no" class of docs/CLI.md: the project's own
    #    tests fail, as mutation.baseline-failed, tests.list-failed and tests.selection-failed
    #    do, none of which is a requirement of the environment (82) or a usage error (80). By
    #    the precedence order (130, 80 and 81, 100, 82, 75, 1) it yields to everything beside
    #    it: an interrupt that stops the coverage run is interrupted (130), never this
    #    problem, since the context's cause is checked first, as loadCoverage already does. A
    #    coverage command whose owned group's cleanup did not finish is coverage.cleanup-
    #    failed (100).
    # 3. Subject keys (q-D): command, the shell line that failed as itos-cc ran it (the
    #    variables it set are not part of it; in the fallback the line selecting that one
    #    test), and exit_code, its exit code, -1 when it gave none (a signal, or no shell to
    #    start). These are the same keys as tests.list-failed.
    # 4. Wording. - message: the listed tests of mutation.tests in itos-cc.yaml failed their
    #    coverage run without any mutant (<exit status>), so no mutant was judged: <command> -
    #    fix: Make the listed tests pass when they run with ITOS_CC_TEST_COVERDIR set and
    #    GOCOVERDIR unset, then run again.
    # 5. --json shape, beside tests.list-failed's, both "ok": false with no "files":
    # (the --json shapes beside tests.list-failed: rule, exit, message, fix, command,
    # exit_code; "ok": false, no "files")
    # (the --json shapes beside tests.list-failed: rule, exit, message, fix, command,
    # exit_code; "ok": false, no "files")
    # The agent confirms the exact command text by running the scenario. The fixture's run
    # joins IDs as its itos-cc.yaml says.
    # 6. Boundary. The measurer reports the failure as a typed failure of the coverage port, a
    #    new port.Failure variant carrying the command line and exit code, from MeasureListed,
    #    and gives no report. The mutation domain turns it into a failure of its own, as
    #    ListingFailed is (domain/run_coverage.go:28-35), in strict and non-strict measurement
    #    alike. The non-strict branch that reports coverage-failed and goes on (110-119) must
    #    not swallow it. The slice's runFailure (internal/slices/mutation/errors.go:171-183)
    #    words it, so the strict path (loadCoverage, which today returns the port failure
    #    unwrapped) reaches the same problem. The domain carries no rule, wording or exit
    #    code.
    # 7. The fake (internal/coverage/porttest/measurer.go, MeasureListed and listed, 91-163
    #    and 306-386) returns the same failure when a declaration's listed collection fails,
    #    with the command it logged and a declared exit code (default 1). It also does so when
    #    the harness does not split and a test in Failing runs alone. Today it attaches nil.
    #    The listed contract holds the real measurer, with a harness that fails, and the fake
    #    to it. ListedTests.CoverageExit (features/steps.go:212) already reaches the fake as
    #    ListedFailed (cmd/itos-cc/scenarios_fakes_test.go:149) and carries the exit code
    #    through.
    # 8. Out of scope (q-F). A go tool covdata conversion that fails, and a listed test that
    #    writes no per-ID data, keep today's handling (logged; that test covers nothing). They
    #    are not the tests failing. Counted runs already stop at this stage as
    #    count.preparation-failed, stage listed-coverage
    #    (features/mutate_counted_preparation.go), and mutation sample measures no coverage.
    #    Neither changes.
    # 9. Help and docs. mutation run's rules list gains tests.coverage-failed   the listed
    #    tests' coverage run failed without any mutant: command, exit_code
    #    (run_contract.go:403-404). Its listed-tests paragraph (322-345) says the coverage run
    #    that fails is tests.coverage-failed, nothing judged and no snapshot written. As the
    #    issue asks, it also says that the run of every test sets ITOS_CC_TEST_COVERDIR
    #    without GOCOVERDIR: a harness builds its binary with -cover only when it gives the
    #    processes GOCOVERDIR, or gives each test's processes GOCOVERDIR=<dir>/<test ID>
    #    itself. Otherwise a -cover binary warns on stderr that it wrote no data. pertest.go's
    #    Go collector sets no whole variable, which confirms this. docs/CLI.md gains the row
    #    tests.coverage-failed | 1 | mutation run | command, exit_code, and exit code 1's
    #    meaning gains "the listed tests' coverage run that fails". README's listed-tests
    #    paragraph (about line 447) says it beside tests.selection-failed. The help golden is
    #    updated if it holds the text.
    # Owns: see the table above. Must not touch: coverage/supervised.go's
    # MeasureTestsSupervised (counted runs' fail-closed path), the counted run's stages,
    # coverage/run.go and the per-language plans (slice-16's), and features/coverage.feature.
    # slice-15, issue #15: the listed tests' coverage run, every listed test
    # run once with ITOS_CC_TEST_COVERDIR set, failed without any mutant,
    # and itos-cc logged one line and went on with no listed coverage: a
    # line only listed tests reach was uncovered, so --fail-uncovered asked
    # for tests of code the suite covers, and --json never named the cause.
    # Decided by the coordinator on 2026-10-21: as a list command that fails
    # is tests.list-failed, such a run is "tests.coverage-failed", exit 1,
    # with the command and its exit code, and nothing is judged and no
    # snapshot written, with or without --fail-uncovered. Both drivers run
    # it: the fake measurer fails the listed run as declared
    # (ListedTests.CoverageExit), and the binary driver's harness fails its
    # run of every test when ITOS_CC_TEST_COVERDIR is set and the
    # scenario's variable says so, as GREET_FAIL_ALONE fails a selection.
    # Before slice-15 each row judged its mutants: with --fail-uncovered the
    # lines only listed tests reach were mutation.uncovered, and without it
    # their mutants survived their own tests, so each fails at the problem's
    # step.
    @slice-15 @ID-MUT-268
    Scenario Outline: A coverage run of the listed tests that fails judges nothing
      Given the listed tests fail when they all run for coverage, though they pass together when a mutant's run selects them
      When I run "itos-cc mutation run <flags>"
      Then the problem is "tests.coverage-failed", with the command that ran every listed test and its exit code
      And no mutant runs, no snapshot is written, and the exit code is 1
      And no problem is "mutation.uncovered"

      Examples:
        | flags                   |
        | --json                  |
        | --fail-uncovered --json |

    # slice-15: a harness that does not split the coverage run has each
    # listed test run alone for it (ID-MUT-123); one of those runs failing
    # is the same problem, naming the command that ran that test alone. The
    # Given is ID-MUT-137's, under a harness that does not split: ID-A-02
    # fails alone, for coverage as for its selection. Both drivers run it;
    # the fake measurer runs the fallback as the real one does once it fails
    # a test the declaration lists as Failing. Before slice-15 the run
    # logged "listed test ID-A-02 fails without any mutant" and judged every
    # mutant with no listed coverage, failing at the problem's step.
    @slice-15 @ID-MUT-269
    Scenario: A listed test that fails its own coverage run judges nothing
      Given the module of ID-MUT-122, whose harness ignores ITOS_CC_TEST_COVERDIR
      And ID-A-02 fails when it runs alone without a mutant, though the listed tests pass together
      When I run "itos-cc mutation run --json"
      Then the problem is "tests.coverage-failed", with the command that ran ID-A-02 alone and its exit code
      And no mutant runs, no snapshot is written, and the exit code is 1

  # A kill made by listed tests (listed-tests) depends on those tests, which
  # import nothing of the code, so the tests' hash of ID-MUT-65 never sees
  # them change. Decided with the person on 2026-10-07: such a kill is
  # fresh while the files its covering tests name are unchanged (q-19), and
  # while the support files are: mutation.tests.support, globs such as
  # features/*_test.go, where the checks the tests run live (q-21, raised by
  # a side agent after q-19). The snapshot records, for each listed
  # outcome, the hashes of those files and of every support file; mutation
  # check, run, sample and the graph compare them through
  # mutate.FreshnessOf, and run no list command to do it. Deleting a test
  # from its file changes that file, so its kills go stale too. One
  # support edit stales every listed kill, and each then reruns only its
  # covering tests. Own-scope outcomes keep today's rule. Since slice-40
  # (q-84), a listed kill whose tests are each dated, by a file or a
  # fingerprint, no longer rests on the tests that import its file as well
  # (ID-MUT-308 to ID-MUT-317).
  Rule: A kill made by listed tests goes stale when its tests change

    @listed-kill-freshness @ID-MUT-130
    Scenario: A listed kill stays fresh while its tests' files and the support files are unchanged
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And nothing has changed since
      When I run "itos-cc mutation check"
      Then the exit code is 0
      And "itos-cc mutation run" reuses that kill

    @listed-kill-freshness @ID-MUT-131
    Scenario: Editing the file of a covering test makes its kill stale
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And a.feature has changed since
      When I run "itos-cc mutation check"
      Then the problem is "mutation.stale" for that mutant's function, and its message names a.feature
      And "itos-cc mutation run" runs that mutant again, its own tests first

    @listed-kill-freshness @ID-MUT-132
    Scenario: Editing a file no covering test names changes nothing
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And b.feature, which defines only tests that do not cover it, has changed since
      When I run "itos-cc mutation check"
      Then the exit code is 0

    @listed-kill-freshness @ID-MUT-133
    Scenario: Editing a support file makes every listed kill stale
      Given itos-cc.yaml lists features/*_test.go under mutation.tests.support
      And a run recorded listed kills and kills by own tests
      And features/steps_test.go has changed since
      When I run "itos-cc mutation check"
      Then each function holding a listed kill is "mutation.stale", its message naming features/steps_test.go
      And the functions whose kills were all by their own tests stay fresh

    @listed-kill-freshness @ID-MUT-134
    Scenario: The graph agrees
      Given a.feature has changed since a run recorded a kill by ID-A-02
      When the graph is built
      Then the function holding that kill is marked stale, as mutation check calls it

  # slice-39, issue #30, decided with the person on 2026-10-18 (q-19's
  # option (b) where the list gives it, beside ADR-0011's per-file rule):
  # a listed kill is dated by its own test, not its test's whole file. A
  # Gherkin file holds many scenarios and changes far more often than any
  # one of them, so a comment or a new @wip scenario staled every kill its
  # file's scenarios made. mutation.tests.list may print a third
  # tab-separated column, a fingerprint of the test's own text,
  # <ID>\t<file>\t<fingerprint>; the lister computes it, and itos-cc stays
  # unaware of Gherkin. With a fingerprint, a listed outcome records each
  # covering test's fingerprint beside its file's hash, and stays fresh
  # while each covering test is still listed with that fingerprint, whatever
  # else its file holds and whichever file it moved to. An unchanged file
  # means an unchanged fingerprint, so the list command runs only when a
  # covering test's file changed, once per command, in mutation check, run,
  # sample and except alike: most checks run no lister, and only one after a
  # spec commit does (coordinator's call, 2026-10-21, after a side agent
  # noted the cost). A list that fails then is tests.list-failed, exit 1.
  # A test listed without a fingerprint is dated by its file, as ADR-0011
  # has it, with no list command run to judge it (MUT-130 to 133
  # unchanged). Support files
  # (mutation.tests.support) stale every listed kill, fingerprinted or not.
  # An outcome recorded before slice-39 keeps its file hashes, and is dated by
  # them until a run records it again. Before slice-39 the third column is
  # read as part of the file's name, so none of these scenarios holds.
  # The fix does nothing in practice until a lister prints the column:
  # itos's tests list is asked to in donvargax/itos#112, gated on the
  # itos-cc release that ships slice-39, since older releases read a third
  # column as part of the file's name.
  Rule: A listed kill is dated by its test's fingerprint when the list prints one

    @slice-39 @ID-MUT-276
    Scenario: A fingerprinted listed kill stays fresh when the rest of its file changes
      Given mutation.tests.list prints "ID-A-02", "a.feature" and the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills
      And a.feature has changed since, in a comment and in another scenario, and the list still prints "f1" for ID-A-02
      When I run "itos-cc mutation check"
      Then the exit code is 0
      And "itos-cc mutation run" reuses that kill
      And after ID-A-02 moves to b.feature, still listed with "f1", the kill is still fresh

    @slice-39 @ID-MUT-277
    Scenario: A changed or missing fingerprint makes the listed kill stale
      Given mutation.tests.list prints the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills
      And the list now prints "f2" for ID-A-02
      When I run "itos-cc mutation check"
      Then the problem is "mutation.stale" for that mutant's function, and its message names ID-A-02
      And "itos-cc mutation run" runs that mutant again, its own tests first
      And when the list no longer prints ID-A-02 at all, the kill is stale the same way

    @slice-39 @ID-MUT-278
    Scenario: Unfingerprinted tests and support files keep their rules
      Given itos-cc.yaml lists features/*_test.go under mutation.tests.support
      And mutation.tests.list prints a fingerprint for ID-A-02 in a.feature and none for ID-B-01 in b.feature
      And a run recorded a mutant ID-A-02 kills and another ID-B-01 kills
      When a.feature and b.feature change elsewhere, the list's fingerprint for ID-A-02 unchanged
      Then "itos-cc mutation check" reports ID-B-01's kill "mutation.stale", naming b.feature, and ID-A-02's fresh
      And after features/steps_test.go changes, both kills are "mutation.stale", naming it

    @slice-39 @ID-MUT-279
    Scenario: The list runs only when a fingerprinted kill's file changed
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 listed with a fingerprint in a.feature
      And a.feature has changed since, and the list command now fails
      When I run "itos-cc mutation check"
      Then the problem is "tests.list-failed", with its exit code, and the exit code is 1
      But with a.feature unchanged, mutation check runs no list command and the exit code is 0
      And when every recorded listed kill was listed without a fingerprint, mutation check runs no list command

  # q-84, decided with the person on 2026-10-22: a file's snapshot records the
  # tests that import it, own package and one hop (ID-MUT-65, ID-MUT-70), and
  # any change among them staled every outcome of the file. That included
  # kills the listed tests made, which rest on those tests and the support
  # files (ADR-0011, ADR-0013, slice-39), not on the importing tests. For
  # itos-cc, 97 of the 186 test files a domain file records are cmd/itos-cc's,
  # so one edit there staled every listed kill of every package it imports.
  # A kill, timeout or over-limit decided by listed tests now holds while the
  # tests it rests on (their files' hashes, or their fingerprints) and the
  # support files do. Own-scope outcomes, survivors and uncovered mutants
  # still rest on the importing tests. A function holding any of them is
  # stale as before, and mutation run re-judges only those, reusing its
  # listed kills. Check, run (--since included), sample, except and the graph
  # share the verdict, FreshnessOf's.
  # The person (2026-10-23): build it now, though T-46's measured nightly
  # (run 39930716718) bounds the saving at some 62 minutes a night, paid only
  # on nights the support files are unchanged; narrowing support (the ideas
  # listed-suites, listed-support-doublestar) is queued after it. And a
  # listed kill is exempt only when every test it rests on is dated, by a
  # recorded non-empty file hash or by a fingerprint: one whose test has no
  # recorded file, or an unreadable one, and no fingerprint keeps resting on
  # the importing tests (ID-MUT-317), so support is never its only date.
  # The coordinator's calls: run reuses per mutant (each listed kill on its
  # own), check judges per function (any outcome resting on a changed input
  # stales it); FreshnessOf keeps its order, the importing-test reason first
  # when any outcome rests on it, so no label changes; no snapshot change
  # and no version bump, so records written before benefit at once, an entry
  # already marked stale staying so until judged and a snapshot with
  # unrecorded tests staling everything as before; --since marks an entry it
  # does not judge stale only when it holds an outcome resting on the
  # importing tests; --all-tests and --test-command keep resting on both, as
  # they do.
  Rule: A listed kill does not rest on the tests that import its file

    @slice-40 @ID-MUT-308
    Scenario: A listed kill stays fresh when only a test importing its file changed
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And main_test.go, a test of main.go's own package, has changed since
      When I run "itos-cc mutation check"
      Then "greet" and "main", whose kills were all by listed tests, are fresh, and "half", killed by its own test, is "mutation.stale", its message naming main_test.go
      And "itos-cc mutation run" reuses that kill
      And that run judges the mutant of "half" again

    @slice-40 @ID-MUT-309
    Scenario: mutation sample still samples a listed kill after a test importing its file changed
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And main_test.go, a test of main.go's own package, has changed since
      When I run "itos-cc mutation sample --count 100"
      Then that mutant runs the run command selecting ID-A-02, after its own tests survive it
      And no "mutation.mismatch" is reported

    @slice-40 @ID-MUT-310
    Scenario: The file of a covering test still stales its kill, named, when an importing test changed too
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And main_test.go, a test of main.go's own package, has changed since
      And a.feature has changed since
      When I run "itos-cc mutation check"
      Then the problem is "mutation.stale" for that mutant's function, and its message names a.feature
      And "itos-cc mutation run" runs that mutant again, its own tests first

    @slice-40 @ID-MUT-311
    Scenario: A support file still stales every listed kill, named, when an importing test changed too
      Given itos-cc.yaml lists features/*_test.go under mutation.tests.support
      And a run recorded listed kills and kills by own tests
      And main_test.go, a test of main.go's own package, has changed since
      And features/steps_test.go has changed since
      When I run "itos-cc mutation check"
      Then each function holding a listed kill is "mutation.stale", its message naming features/steps_test.go
      And "half", killed by its own test, is "mutation.stale", its message naming main_test.go

    @slice-40 @ID-MUT-312
    Scenario: A fingerprinted listed kill stays fresh when its file and an importing test changed elsewhere
      Given mutation.tests.list prints "ID-A-02", "a.feature" and the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills
      And a.feature has changed since, in a comment and in another scenario, and the list still prints "f1" for ID-A-02
      And main_test.go, a test of main.go's own package, has changed since
      When I run "itos-cc mutation check"
      Then "greet", whose kills were all by listed tests, is fresh
      And "itos-cc mutation run" reuses that kill

    @slice-40 @ID-MUT-313
    Scenario: A changed fingerprint still stales its kill, named, when an importing test changed too
      Given mutation.tests.list prints the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills
      And main_test.go, a test of main.go's own package, has changed since
      And the list now prints "f2" for ID-A-02
      When I run "itos-cc mutation check"
      Then the problem is "mutation.stale" for that mutant's function, and its message names ID-A-02

    # A guardrail: it passes before the change and must still pass after.
    @slice-40 @ID-MUT-314
    Scenario: Survivors, uncovered mutants and own kills still rest on the tests that import their file
      Given a fresh outcome recorded with scope "listed" and tests ["ID-A-02"]
      And main_test.go, a test of main.go's own package, has changed since
      When I run "itos-cc mutation check"
      Then "main", holding a survivor of the listed tests, "half", killed by its own test, and "twice", uncovered, are each "mutation.stale", naming main_test.go
      And "itos-cc mutation run" judges the survivor of "main" and the mutant of "half" again

    @slice-40 @ID-MUT-315
    Scenario: --since keeps a function it does not judge fresh when its kills were all by listed tests
      Given a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a.feature
      And a commit after "base" changed main_test.go and "twice", and nothing else of main.go
      When I run "itos-cc mutation run --since base"
      Then "itos-cc mutation check" calls "greet" and "main" fresh, and "half" "mutation.stale", kept by a mutation run that did not judge it

    @slice-40 @ID-MUT-316
    Scenario: The graph agrees
      Given main_test.go has changed since a run recorded a kill by ID-A-02
      When the graph is built
      Then the function holding that kill is not marked stale and "half" is, as mutation check calls them

    # A guardrail (the person, 2026-10-23): ID-A-02 has no recorded file in
    # this fixture and no fingerprint, so its kill of "greet" is not dated by
    # anything but the support files, and keeps resting on the importing
    # tests. It passes before the change and must still pass after.
    @slice-40 @ID-MUT-317
    Scenario: A listed kill whose test has no recorded file still rests on the tests that import its file
      Given a fresh outcome recorded with scope "listed" and tests ["ID-A-02"]
      And main_test.go, a test of main.go's own package, has changed since
      When I run "itos-cc mutation check"
      Then "greet" is "mutation.stale", its message naming main_test.go

  # listed-tests timed every run of listed tests from one run of all of
  # them: the coverage run (or the sum of the per-test runs), times the
  # timeout factor. For itos, about 3 minutes times 10, so a mutant that
  # hangs a selection of one or two scenarios waited about 30 minutes, and
  # the only proof that the listed tests pass without a mutant was all of
  # them passing together. Found by a side agent before v0.5.0; the fix is
  # the coordinator's call (2026-10-07): each selection of listed tests,
  # the first time a mutant needs it, runs once without a mutant in the
  # worker's copy. Its time, times the timeout factor plus 5 seconds
  # (timeout-allowance, ID-MUT-149), is that selection's timeout (as
  # ID-MUT-14 and 15 for a file's tests), and a selection that fails without a mutant decides none of the
  # mutants that would run it, as a failing baseline decides none of its
  # file's. Selections are cached by their set of IDs for the run; mutation
  # sample follows the same rule.
  Rule: Each selection of listed tests has its own baseline

    @listed-selection-baseline @ID-MUT-135
    Scenario: A selection's timeout comes from its own run
      Given listed tests ID-A-01, which runs in about a second alone, and ID-A-02, which takes about ten
      And a mutant only ID-A-01 reaches makes the binary hang
      When I run "itos-cc mutation run"
      Then ID-A-01 runs once alone without a mutant before that mutant's run
      And the mutant times out after ten times that run plus 5 seconds, not after ten times both tests' time
      And its outcome is "timeout", counted killed

    @listed-selection-baseline @ID-MUT-136
    Scenario: A selection is run without a mutant once per run
      Given two mutants whose lines only ID-A-02 reaches, both surviving their own tests
      When I run "itos-cc mutation run"
      Then ID-A-02 runs alone without a mutant exactly once

    @listed-selection-baseline @ID-MUT-137
    Scenario: A selection that fails without a mutant decides none of its mutants
      Given ID-A-02 fails when it runs alone without a mutant, though the listed tests pass together
      When I run "itos-cc mutation run"
      Then the problem is "tests.selection-failed", with the IDs of the selection
      And no mutant that would run ID-A-02 is recorded killed, and its file's snapshot keeps what it held
      And the exit code is 1

  # slice-14, issue #53: Go's cover profile gives an empty case clause a
  # block of 0 statements, and reading its covered weight as "count > 0
  # times statements" made a clause that ran read as not run, so a site in
  # it (the case's value) was uncovered though a test runs it. A block that
  # ran is covered whatever its statement count.
  Rule: A site in a Go case clause the tests run is covered

    @slice-14 @ID-MUT-266 @binary-only
    Scenario: A site in an empty Go case clause the tests run is judged, not uncovered
      Given the Go module "example.com/k" with the file "kind.go":
        """
        package k

        func Kind(n int) string {
        	s := "other"
        	switch n {
        	case 0:
        	case 2:
        		s = "two"
        	}
        	return s
        }
        """
      And its test calls Kind(0) and expects "other"
      When I run "itos-cc mutation run --json kind.go"
      Then the mutant of line 6's 0 into 1 is survived, not uncovered
      # Kind(0) still returns "other" with case 1, so the test cannot kill it

  # Issue #9, part 2: mutants on lines no test executes are not run, so a
  # changed function with no test passes. A gate passes --fail-uncovered
  # (with --since for a task's commits) to make each uncovered mutant a
  # failure. An explicit flag, not implied by --since, so a gate mode (#8)
  # can turn it on later and plain runs keep today's output. Uncovered
  # mutants are never reused from a snapshot, so each run decides them
  # afresh from coverage. Without --fail-uncovered, --no-coverage still
  # runs every mutant. ADR-0017 makes --no-coverage conflict with strict
  # mode; strict Go also requires independent executable-block evidence,
  # even where no mutation site exists (strict-go-coverage below).
  Rule: Uncovered mutants as failures

    @slice-2 @ID-MUT-47
    Scenario: An uncovered mutant fails the run
      Given coverage shows line 12 of src/board.ts is never executed
      And every mutant on the lines the tests execute is killed
      When I run "itos-cc mutation run --fail-uncovered src/board.ts"
      Then each mutant on line 12 is listed as "uncovered src/board.ts:12:9 `>` → `>=` in board#place"
      And the exit code is 1

    @slice-2 @ID-MUT-48
    Scenario: A file the tests never load fails whole
      Given coverage measured other TypeScript files but never src/unused.ts
      When I run "itos-cc mutation run --fail-uncovered src/unused.ts"
      Then every mutant in src/unused.ts is listed as uncovered
      And the exit code is 1

    @slice-2 @ID-MUT-49
    Scenario: With --since only the judged functions' uncovered mutants fail
      Given a commit after "base" changed "Board#place", which no test executes
      And "Board#clear", unchanged, is not executed by any test either
      When I run "itos-cc mutation run --since base --fail-uncovered"
      Then the mutants of "Board#place" are listed as uncovered
      And none of "Board#clear" is
      And the exit code is 1

    @slice-2 @ID-MUT-50
    Scenario: Uncovered mutants as JSON
      When I run "itos-cc mutation run --fail-uncovered --json src/board.ts"
      Then each uncovered mutant is a "mutation.uncovered" problem with file, line, column, function, original, and replacement
      And "ok" is false

    # ADR-0017 deliberately replaces the former promise that combining
    # --no-coverage with --fail-uncovered silently runs every mutant.
    @slice-2 @strict-go-coverage @ID-MUT-51
    Scenario: Skipping coverage conflicts with strict mode
      When I run "itos-cc mutation run --no-coverage --fail-uncovered src/board.ts"
      Then the problem is "flags.conflict", naming "--no-coverage"
      And no coverage command or mutant runs
      And the exit code is 80

  # slice-25, issue #18: a Go file the build constraints leave out on the
  # system the run is on, probe_windows.go on Linux, was mutated as any
  # other. Its package's tests never build it, so its mutants were
  # uncovered, or survived with --no-coverage, and with --fail-uncovered
  # its functions were also mutation.coverage-missing (#18's comment). An
  # item changing code only Windows builds could never pass the gate on
  # Linux or macOS, and the gate of a project whose CI runs on Linux alone
  # failed whatever its tests did.
  # Decided by the person on 2026-10-18 (#18, why): a file the build
  # constraints of the current platform leave out is reported as not
  # built here, with a named reason, and fails nothing, --fail-uncovered
  # included. Outcomes are not merged across platforms. A branch on
  # runtime.GOOS stays a test-design matter: its lines are uncovered as
  # before.
  # Decided by the person on 2026-10-21: only constraints that rule out
  # this GOOS and GOARCH count; check and sample here ignore the results
  # another system recorded, a survivor included; mutation list keeps
  # listing the file's sites. Coordinator's calls, 2026-10-21, for the
  # rest, the rule pure in the domain over the file's name and text with
  # go/build/constraint, GOOS and GOARCH from the toolchain's go env:
  # - The platform is the GOOS and GOARCH the project's go env reports, a
  #   value, never runtime.GOOS in the logic (AGENTS.md). A file is not
  #   built here when its name's _GOOS or _GOARCH suffix, or its //go:build
  #   line, rules out that GOOS and GOARCH whatever the other tags. A file
  #   left out only by a tag of the project's own (integration, ignore) is
  #   judged as before, as listed tests may build it.
  # - mutation run, check and sample leave such a file out of "files" and
  #   name it once in "not_built": [{"file", "platform"}] (a key added,
  #   there only when one is), with "platform" as "linux/amd64". Each
  #   command says on stderr "itos-cc: <file>: not built on <platform>,
  #   so not judged here". It fails nothing.
  # - Its snapshot, written by a run on a system that builds it, is left
  #   as it is: not rewritten, not an orphan, not read. mutation check does
  #   not judge its recorded outcomes here, and mutation sample draws none
  #   of them, as a mutant the build leaves out would survive and read as a
  #   mismatch.
  # - A counted run selects no site in it, as MUT-246 does for an
  #   excluded source; mutation list keeps listing its sites, as they are
  #   the project's.
  # - Only Go has build constraints itos-cc reads. Other languages are as
  #   before.
  # Both drivers run it, on every system. The fixture names "another
  # system" from the driver's platform: windows on Linux and macOS, linux
  # on Windows; and another architecture from the host's.
  Rule: Code only another system builds is not judged on this one

    # Before slice-25 the file's mutants are uncovered, and survivors with
    # --no-coverage; the run fails them and exits 1, failing at the Then.
    # With --no-coverage, which measures nothing, the But's branch mutant
    # runs and survives, as no test here executes its line.
    @slice-25 @ID-MUT-304
    Scenario Outline: A Go file only another system builds is not judged here, left out by <constraint>
      Given a Go package whose function Probe is split by <constraint> into a file only another system builds and a file this one builds, whose test reaches the second
      When I run "itos-cc mutation run <flags> --json" for both files
      Then the first is in "not_built" with this system's platform, and not in "files"
      And stderr says it is not built on this system's platform, so not judged here
      And the second's mutants are judged as its tests decide, and the exit code is 0
      But when the second also branches on runtime.GOOS for another system, its mutants on that branch are uncovered, and the exit code is 1

      Examples:
        | constraint                                    | flags            |
        | its file name's suffix                        | --fail-uncovered |
        | a //go:build line naming another system       | --fail-uncovered |
        | a //go:build line naming another architecture | --fail-uncovered |
        | its file name's suffix                        | --no-coverage    |

    # Before slice-25 run judges the changed file, its mutants uncovered,
    # and check reports its changed function mutation.stale; both exit 1,
    # failing at the Then.
    @slice-25 @ID-MUT-305
    Scenario Outline: A gate on this system passes a range that changed only code another system builds
      Given the package of MUT-304, its snapshots committed, and a commit since main that changes only the file another system builds
      When I run "itos-cc mutation <command> --since main <mode> --json"
      Then that file is in "not_built", and no problem names it
      And the exit code is 0

      Examples:
        | command | mode                   |
        | run     | --fail-uncovered       |
        | run     | --fail-uncovered=lines |
        | check   | --fail-uncovered       |
        | check   | --fail-uncovered=lines |

    # The snapshot is raw JSON, as a run on a system that builds the file
    # writes it: killed mutants and one survivor.
    # Before slice-25 check reports the recorded survivor as
    # mutation.survived and exits 1, failing at the Then.
    @slice-25 @ID-MUT-306
    Scenario: The results another system recorded are neither judged nor sampled here
      Given the package of MUT-304, where the snapshot of the file another system builds records killed mutants and one survivor, as a run on that system recorded them
      When I run "itos-cc mutation check --json"
      Then that file is in "not_built", no problem names it, and the exit code is 0
      And "itos-cc mutation sample --count 20" runs none of its mutants and exits 0
      And "itos-cc mutation run" leaves its snapshot byte for byte, and names no orphan

    # A scenario of its own, as every counted step skips on Windows (#29),
    # so MUT-306 still runs there. Before slice-25 the count-one run may
    # select a site in the file, failing at the Then.
    @slice-25 @ID-MUT-307
    Scenario: A counted run selects no site in a file only another system builds
      Given the package of MUT-304, committed, whose only eligible sites are in the file another system builds and in a function of the other file
      When a count-one run judges it, with several seeds
      Then no selected site is in the file another system builds

  Rule: Differential runs

    @ID-MUT-23
    Scenario: Killed mutants of unchanged functions stay killed
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since
      When I run "itos-cc mutation run"
      Then those mutants are reused without running

    @ID-MUT-24
    Scenario: Changed functions rerun
      Given "Board#place" changed since the previous run
      When I run "itos-cc mutation run"
      Then every mutant of "Board#place" runs again

    @ID-MUT-25
    Scenario: Moving a function does not count as a change
      Given an import was added above "f" so it moved down the file
      When I run "itos-cc mutation run"
      Then the killed mutants of "f" are still reused

    @ID-MUT-26
    Scenario: Survivors are always retried
      Given a mutant of "Board#place" survived the previous run
      When I run "itos-cc mutation run"
      Then it runs again, since new tests may kill it
      # unless itos-cc.yaml excepts it: an excepted survivor is reused as a
      # kill is, while its function and its tests are unchanged (Rule:
      # Equivalent mutants excepted, each with a reason)

    @ID-MUT-27
    Scenario: Forcing a full rerun
      When I run "itos-cc mutation run --mutate-all"
      Then killed mutants of unchanged functions run again too

  # Units sharing a namespace#name in one file (several Go init functions,
  # Kotlin or TypeScript overloads) were told apart three ways: mutation run
  # kept only the last one's hash, so only it reused its outcomes, and the
  # --since judge and the entries it keeps keyed by name alone; mutation
  # check matched any entry of the name with the same hash; the graph paired
  # them by position. All of them match an entry by name and hash, as check
  # did (the coordinator's call, 2026-10-07, on the person's go-ahead to fix
  # mutate-same-name-units and graph-stale-same-name together). A unit is
  # fresh when an entry of its name has its hash, so reordering such units
  # changes nothing and editing one makes only that one stale; units with
  # the same name and the same hash pair with their entries in file order.
  Rule: Functions sharing a name

    @same-name-units @ID-MUT-114
    Scenario: Functions sharing a name each reuse their own results
      Given src/setup.go declares two init functions with different bodies, each with a mutant the tests kill
      And a previous run recorded both
      When I run "itos-cc mutation run src/setup.go" with nothing changed
      Then the mutants of both init functions are reused, and none runs

    @same-name-units @ID-MUT-115
    Scenario: Editing one of two functions sharing a name reruns only it
      Given a previous run recorded both init functions of src/setup.go, all killed
      And the second init function has changed since
      When I run "itos-cc mutation check src/setup.go"
      Then the only problem is "mutation.stale", for the second init function
      And "itos-cc mutation run src/setup.go" runs only the second init function's mutants

    @same-name-units @ID-MUT-116
    Scenario: Reordering functions sharing a name changes nothing
      Given a previous run recorded both init functions of src/setup.go, all killed
      And the two have swapped places since, unchanged
      When I run "itos-cc mutation check src/setup.go"
      Then the exit code is 0
      And "itos-cc mutation run src/setup.go" reuses every mutant

  # Issue #9, part 1: a gate judges a task's commits, not the working tree,
  # so --since <ref> picks the functions the commits since <ref> changed.
  # The range is git diff <ref>...HEAD: the branch's own commits whatever its
  # base did since, never uncommitted changes. A deleted line counts as a
  # change of the lines either side of it. Paths are relative to the working
  # directory, so from a subdirectory only the changes under it count.
  # Uncovered mutants failing (#9 part 2) and each mutant in --json (#9
  # part 3) are separate items. Decided with the person on 2026-10-06:
  # --since refuses --changed, paths narrow the range, and the functions
  # judged are counted in the summary and named in --json. The started code
  # (mutate.Options.Judge, project.ChangedSince) follows these already.
  Rule: Judging the functions a range of commits changed

    @slice-1 @ID-MUT-37
    Scenario: Only the functions the commits since a ref changed are judged
      Given a commit after "base" changed lines of "Board#place" in src/board.ts
      And "Board#clear" in the same file did not change
      When I run "itos-cc mutation run --since base"
      Then the mutants of "Board#place" run
      And no mutant of "Board#clear" runs
      And stdout says "src/board.ts: … (judged 1 of 2 functions)"

    @slice-1 @ID-MUT-38
    Scenario: Functions not judged keep what their snapshot holds
      Given a previous run recorded a survivor in "Board#clear"
      And only "Board#place" changed since "base"
      When I run "itos-cc mutation run --since base"
      Then the snapshot of src/board.ts still records that survivor in "Board#clear"
      And it is not reported, and the exit code is 0 when every mutant of "Board#place" is killed
      # a function never judged before gets no entry: it neither ran nor has an outcome to keep

    @slice-1 @ID-MUT-39
    Scenario: Uncommitted changes are not in the range
      Given "Board#place" changed since "base" in a commit
      And "Board#clear" has an uncommitted change
      When I run "itos-cc mutation run --since base"
      Then "Board#place" is judged
      And "Board#clear" is not

    @slice-1 @ID-MUT-40
    Scenario: Deleting lines changes the function around them
      Given a commit after "base" only deleted a line inside "Board#place"
      When I run "itos-cc mutation run --since base"
      Then "Board#place" is judged

    @slice-1 @ID-MUT-41
    Scenario: Paths narrow the range
      Given commits after "base" changed src/board.ts and lib/util.ts
      When I run "itos-cc mutation run --since base src"
      Then only src/board.ts is mutated

    @slice-1 @ID-MUT-42
    Scenario: Nothing changed since the ref
      Given no commit after "base" changed a source file
      When I run "itos-cc mutation run --since base"
      Then stderr says "itos-cc: no source files to mutate"
      And the exit code is 0

    @slice-1 @ID-MUT-43
    Scenario: A ref git cannot resolve is a usage error
      When I run "itos-cc mutation run --since nosuch"
      Then stderr says "itos-cc: --since nosuch: not a commit in this repository"
      And the problem is "since.bad-ref", with ref "nosuch"
      And the exit code is 80

    @slice-1 @ID-MUT-44
    Scenario: --since outside a git repository is a missing environment
      Given the working directory is not a git repository
      When I run "itos-cc mutation run --since main"
      Then the problem is "since.no-git"
      And the exit code is 82

    @slice-1 @ID-MUT-45
    Scenario: --since and --changed are not combined
      When I run "itos-cc mutation run --since main --changed"
      Then the problem is "flags.conflict", with flag "--changed"
      And the exit code is 80
      # --changed judges whole files of the working tree, --since functions of
      # commits: together they would judge neither

    @slice-1 @ID-MUT-46
    Scenario: The functions judged, as JSON
      When I run "itos-cc mutation run --since base --json"
      Then each file in "files" has "judged", the namespace#name of each function judged
      And without --since no file has "judged"

    # A file the range changed only outside its functions (its imports) is
    # selected with "judged 0 of N", and its snapshot and summary comment
    # were written though nothing in it ran. Nothing is written for it (the
    # coordinator's call, 2026-10-07). The summary comment counting the whole
    # snapshot while stdout counts what was judged is by design: the comment
    # is the file's state, stdout the run's.
    @since-untouched-files @ID-MUT-102
    Scenario: A file with nothing judged is left as it was
      Given a commit after "base" changed only the imports of src/board.ts
      When I run "itos-cc mutation run --since base"
      Then stdout says "src/board.ts: … (judged 0 of 2 functions)"
      And neither .metrics/mutate/src/board.ts.json nor src/board.ts is written

    # git diff -U0 prints no hunk for a file renamed without a change, so
    # none of its functions was judged and its snapshot stayed at the old
    # path while the new one had none: mutation check then called every
    # function missing. --since follows renames: a renamed file's snapshot
    # moves with it, and only the functions the range changed are judged, as
    # a move is no change (the coordinator's call, 2026-10-07). The scenarios
    # rename a Go file within its package, so no test's import changes and
    # the results stay fresh.
    @since-follows-renames @ID-MUT-103
    Scenario: A renamed file's results follow it
      Given fresh results for every function of src/board.go, all killed
      And a commit after "base" renamed src/board.go to src/grid.go without changing it
      When I run "itos-cc mutation run --since base"
      Then no function is judged
      And .metrics/mutate/src/grid.go.json holds the results .metrics/mutate/src/board.go.json held, under its new path
      And .metrics/mutate/src/board.go.json is gone
      And "itos-cc mutation check src/grid.go" exits 0

    @since-follows-renames @ID-MUT-104
    Scenario: A renamed and edited file judges only what changed
      Given fresh results for every function of src/board.go, all killed
      And a commit after "base" renamed src/board.go to src/grid.go and changed "Board#Place"
      When I run "itos-cc mutation run --since base"
      Then only "Board#Place" is judged
      And the other functions' results are kept in .metrics/mutate/src/grid.go.json

  # Issue #22 (slice-11): a function's hash covered its whole text, comments
  # included, so a commit that only added a lint exception's comment to one
  # of its lines made it changed for --since and its results stale, asking
  # for a run no mutant of it could answer differently. Decided with the
  # person on 2026-10-18: the hash leaves comments out, in every language,
  # with the space and line breaks only a comment needed, so a function
  # whose code is unchanged keeps its hash and its results, and a recorded
  # site a comment moved keeps its outcome. --since judges a function the
  # range changed only when its hash changed too: a function the range
  # touched only in its comments is not judged, by mutation run, check or
  # sample. Shipping it changes the hash of every function whose text holds
  # a comment, so those functions' results read stale once, and an
  # exception in itos-cc.yaml on such a function exception-stale until
  # mutation except --renew carries it over; a function without comments
  # keeps its hash and its results. A site's offset is
  # taken in the comment-free text, so a comment above a site keeps its
  # recorded outcome. Python docstrings are code, not comments. The steps
  # that made a function changed by editing only a comment (changePlace,
  # changedBefore, deletedLine, rename and their kin) change code now.
  Rule: A change to a function's comments alone is no change

    @slice-11 @ID-MUT-256
    Scenario Outline: A comment-only change inside a function leaves its results fresh
      Given fresh results for every function of src/board.go, all killed
      And a commit after "base" only <changed> inside "Board#Place"
      When I run "itos-cc mutation check --since base"
      Then the exit code is 0
      And "itos-cc mutation check --json src/board.go" gives "Board#Place" the state "fresh"
      And "itos-cc mutation run --since base" judges no function and runs no mutant
      But after a commit that changes its code, "itos-cc mutation check --since base" reports "mutation.stale" for "Board#Place"

      Examples:
        | changed                                                      |
        | added a comment on a line of its own above its mutation site |
        | added a comment at the end of its mutation site's line       |
        | reworded its comment                                         |
        | deleted its comment's line                                   |

    # Issue #22 as found: no range had judged the function, so it had no
    # results, and its comment made it "mutation.missing".
    @slice-11 @ID-MUT-257
    Scenario: A function with no results whose only change is a comment is not judged
      Given "Board#Reset" has no mutation results, and a commit after "base" only added a comment at the end of one of its lines
      When I run "itos-cc mutation check --since base"
      Then the exit code is 0
      And "itos-cc mutation run --since base" judges no function and runs no mutant

    @slice-11 @ID-MUT-258
    Scenario Outline: A comment-only change leaves a <language> function fresh
      Given a <language> function with fresh results, all killed, committed at "base"
      And a commit after "base" only added a comment on a line of its own above its mutation site, and another at the end of that site's line
      When I run "itos-cc mutation check --since base"
      Then the exit code is 0
      And "itos-cc mutation check --json" of its file gives the function the state "fresh"

      Examples:
        | language   |
        | typescript |
        | python     |
        | kotlin     |

      # slice-36, dart-support: Dart's comment and documentation_comment
      # nodes are left out of the hash since slice-29, so this row is a
      # guardrail that passes before slice-36. Strict Dart evidence is bound
      # to that hash, so a comment changes no evidence either. It needs no
      # Flutter: the results come from a --test-command.
      @slice-36
      Examples:
        | language |
        | dart     |

  # Issue #8, part 1: a gate (a commit hook) proves that results exist for
  # exactly the code being committed, without running anything. Decided with
  # the person on 2026-10-06 (q-1 to q-3): it is a subcommand of the group,
  # mutation check, and it gives the verdict a full run would give, from the
  # snapshots alone. For each selected function it compares the function's
  # hash with its entry in .metrics/mutate/: no entry is missing, another
  # hash is stale (a move is not a change, as for reuse), and a fresh entry
  # fails on a recorded survivor, or on a recorded uncovered mutant with
  # --fail-uncovered. Without strict Go statement coverage, a function with
  # no mutation site needs no entry. ADR-0017 adds strict Go obligations. It
  # takes the selection of mutation run: paths, --changed, and --since, which
  # checks only the functions the range changed. Results going stale when
  # the tests change (#8 part 2) and re-running a sample in CI (#8 part 3)
  # are their own items.
  Rule: Checking cached results without running

    @mutation-check @ID-MUT-57
    Scenario: Fresh results with every mutant killed pass
      Given a run recorded every mutant of src/board.ts killed
      And no function of src/board.ts has changed since
      When I run "itos-cc mutation check src/board.ts"
      Then no coverage or test command runs
      And the exit code is 0

    @mutation-check @ID-MUT-58
    Scenario: A function changed since its results is stale
      Given "Board#place" changed since the run that recorded it
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.stale", with file "src/board.ts" and function "Board#place"
      And the exit code is 1

    @mutation-check @ID-MUT-59
    Scenario: A function with no results is missing
      Given "Board#reset" was added after the last run
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.missing", with file "src/board.ts" and function "Board#reset"
      And the exit code is 1
      # a file with no snapshot at all has every function with a site missing

    @mutation-check @ID-MUT-60
    Scenario: A recorded survivor fails the check
      Given a fresh run recorded a survivor in "Board#place"
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.survived", with its file, line, column, function, original, and replacement
      And the exit code is 1

    @mutation-check @ID-MUT-61
    Scenario: A recorded uncovered mutant fails only with --fail-uncovered
      Given a fresh run recorded every covered mutant of src/board.ts killed and one uncovered
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0
      But "itos-cc mutation check --fail-uncovered src/board.ts" reports it as "mutation.uncovered" and exits 1

    @mutation-check @ID-MUT-62
    Scenario: With --since only the functions the range changed are checked
      Given only "Board#place" changed since "base", and its fresh results are all killed
      And "Board#clear", unchanged, has a recorded survivor
      When I run "itos-cc mutation check --since base"
      Then the exit code is 0

    @mutation-check @ID-MUT-63
    Scenario: A function with no mutation site needs no results
      Given "Board#size" has no mutation site and no entry in the snapshot
      And every other function of src/board.ts has fresh results, all killed
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0

    @mutation-check @ID-MUT-64
    Scenario: Each function's state as JSON
      When I run "itos-cc mutation check --json src/board.ts"
      Then each file in "files" has "functions", each with function and state "fresh", "stale" or "missing"
      And the problems are those the plain output prints

    # A fresh entry was judged only by the mutants it records, so when a
    # newer itos-cc adds a mutation operator, a function whose text did not
    # change has sites its entry never recorded, and mutation check passed
    # it although no run judged them. Decided with the person on 2026-10-07
    # (q-13): such a function is stale, and the problem names the sites; a
    # mutation run runs only those sites and reuses the recorded outcomes.
    # The fixtures stand for a new operator by removing a mutant from the
    # entry.
    @check-unrecorded-sites @ID-MUT-106
    Scenario: A site the entry never recorded makes the function stale
      Given fresh results for every function of src/board.ts, all killed
      But the entry of "Board#place" does not record one of its sites, as when a newer itos-cc adds an operator
      When I run "itos-cc mutation check src/board.ts"
      Then the problem is "mutation.stale", with file and function "Board#place", and its message names that site's line, column, original and replacement
      And the exit code is 1

    @check-unrecorded-sites @ID-MUT-107
    Scenario: mutation run runs only the sites the entry never recorded
      Given the entry of "Board#place" does not record one of its sites, and records every other one killed
      When I run "itos-cc mutation run src/board.ts"
      Then only that site's mutant runs
      And the other mutants of "Board#place" are reused
      And afterwards "itos-cc mutation check src/board.ts" exits 0

  # Issue #8, part 2, and IDEAS.md "Killed mutants must notice test
  # changes": a result was keyed by its function's hash alone, which covers
  # the function's own source, so deleting the test that killed a mutant
  # changed nothing and its kill was reused. Decided with the person on
  # 2026-10-06 (q-4, q-5): each file's snapshot also records the hash of
  # every test file the graph finds importing it (for Go, its package's test
  # files and the test files of packages that import it). A kill is reused,
  # and a function is fresh for mutation check, only while both its own
  # hash and those tests' hashes match; since slice-40 (q-84), a kill by
  # listed tests each dated by a file or a fingerprint rests on those tests
  # and the support files instead (ID-MUT-308 to ID-MUT-317). A snapshot
  # written before this records no tests and is stale. These are the baseline dependencies;
  # listed outcomes also follow ADR-0013, and Go all-tests and test-command
  # outcomes also follow ADR-0016 and the all-tests-freshness scenarios.
  # A test that does not import the file leaves an own-scope outcome fresh.
  Rule: Results go stale when their tests change

    @mutation-test-hash @ID-MUT-65
    Scenario: The snapshot records the tests that import the file
      Given src/board.test.ts imports src/board.ts and src/other.test.ts does not
      When I run "itos-cc mutation run src/board.ts"
      Then .metrics/mutate/src/board.ts.json records the hash of src/board.test.ts
      And not of src/other.test.ts

    @mutation-test-hash @ID-MUT-66
    Scenario: Changing a test that imports the file reruns its kills
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since, but src/board.test.ts has
      When I run "itos-cc mutation run src/board.ts"
      Then every mutant of "Board#place" runs again

    @mutation-test-hash @ID-MUT-67
    Scenario: Deleting a test that imports the file reruns its kills
      Given a previous run killed every mutant of "Board#place"
      And src/board.test.ts was deleted since
      When I run "itos-cc mutation run src/board.ts"
      Then no mutant of "Board#place" is reused

    @mutation-test-hash @ID-MUT-68
    Scenario: mutation check calls results stale when their tests changed
      Given fresh results for every function of src/board.ts, all killed
      And src/board.test.ts changed since they were recorded
      When I run "itos-cc mutation check src/board.ts"
      Then each function of src/board.ts is a "mutation.stale" problem whose message names src/board.test.ts
      And the exit code is 1

    @mutation-test-hash @ID-MUT-69
    Scenario: A snapshot that records no tests is stale
      Given .metrics/mutate/src/board.ts.json was written before snapshots recorded tests
      And no function of src/board.ts has changed since
      When I run "itos-cc mutation check src/board.ts"
      Then each function of src/board.ts is a "mutation.stale" problem
      And "itos-cc mutation run src/board.ts" reuses none of its kills

    @mutation-test-hash @ID-MUT-70
    Scenario: A Go file's tests are its package's and those of packages that import it
      Given src/board.go is in package board, tested by src/board_test.go
      And package app imports board and is tested by app/app_test.go
      When I run "itos-cc mutation run src/board.go"
      Then .metrics/mutate/src/board.go.json records the hashes of src/board_test.go and app/app_test.go

    # A snapshot records the tests that import its file once per file, so
    # when they changed, mutation run --since, which rewrites the file's
    # snapshot with the new tests' hashes, could not keep the old outcomes of
    # the functions it did not judge without calling them fresh: it dropped
    # their entries, check then called them missing, and any survivor they
    # recorded was lost. Decided with the person on 2026-10-07 (q-14): it
    # keeps those entries, marked stale, so check calls them stale and their
    # outcomes are kept until a run judges them again.
    @since-keeps-stale-entries @ID-MUT-108
    Scenario: --since keeps the entries it does not judge, marked stale, when their tests changed
      Given fresh results for "Board#place" and "Board#clear", with a survivor recorded in "Board#clear"
      And a commit after "base" changed "Board#place" and src/board.test.ts
      When I run "itos-cc mutation run --since base"
      Then .metrics/mutate/src/board.ts.json still holds the entry of "Board#clear" and its survivor, marked stale
      And "itos-cc mutation check src/board.ts" reports "Board#clear" as "mutation.stale", not "mutation.missing"
      And a later "itos-cc mutation run src/board.ts" runs the mutants of "Board#clear" again

  # Issue #8, part 3: mutation check trusts the snapshots, so a cache written
  # by hand, or against other code, passes it. CI re-runs a few cached
  # mutants and compares. Decided with the person on 2026-10-07 (q-6 to
  # q-8): it is a subcommand of the group, mutation sample, and it writes
  # nothing, neither snapshot nor summary comment. It samples only the fresh
  # entries of its selection (stale and missing ones are mutation check's to
  # report), and among them only mutants that ran: killed, timeout or
  # survived, never uncovered, so no coverage runs. A recorded survivor that
  # still survives is no failure here: failing on survivors is mutation
  # check's. --count N draws N of them from the whole selection, 20 by
  # default, all when there are fewer. The draw is seeded with the HEAD
  # commit's id, so a rerun of one commit samples the same mutants and each
  # new commit others; --seed <text> overrides it to reproduce a run. Any
  # outcome that differs from the snapshot fails as mutation.mismatch,
  # naming both, whichever way it went: the cache was wrong either way.
  # Killed and timeout agree (the coordinator's call, from rule MUT-14: both
  # mean the tests noticed, and a slower machine turns one into the other).
  # It takes the selection of mutation check: paths, --changed and --since.
  Rule: Re-running a sample of cached results

    @mutation-sample @ID-MUT-71
    Scenario: A sample whose outcomes hold passes and writes nothing
      Given fresh results for every function of src/board.ts, with killed mutants and one survivor
      And the tests still kill and miss the same mutants
      When I run "itos-cc mutation sample src/board.ts"
      Then the baseline runs, then each sampled mutant, in a worker's copy
      And no coverage command runs
      And .metrics/mutate/src/board.ts.json and src/board.ts are unchanged
      And the exit code is 0

    @mutation-sample @ID-MUT-72
    Scenario: A recorded kill that now survives is a mismatch
      Given the snapshot records the mutant `>` → `>=` at src/board.ts:5:9 in "Board#place" as killed
      But the tests no longer kill it
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.mismatch", with file, line, column, function, original, replacement, recorded "killed" and outcome "survived"
      And the exit code is 1

    @mutation-sample @ID-MUT-73
    Scenario: A recorded survivor that is now killed is a mismatch too
      Given the snapshot records a mutant of "Board#place" as survived
      But the tests now kill it
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.mismatch", with recorded "survived" and outcome "killed"
      And the exit code is 1

    @mutation-sample @ID-MUT-74
    Scenario: Killed and timeout agree
      Given the snapshot records a mutant of "Board#place" as killed
      And it now runs past its timeout
      When I run "itos-cc mutation sample src/board.ts"
      Then no problem is reported
      And the exit code is 0
      # and a recorded timeout that is now killed agrees too

    @mutation-sample @ID-MUT-75
    Scenario: Only fresh mutants that ran are sampled
      Given "Board#place" has fresh results, with one mutant recorded uncovered
      And "Board#clear" changed since its results, and "Board#reset" has none
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then only the killed, timeout and survived mutants of "Board#place" run
      And no problem names "Board#clear" or "Board#reset"
      And the exit code is 0

    @mutation-sample @ID-MUT-76
    Scenario: --count says how many, 20 by default
      Given the selection holds 50 fresh mutants that ran, across several files
      When I run "itos-cc mutation sample"
      Then 20 of them run
      And "itos-cc mutation sample --count 5" runs 5
      And "itos-cc mutation sample --count 80" runs all 50

    @mutation-sample @ID-MUT-77
    Scenario: A rerun of one commit samples the same mutants
      Given the selection holds 50 fresh mutants that ran
      When I run "itos-cc mutation sample" twice at the same HEAD commit
      Then both runs sample the same mutants
      And stdout names the seed, the HEAD commit's id

    @mutation-sample @ID-MUT-78
    Scenario: --seed reproduces a run
      Given a run printed the seed "4813e48"
      And HEAD has moved since, with the same snapshots
      When I run "itos-cc mutation sample --seed 4813e48"
      Then it samples the mutants that run sampled

    @mutation-sample @ID-MUT-79
    Scenario: Outside a git repository the seed must be given
      Given the working directory is not a git repository
      When I run "itos-cc mutation sample"
      Then the problem is "sample.no-git"
      And the exit code is 82
      But "itos-cc mutation sample --seed 1" runs

    @mutation-sample @ID-MUT-80
    Scenario: A count below 1 is a usage error
      When I run "itos-cc mutation sample --count 0"
      Then the problem is "flags.value-invalid", with flag "--count" and value "0"
      And the exit code is 80

    @mutation-sample @ID-MUT-81
    Scenario: With --since only the functions the range changed are sampled
      Given only "Board#place" changed since "base", and a run since recorded its results
      And "Board#clear", unchanged, has fresh results too
      When I run "itos-cc mutation sample --since base --count 100"
      Then only mutants of "Board#place" run

    @mutation-sample @ID-MUT-82
    Scenario: Nothing to sample
      Given no fresh mutant that ran in the selection
      When I run "itos-cc mutation sample"
      Then stderr says "itos-cc: no cached mutant to sample"
      And no test command runs
      And the exit code is 0
      # stale or missing results are mutation check's to fail

    @mutation-sample @ID-MUT-83
    Scenario: A failing baseline
      Given the tests of src/board.ts fail without any mutation
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.baseline-failed", with file
      And none of its mutants runs
      And the exit code is 1

    # Since q-15 each sampled mutant also has "scope", the scope of the
    # tests it ran with, a key added with sample-recorded-scope.
    @mutation-sample @ID-MUT-84
    Scenario: The sample as JSON
      When I run "itos-cc mutation sample --json src/board.ts"
      Then stdout is one object with "schema": 1, "ok", "seed", and "files"
      And each file has "mutants", each sampled one with line, column, function, original, replacement, recorded, outcome, and scope
      And the problems are those the plain output prints

    # slice-50, flaky-kills' first item (q-93, the person 2026-10-24): a
    # test with unseeded randomness can kill a mutant in one run and miss it
    # in the next, so cd.yml's sample went red as mutation.mismatch with a
    # fix (re-run the cache) that could not help. Decided:
    # 1. A sampled mutant whose outcome disagrees with the one recorded runs
    #    twice more, in the same scope; a mutant that agrees runs once, as
    #    before. Flaky means both outcomes seen within this one invocation,
    #    on one tree: if a re-run agrees with the recording, neither outcome
    #    is trusted. If both re-runs repeat the new outcome, it is a
    #    mismatch, as before.
    # 2. Its own rule, mutation.flaky, exit 1, with the mismatch's fields and
    #    "outcomes", every outcome seen in run order; its fix says to make
    #    the tests deterministic (seed their randomness, settle their timing)
    #    and commit, naming no mutation run. Moving a confirmed flake off
    #    mutation.mismatch changes a rule value, so the feat is a
    #    BREAKING-CHANGE in a minor release.
    # 3. Killed, timeout and over-limit still agree with each other; a
    #    timeout against a survivor counts as a disagreement, so it can be
    #    flaky.
    # 4. No exception for a flaky mutant: mutation except already takes
    #    only a survivor. sample writes nothing, as before.
    # 5. Later items: run records a flaky key and check fails on it
    #    (flaky-kills-record); --repeat N (sample-repeat).
    # Tests flip on purpose through the harness, never through anything the
    # tests hash: in process, the declared Tests gain outcomes per trial,
    # cycled; through the binary, the fixture's control file outside the
    # project counts runs per mutant. Before this item MUT-337, MUT-338,
    # MUT-341 and MUT-342 fail at the mutation.flaky step, MUT-339 at the
    # ran-3-times step and MUT-340 at the ran-once step; MUT-72 and MUT-73
    # stay guardrails.
    @slice-50 @ID-MUT-337 @wip
    Scenario: A recorded kill its tests decide differently each run is flaky, not a mismatch
      Given the snapshot records the mutant `>` → `>=` at src/board.ts:5:9 in "Board#place" as killed
      But the tests miss it on one run and kill it on the next, with nothing they hash changed
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.flaky", with file, line, column, function, original, replacement, recorded "killed" and outcomes "survived", "killed"
      And no problem is "mutation.mismatch"
      And .metrics/mutate/src/board.ts.json and src/board.ts are unchanged
      And the exit code is 1

    @slice-50 @ID-MUT-338 @wip
    Scenario: A recorded survivor its tests decide differently each run is flaky too
      Given the snapshot records a mutant of "Board#place" as survived
      But the tests kill it on one run and miss it on the next, with nothing they hash changed
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.flaky", with recorded "survived" and outcomes "killed", "survived"
      And the exit code is 1

    @slice-50 @ID-MUT-339 @wip
    Scenario: A mismatch its re-runs repeat stays a mismatch
      Given the snapshot records the mutant `>` → `>=` at src/board.ts:5:9 in "Board#place" as killed
      But the tests no longer kill it
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.mismatch", with file, line, column, function, original, replacement, recorded "killed" and outcome "survived"
      And that mutant ran 3 times
      And the exit code is 1

    @slice-50 @ID-MUT-340 @wip
    Scenario: An outcome that agrees runs once
      Given fresh results for every function of src/board.ts, with killed mutants and one survivor
      And the tests still kill and miss the same mutants
      When I run "itos-cc mutation sample src/board.ts"
      Then each sampled mutant ran once
      And the exit code is 0

    @slice-50 @ID-MUT-341 @wip
    Scenario: A survivor that runs past its timeout on one run and survives the next is flaky
      Given the snapshot records a mutant of "Board#place" as survived
      But it runs past its timeout on one run and survives the next
      When I run "itos-cc mutation sample src/board.ts"
      Then the problem is "mutation.flaky", with recorded "survived" and outcomes "timeout", "survived"
      And the exit code is 1

    @slice-50 @ID-MUT-342 @wip
    Scenario: A flaky mutant in the plain output and as JSON
      Given the snapshot records the mutant `>` → `>=` at src/board.ts:5:9 in "Board#place" as killed
      But the tests miss it on one run and kill it on the next, with nothing they hash changed
      When I run "itos-cc mutation sample src/board.ts"
      Then stdout says "1 sampled, 0 mismatched, 1 flaky" and "flaky" with src/board.ts:5:9, Board#place and "recorded killed, now survived, killed"
      And with --json the sampled mutant has "outcomes": ["survived", "killed"]
      And the problem's fix says to make the tests deterministic and names no mutation run

    # A snapshot did not record which tests decided its outcomes, so
    # mutation sample, re-running with the file's own tests unless told
    # otherwise, read a kill only an end-to-end test makes under --all-tests
    # as a survivor: a mutation.mismatch that was no cache error. Decided
    # with the person on 2026-10-07 (q-15): each recorded outcome keeps its
    # scope, "own" (the file's own tests), "all-tests", or the
    # --test-command line, and a reused outcome keeps the scope it was
    # decided with. mutation sample re-runs each mutant with its recorded
    # scope; --all-tests or --test-command given to sample override it for
    # every mutant. An outcome with no recorded scope, written before this,
    # is taken as "own" (the coordinator's call: it is what a run without
    # those flags used).
    @sample-recorded-scope @ID-MUT-109
    Scenario: Each recorded outcome keeps the scope of the tests that decided it
      When I run "itos-cc mutation run --all-tests src/board.ts"
      Then each mutant in .metrics/mutate/src/board.ts.json records scope "all-tests"
      And with --test-command 'make test' each records "make test", and with neither each records "own"

    @sample-recorded-scope @ID-MUT-110
    Scenario: A reused outcome keeps its scope
      Given a run with --all-tests killed every mutant of "Board#place"
      And nothing has changed since
      When I run "itos-cc mutation run src/board.ts"
      Then the mutants of "Board#place" are reused and still record scope "all-tests"

    @sample-recorded-scope @ID-MUT-111
    Scenario: mutation sample re-runs each mutant with its recorded scope
      Given a run with --all-tests recorded a kill in src/board.ts that only an end-to-end test makes
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then that mutant runs the whole suite and is killed
      And no "mutation.mismatch" is reported

    @sample-recorded-scope @ID-MUT-112
    Scenario: A scope given to mutation sample overrides the recorded one
      Given outcomes of src/board.ts recorded with scope "all-tests"
      When I run "itos-cc mutation sample --test-command 'make test' src/board.ts"
      Then every sampled mutant runs "make test"

    @sample-recorded-scope @ID-MUT-113
    Scenario: An outcome with no recorded scope was decided by the file's own tests
      Given fresh results for src/board.ts written before outcomes recorded their scope
      When I run "itos-cc mutation sample src/board.ts"
      Then each sampled mutant runs the file's own tests

    # mismatch-fix-scope: since sample-recorded-scope each mutant is sampled
    # in a scope, but a mutation.mismatch's fix still said "itos-cc mutation
    # run --mutate-all <file>" with neither flag, so following it on an
    # all-tests outcome re-recorded it under the file's own tests, and a kill
    # only the whole suite makes became a survivor. Decided by the
    # coordinator on 2026-10-07, the person agreeing to the item:
    # - The fix names the scope the sampled mutant ran with (its recorded
    #   one, or the one given to sample): "own" and "listed" add no flag, as
    #   a run without one uses the file's own tests and then the listed
    #   tests; "all-tests" adds --all-tests; a --test-command line adds
    #   --test-command with the line in single quotes, a quote in it written
    #   '\''.
    # - The problem carries the scope as "scope", as the sample's JSON does.
    @mismatch-fix-scope @ID-MUT-147
    Scenario Outline: A mismatch's fix re-runs the mutant in the scope it was sampled with
      Given a run <recorded with> recorded a kill in src/board.ts that those tests no longer make
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then the problem is "mutation.mismatch", with scope "<scope>"
      And its fix says to run "<fix>"

      Examples:
        | recorded with                   | scope     | fix                                                                     |
        | without flags                   | own       | itos-cc mutation run --mutate-all src/board.ts                          |
        | with --all-tests                | all-tests | itos-cc mutation run --mutate-all --all-tests src/board.ts              |
        | with --test-command 'make test' | make test | itos-cc mutation run --mutate-all --test-command 'make test' src/board.ts |

    @mismatch-fix-scope @ID-MUT-148
    Scenario: A scope given to sample is the one the fix names
      Given outcomes of src/board.ts recorded with scope "own", one of them a kill the whole suite no longer makes
      When I run "itos-cc mutation sample --all-tests --count 100 src/board.ts"
      Then the problem is "mutation.mismatch", with scope "all-tests"
      And its fix says to run "itos-cc mutation run --mutate-all --all-tests src/board.ts"

    # slice-45, mismatch-fix-quotes (2026-10-24, a coordinator's call inside
    # the item): the fix wrapped its command in single quotes, so with a
    # --test-command scope it read "run 'itos-cc mutation run --mutate-all
    # --test-command 'make test' src/board.ts'", which no one can paste.
    # The fix now ends with the command after a colon, for every scope, with
    # no quotes of its own: "The cached results are not what the tests do:
    # commit .metrics/mutate/ with the code after running: <command>". The
    # step "its fix says to run" of MUT-147 and MUT-148 checks that ending in
    # the feat, its sentence unchanged. Before this item the fix's command
    # sat inside quotes, failing at the ending step.
    @slice-45 @ID-MUT-334
    Scenario: A mismatch's fix ends with its command, which pastes as written
      Given a run with --test-command 'make test' recorded a kill in src/board.ts that those tests no longer make
      When I run "itos-cc mutation sample --count 100 src/board.ts"
      Then the problem's fix ends with ": itos-cc mutation run --mutate-all --test-command 'make test' src/board.ts"
      And the fix holds no quote but the two around make test

  Rule: Results

    @ID-MUT-28
    Scenario: Summary per file
      When I run "itos-cc mutation run src/board.ts"
      Then stdout says "src/board.ts: 14 killed, 1 survived, 2 uncovered (ran 7, reused 8)"
      And each survivor is listed as "survived src/board.ts:5:9 `>` → `>=` in board#place"

    @ID-MUT-29
    Scenario Outline: Exit codes
      Given <situation>
      When I run "itos-cc mutation run"
      Then the exit code is <code>

      Examples:
        | situation                     | code |
        | every covered mutant is killed | 0    |
        | a baseline fails              | 1    |
        | a mutant survives             | 1    |

    @ID-MUT-30
    Scenario: Survivors as JSON
      When I run "itos-cc mutation run --json src/board.ts"
      Then stdout is one object with "schema": 1, "ok", and "files", each with file, killed, survived, uncovered, ran, reused, and baseline
      And each survivor is a "mutation.survived" problem with file, line, column, function, original, and replacement

    # Issue #9, part 3: a gate reads mutation run's result without parsing text or
    # the cache files, so --json lists every mutant it decided. Each file's
    # "mutants" are in site order, with the keys mutation list gives a site less
    # "file" (its file holds them), plus "outcome" and "reused": "reused" is
    # true for an outcome taken from the snapshot without running, which a
    # gate that trusts cached kills (#8) needs to tell apart. A new key, so
    # "schema" stays 1. With --since only the judged functions' mutants are
    # listed, as only they are counted. Since q-15 each mutant also has
    # "scope", the scope of the tests that decided it, a key added with
    # sample-recorded-scope.
    @slice-3 @ID-MUT-52
    Scenario: Every mutant as JSON
      Given src/board.ts has mutants that are killed, one that survives, and one on a line no test executes
      When I run "itos-cc mutation run --json src/board.ts"
      Then its file in "files" has "mutants", one for each site in site order
      And each has line, column, function, original, replacement, outcome, reused, and scope
      And their outcomes are "killed", "survived", and "uncovered" as each was decided

    @slice-3 @ID-MUT-53
    Scenario: A timed-out mutant's outcome is its own
      Given a mutant of src/board.ts runs past its timeout
      When I run "itos-cc mutation run --json src/board.ts"
      Then its outcome is "timeout"
      And it is counted in "killed"

    @slice-3 @ID-MUT-54
    Scenario: Mutants taken from the snapshot say so
      Given a previous run killed every mutant of "Board#place"
      And "Board#place" has not changed since, while "Board#clear" has
      When I run "itos-cc mutation run --json src/board.ts"
      Then the mutants of "Board#place" are "killed" with reused true
      And the mutants of "Board#clear" have reused false

    @slice-3 @ID-MUT-55
    Scenario: With --since only the judged functions' mutants are listed
      Given only "Board#place" changed since "base"
      When I run "itos-cc mutation run --since base --json"
      Then the "mutants" of src/board.ts all have function "Board#place"

    @slice-3 @ID-MUT-56
    Scenario: A failing baseline lists no mutant
      Given the tests of src/board.ts fail without any mutation
      When I run "itos-cc mutation run --json src/board.ts"
      Then its file has "baseline": "failed" and "mutants": []

    @ID-MUT-31
    Scenario: Results are cached per file
      When I run "itos-cc mutation run src/billing/invoice.ts"
      Then .metrics/mutate/src/billing/invoice.ts.json records each function's hash and the outcome of every mutant

    @ID-MUT-32
    Scenario: A summary comment at the end of each source file
      Given the file "x.py":
        """
        def a(x):
            return x > 0
        """
      When its mutants run and "> → >=" survives while one other mutant is killed
      Then the file ends with:
        """
        # itos-cc mutation: 1 killed, 1 survived, 0 uncovered
        # survived: line 2 `>` → `>=` in a
        # end itos-cc mutation
        """

    @ID-MUT-33
    Scenario: The summary comment is replaced, not repeated
      Given a file that already ends with an itos-cc mutation comment
      When its mutants run again
      Then the file holds exactly one, updated summary comment
      And the file is only written when the summary changed

    @ID-MUT-34
    Scenario: A comment block followed by code is not ours
      Given an itos-cc mutation comment block followed by more code
      When the summary is rewritten
      Then that block is left in place

    @ID-MUT-35
    Scenario: Turning the summary comment off
      When I run "itos-cc mutation run --no-annotate"
      Then no source file is modified

    # annotate-marker: the summary comment opened "itos-cc mutate:" and
    # closed "end itos-cc mutate", naming the mutate command that became
    # mutation run. Decided by the coordinator on 2026-10-07: it opens
    # "itos-cc mutation:" and closes "end itos-cc mutation", and a run still
    # finds a comment with the old markers, so a file annotated before is
    # rewritten with one comment, never two. ID-MUT-32 and the scenarios of
    # annotate-excepted quote the old text and change with the fix.
    @annotate-marker @ID-MUT-145
    Scenario: A summary comment with the old markers is replaced by one with the new
      Given the file "x.py" ends with a summary comment opening "# itos-cc mutate:" and closing "# end itos-cc mutate"
      When its mutants run again
      Then the file holds exactly one summary comment
      And it opens "# itos-cc mutation:" and closes "# end itos-cc mutation"

    @ID-MUT-36
    Scenario: Nothing to mutate
      Given the selection holds no production source files
      When I run "itos-cc mutation run"
      Then stderr says "itos-cc: no source files to mutate"
      And the exit code is 0

  # Issue #10: an equivalent mutant, one that changes no behaviour (`<` to
  # `!=` on a loop that only counts up, a `0` never read), survives every
  # run, so a gate on survivors blocks forever, and the next change contorts
  # the code or tests an implementation detail to kill it. Decided with the
  # person on 2026-10-07 (q-9 to q-12):
  # - The exceptions live in itos-cc.yaml at the project root, under
  #   mutation.exceptions: itos-cc's first project setting (docs/CLI.md
  #   rule 38), reviewed like any change to the code.
  # - itos-cc mutation except <file>:<line>:<column> --reason '…' writes an
  #   entry for a survivor its fresh snapshot records: the file, the
  #   function, the function's hash, the site's place within the function
  #   (its line counted from the function's first, so a move keeps it), the
  #   original, the replacement and the reason. Excepting a site again
  #   replaces its entry; the file's other keys and comments are kept.
  # - An excepted survivor fails nothing: it is counted "excepted", not
  #   "survived". It is reused without running, as a kill is, while its
  #   function and the tests that import its file are unchanged; when its
  #   tests change it runs again, and if it is now killed its entry is
  #   stale. An entry whose function changed, or whose site is gone, is
  #   stale without running anything, and its mutant is judged as if it
  #   had no entry. A stale entry fails as mutation.exception-stale until
  #   it is removed or the site excepted again. --mutate-all runs excepted
  #   mutants too.
  # - Only survivors: an uncovered mutant needs a test, not a reason.
  # mutation check gives the same verdict from the snapshots. mutation
  # sample needs nothing new: an excepted survivor still survives, which
  # matches what its snapshot records.
  Rule: Equivalent mutants excepted, each with a reason

    @mutation-exceptions @ID-MUT-85
    Scenario: Excepting a survivor records it with its reason
      Given a fresh run recorded the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'the loop only counts up'"
      Then itos-cc.yaml has one entry under mutation.exceptions
      And it records src/board.ts, "Board#count", the function's hash, the site's line within the function, its column, `<`, `<=`, and the reason
      And the exit code is 0

    @mutation-exceptions @ID-MUT-86
    Scenario: Only a recorded survivor can be excepted
      Given the snapshot records the mutant at src/board.ts:5:9 as killed, and the one at src/board.ts:9:3 as uncovered
      When I run "itos-cc mutation except src/board.ts:5:9 --reason 'x'"
      Then the problem is "exception.no-survivor", with file, line and column
      And the exit code is 80
      And excepting src/board.ts:9:3 is refused the same way

    # slice-23, issue #58: a changed line that no test can reach fails
    # --fail-uncovered=lines, and mutation except refuses its site, so the
    # refusal read as a gap in the tool. Decided by the person on
    # 2026-10-18: keep q-12, an exception covers survivors only; the
    # refusal, still exception.no-survivor, says that this is on purpose and
    # suggests routing the branch through code a test already reaches.
    # Coordinator's call on the wording, 2026-10-21: the message ends "and an
    # exception covers survivors only, on purpose: an uncovered line needs a
    # test, not a reason"; the fix reads "Add a test that runs line 9, or
    # route the branch through code a test already reaches, such as a
    # helper the tests call; then run 'itos-cc mutation run src/board.ts'."
    # A killed site, or a function with stale or missing results, is worded
    # as before. Both drivers run it, as it needs no real tool. Before
    # slice-23 the message says only that a survivor is excepted, and the fix
    # asks for a site listed as survived, failing at the second Then.
    @slice-23 @ID-MUT-299
    Scenario: Refusing to except an uncovered site says it is on purpose and names the way out
      Given the snapshot records the mutant at src/board.ts:5:9 as killed, and the one at src/board.ts:9:3 as uncovered
      When I run "itos-cc mutation except src/board.ts:9:3 --reason 'no test can reach it'"
      Then the problem is "exception.no-survivor", with file "src/board.ts"
      And its message says an exception covers survivors only, on purpose
      And its fix names a test that runs line 9, or routing the branch through code a test already reaches
      And the exit code is 80
      But refusing the killed site at src/board.ts:5:9 says neither

    @mutation-exceptions @ID-MUT-87
    Scenario: A reason is required
      When I run "itos-cc mutation except src/board.ts:7:19"
      Then the problem is "flags.value-missing", with flag "--reason"
      And the exit code is 80
      And "--reason ''" is "flags.value-invalid", exit 80

    @mutation-exceptions @ID-MUT-88
    Scenario: The rest of itos-cc.yaml is kept
      Given itos-cc.yaml holds a comment, another key, and an entry for src/board.ts:7:19
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'reworded'"
      Then the entry's reason is "reworded", and it is still the only one for that site
      And the comment and the other key are unchanged

    # slice-20, issue #55: mutation except decoded itos-cc.yaml and wrote the
    # whole document back through yaml.v3, so lines it never meant to touch
    # changed: a flow mapping such as mutation.tests' join lost the spaces
    # inside its braces, blank lines and a leading "---" were dropped, a list
    # at its key's indentation moved two spaces in, a block four spaces deep
    # came back two, the spaces before a trailing comment became one, and a
    # folded reason was joined onto one line. The commit that records an
    # exception should show only the exception: a restyled line elsewhere has
    # to be found and put back by hand, and is easy to miss in review.
    # Decided in the issue: every line outside the edited exceptions list
    # comes back byte for byte. Coordinator's calls, 2026-10-21, no
    # library keeping a YAML file's bytes (yaml.v3, goccy/go-yaml and kyaml
    # all re-render), the entry's text is spliced into the file at the
    # line and column yaml.v3 gives the list and the entry:
    # - mutation except changes only the lines of the entry it adds or
    #   replaces, so the list's other entries are byte for byte too.
    # - A new entry is written in block style, its keys in today's order,
    #   its dash at the indentation of the list's other entries, or two
    #   spaces under its key when it has none, after the list's last entry.
    #   A flow-style entry is replaced by a block one; a flow-style list is
    #   rewritten whole, as splicing into it is fragile.
    #   A replaced entry is written the same way, in its place, keeping the
    #   comment above it and the one after its first line.
    # - A file with no list gains the keys it needs at the end of the
    #   mutation mapping, or of the document; nothing before them changes.
    # - --renew (MUT-292) and --prune (slice-21) write the same way.
    # The fixture's "other" key stands for mutation.tests' join, which would
    # make the run use listed tests. Both drivers run it, as it needs no real
    # tool. Before slice-20 the whole document is re-encoded, failing at the
    # Then.
    @slice-20 @ID-MUT-290
    Scenario: Excepting a survivor changes no byte of itos-cc.yaml outside its new entry
      Given a fresh run recorded the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      And itos-cc.yaml is laid out by hand: a "---" line, blank lines between keys, `other: { each: "{id}", sep: "|" }`, a block four spaces deep, a comment two spaces after a value, and an exceptions list at its key's indentation with one entry for another function, whose reason is folded
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'the loop only counts up'"
      Then itos-cc.yaml is the text it was, with the new entry's lines after the last entry and no other byte changed
      And the exit code is 0

    @slice-20 @ID-MUT-291
    Scenario: Excepting a site again changes only the lines of its entry
      Given a fresh run recorded the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      And itos-cc.yaml is laid out by hand, its entry for src/board.ts:7:19 with reason "first" between two entries for other functions, a comment above it
      When I run "itos-cc mutation except src/board.ts:7:19 --reason 'reworded'"
      Then only the lines of that entry changed in itos-cc.yaml, its reason now "reworded" and the comment above it kept
      And the exit code is 0

    @mutation-exceptions @ID-MUT-89
    Scenario: An excepted survivor fails nothing
      Given itos-cc.yaml excepts the survivor of "Board#count", and every other mutant of src/board.ts is killed
      When I run "itos-cc mutation run src/board.ts"
      Then stdout says "src/board.ts: 14 killed, 0 survived, 1 excepted, 0 uncovered (ran 15, reused 0)"
      And the exit code is 0

    @mutation-exceptions @ID-MUT-90
    Scenario: An excepted survivor is reused while nothing changed
      Given a run recorded the excepted survivor of "Board#count"
      And neither "Board#count" nor the tests that import src/board.ts have changed since
      When I run "itos-cc mutation run src/board.ts"
      Then the excepted mutant does not run
      And it is counted "excepted"
      But "itos-cc mutation run --mutate-all src/board.ts" runs it

    @mutation-exceptions @ID-MUT-91
    Scenario: After its tests change, an excepted mutant that still survives stays excepted
      Given a run recorded the excepted survivor of "Board#count"
      And src/board.test.ts changed since, and still does not kill it
      When I run "itos-cc mutation run src/board.ts"
      Then the excepted mutant runs
      And it is counted "excepted"
      And the exit code is 0

    @mutation-exceptions @ID-MUT-92
    Scenario: An excepted mutant that is now killed makes its entry stale
      Given a run recorded the excepted survivor of "Board#count"
      And src/board.test.ts changed since, and now kills it
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with file, function, line, column, original, replacement and why "killed"
      And the exit code is 1

    @mutation-exceptions @ID-MUT-93
    Scenario: An entry whose function changed is stale
      Given itos-cc.yaml excepts a survivor of "Board#count"
      And "Board#count" changed since the entry was written
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with why "changed"
      And the mutant at its site, if it survives, is a "mutation.survived" problem too
      And the exit code is 1
      # moving the function is not a change, as for reuse

    @mutation-exceptions @ID-MUT-94
    Scenario: An entry whose site is gone is stale
      Given itos-cc.yaml excepts a survivor in "Board#count"
      And "Board#count" was deleted, or no longer has that site
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "mutation.exception-stale", with why "gone"
      And the exit code is 1

    @mutation-exceptions @ID-MUT-95
    Scenario: An entry does not excuse an uncovered mutant
      Given itos-cc.yaml holds an entry for a site whose mutant is uncovered
      When I run "itos-cc mutation run --fail-uncovered src/board.ts"
      Then the problem is "mutation.uncovered" for that site
      And the exit code is 1

    @mutation-exceptions @ID-MUT-96
    Scenario: mutation check passes an excepted survivor and fails a stale entry
      Given fresh results for src/board.ts with one survivor, which itos-cc.yaml excepts
      When I run "itos-cc mutation check src/board.ts"
      Then the exit code is 0
      But after "Board#count" changes, mutation check reports "mutation.exception-stale" and exits 1

    @mutation-exceptions @ID-MUT-97
    Scenario: With --since only the judged functions' entries are checked
      Given only "Board#place" changed since "base"
      And itos-cc.yaml holds an entry for "Board#count" that would be stale
      When I run "itos-cc mutation run --since base"
      Then no "mutation.exception-stale" problem is reported

    @mutation-exceptions @ID-MUT-98
    Scenario: An invalid itos-cc.yaml is a config error
      Given itos-cc.yaml holds an entry with no reason
      When I run "itos-cc mutation run src/board.ts"
      Then the problem is "config.invalid", with file "itos-cc.yaml"
      And the exit code is 80

    @mutation-exceptions @ID-MUT-99
    Scenario: Excepted mutants as JSON
      When I run "itos-cc mutation run --json src/board.ts" with one excepted survivor
      Then its file has "excepted": 1 beside "killed", "survived" and "uncovered"
      And that mutant has outcome "survived" and "excepted", the entry's reason

    # annotate-excepted: the summary comment at the end of a source
    # file is built from the snapshot alone, which records an excepted
    # survivor as survived, so the comment said "1 survived" and listed it
    # while mutation run's summary counted it excepted and failed nothing.
    # Decided on 2026-10-07:
    # - The comment counts "excepted" apart, only where there is one, as the
    #   summary line does, and lists each excepted survivor after the
    #   survivors, with its reason.
    # - A survivor is excepted in the comment when itos-cc.yaml holds an
    #   entry for its site that holds: the same function hash, the same site.
    #   That needs no run, so a --since run gives the functions it does not
    #   judge the same comment as a full run. A stale entry excepts nothing:
    #   its survivor is listed as survived.
    # - The comment's text is otherwise unchanged; its marker is
    #   mutation-annotate-marker's.
    # - A reason is written on one line, each run of whitespace one space, so
    #   the comment never puts uncommented text in a source file.
    @annotate-excepted @ID-MUT-138
    Scenario: The summary comment counts an excepted survivor apart, with its reason
      Given itos-cc.yaml excepts the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count" with reason "the loop only counts up", and every other mutant of src/board.ts is killed
      When I run "itos-cc mutation run src/board.ts"
      Then src/board.ts ends with:
        """
        // itos-cc mutation: 14 killed, 0 survived, 1 excepted, 0 uncovered
        // excepted: line 7 `<` → `<=` in Board#count: the loop only counts up
        // end itos-cc mutation
        """

    # slice-46, annotate-function-name (issue #10; 2026-10-24, a
    # coordinator's call inside the item): the summary comment named a
    # function by its bare name, so a method count of class Board read "in
    # count" and two classes' count methods read alike, while mutation run's
    # output, itos-cc.yaml and the scenarios name it Board#count. The
    # comment's survived and excepted lines now name a method by its class
    # within the file and its name (Board#count, Outer.Inner#name for a
    # nested class), as itos-cc.yaml and the scenarios do, and a top-level
    # function by its bare name; the file's own namespace is left out, so
    # the comment reads shorter than mutation run's full IDs
    # (board.Board#count). The feat changes MUT-138's expected line to "in
    # Board#count", saying so, and MUT-140's, which quotes the same line;
    # MUT-32's "in a" stays, a top-level function.
    # Before this item the line read "in count", failing at the Then.
    @slice-46 @ID-MUT-335
    Scenario: The summary comment names a method with its class
      Given the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count", and every other mutant of src/board.ts is killed
      When I run "itos-cc mutation run src/board.ts"
      Then src/board.ts ends with:
        """
        // itos-cc mutation: 14 killed, 1 survived, 0 uncovered
        // survived: line 7 `<` → `<=` in Board#count
        // end itos-cc mutation
        """

    @annotate-excepted @ID-MUT-139
    Scenario: A stale entry's survivor is listed as survived
      Given itos-cc.yaml excepts a survivor of "Board#count"
      And "Board#count" changed since the entry was written, and the mutant at its site still survives
      When I run "itos-cc mutation run src/board.ts"
      Then the summary comment of src/board.ts counts it as survived and lists it on a "survived:" line
      And the comment holds no "excepted" count and no "excepted:" line

    @annotate-excepted @ID-MUT-140
    Scenario: A reason over several lines is written on one comment line
      Given itos-cc.yaml excepts the survivor of "Board#count" with reason "the loop\n  only counts up"
      When I run "itos-cc mutation run src/board.ts"
      Then the summary comment of src/board.ts has the line "// excepted: line 7 `<` → `<=` in Board#count: the loop only counts up"
      And every line of the comment begins with "//"

    # exceptions-deleted-file: mutation run and mutation check judge the
    # exceptions of the files they select, so an entry whose function or
    # site is gone fails as stale, but one whose whole file was deleted or
    # renamed is never selected and stays in itos-cc.yaml unnoticed.
    # Decided with the person on 2026-10-07 (q-22):
    # - A run or check of the whole project (no paths, no --changed, no
    #   --since) judges each entry whose file is not a source it selects.
    # - With --since, so does a run whose range deleted or renamed the
    #   entry's file: a gate judges a task with --since, and the task that
    #   removed the file is the one to fix its entries.
    # - Such an entry has moved when exactly one selected source holds a
    #   function of the entry's name and hash: it still excepts the mutant at
    #   its site there, so that mutant is no survivor, and it fails as
    #   mutation.exception-stale with why "moved" and the new file, the fix
    #   saying to change the entry's file to it. Otherwise it fails as why
    #   "gone", as an entry whose function is gone fails: no line.
    # - Only mutation except writes itos-cc.yaml: a run never moves an entry.
    # - With paths or --changed, an entry for a file outside the selection
    #   is not judged, as now.
    @exceptions-deleted-file @ID-MUT-141
    Scenario: A run of the whole project fails an entry whose file is gone
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And src/board.ts and its tests were deleted
      When I run "itos-cc mutation run"
      Then the problem is "mutation.exception-stale", with file "src/board.ts", function "Board#count", no line, and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-142
    Scenario: mutation check of the whole project fails it too
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And fresh results for every source file, src/board.ts having been deleted
      When I run "itos-cc mutation check"
      Then the problem is "mutation.exception-stale", with file "src/board.ts" and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-143
    Scenario: With --since, an entry whose file the range deleted is gone
      Given itos-cc.yaml excepts a survivor of "Board#count" in src/board.ts
      And a commit after "base" deleted src/board.ts and its tests
      When I run "itos-cc mutation run --since base"
      Then the problem is "mutation.exception-stale", with file "src/board.ts" and why "gone"
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-144
    Scenario: An entry whose file was renamed has moved, and still excepts its mutant
      Given itos-cc.yaml excepts the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      And a commit after "base" renamed src/board.ts to src/grid.ts, changing nothing else
      When I run "itos-cc mutation run --since base"
      Then the problem is "mutation.exception-stale", with file "src/board.ts", why "moved", and new file "src/grid.ts"
      And its fix says to change the entry's file to src/grid.ts
      And there is no "mutation.survived" problem
      And itos-cc.yaml still names src/board.ts
      And the exit code is 1

    @exceptions-deleted-file @ID-MUT-146
    Scenario: A run of the whole project finds a renamed file's entry moved
      Given itos-cc.yaml excepts the survivor of "Board#count" in src/board.ts
      And src/board.ts was renamed to src/grid.ts, changing nothing else
      When I run "itos-cc mutation run"
      Then the problem is "mutation.exception-stale", with why "moved" and new file "src/grid.ts"
      And there is no "mutation.survived" problem

  # all-tests-freshness, issue #20, q-24 and ADR-0016 (2026-10-08):
  # - Importer hashes remain every outcome's baseline dependency.
  # - Each Go outcome recorded with scope all-tests or test-command also
  #   depends on every _test.go file beneath its source's nearest Go module
  #   root, excluding nested modules but including build-tagged tests.
  #   Discovery must not silently omit a module's test files merely because
  #   they live outside packages that import the source.
  # - Both broad scopes also depend on project-root mutation.tests.support
  #   matches, using the existing filepath.Glob semantics. Feature files and
  #   other custom-command inputs require support globs; arbitrary external
  #   inputs are not inferred. Own/listed scopes and other languages retain
  #   their existing rules. No list command is needed to check freshness.
  # - These dependencies apply to every broad-scope outcome, including
  #   survivors and excepted survivors, not just kills. A changed, added or
  #   deleted dependency invalidates the outcome; check names the file.
  # - Freshness follows the recorded scope even when a later invocation
  #   uses another scope. Reuse retains that scope and its evidence. Partial
  #   runs never refresh evidence for outcomes they did not rerun.
  # - Legacy broad-scope outcomes lacking this evidence are stale, even
  #   when their source and importer hashes still match, and must rerun.
  # - Run, check, sample and the graph share the same freshness verdict.
  Rule: Go whole-suite outcomes notice changes to their test inputs

    @all-tests-freshness @ID-MUT-150
    Scenario Outline: A binary-only test change makes a whole-suite kill stale
      Given a Go module with a production function and a separate end-to-end _test.go file that runs its built binary without importing its package
      And a mutation run with <scope> recorded a mutant killed only by that end-to-end test
      And only that test file changed since, so it no longer kills the mutant
      When I run "itos-cc mutation check --json" for the production file
      Then its function is stale, the problem is "mutation.stale", and its message names the changed end-to-end test file
      And the exit code is 1
      When I run mutation run with <scope> for the production file
      Then that mutant runs again rather than reusing its recorded kill
      And it survives

      Examples:
        | scope                         |
        | --all-tests                   |
        | --test-command "go test -count=1 ./..." |

    @all-tests-freshness @ID-MUT-151
    Scenario Outline: Whole-suite outcomes notice support files added, changed or removed
      Given a Go whole-suite outcome recorded with <scope> and support globs matching a feature file
      And a support file was <change> since that outcome was recorded
      When I run "itos-cc mutation check --json" for its production file
      Then the function is stale and its "mutation.stale" message names the support file
      And no test or list command runs
      And the exit code is 1

      Examples:
        | scope        | change  |
        | all-tests    | added   |
        | all-tests    | changed |
        | all-tests    | removed |
        | test-command | added   |
        | test-command | changed |
        | test-command | removed |

    @all-tests-freshness @ID-MUT-152
    Scenario: Unchanged broad-scope evidence permits a plain run to reuse a kill
      Given Go kills recorded with all-tests and test-command scopes and current module-test and support hashes
      And none of the source, importer tests, module tests or support files changed
      When I run mutation run without --all-tests or --test-command
      Then those kills are reused
      And each retains its recorded scope and freshness evidence
      And mutation check still reports them fresh

    @all-tests-freshness @ID-MUT-153
    Scenario: Mixed scopes keep their own freshness dependencies
      Given Go mutation outcomes recorded with own, listed, all-tests and test-command scopes
      And importer tests, listed covering tests and listed support files are unchanged
      And only a non-importing module _test.go file changed
      When their freshness is checked
      Then the own and listed outcomes remain usable
      And every all-tests and test-command outcome is stale, including a survivor and an excepted survivor
      And mutation check and the graph agree on each affected function's freshness

    @all-tests-freshness @ID-MUT-154
    Scenario: A partial run cannot bless unjudged whole-suite outcomes
      Given a Go file with two functions whose outcomes were recorded with whole-suite scopes
      And a non-importing module test changed since both outcomes were recorded
      And only one function changed since "base"
      When I run "itos-cc mutation run --since base" for that file
      Then the judged function's outcomes are rerun
      And the unjudged function's outcomes remain stale
      And mutation check reports the unjudged function stale
      And mutation sample excludes its stale outcomes
      And the graph marks the unjudged function stale too

    @all-tests-freshness @ID-MUT-155
    Scenario: Module boundaries and legacy evidence are explicit
      Given a Go broad-scope kill whose source and importer hashes match but whose snapshot records no whole-suite freshness evidence
      When I run mutation check for its production file
      Then its function is stale and mutation run must rerun that outcome before it can be reused
      Given a fresh broad-scope outcome in the outer Go module
      When a build-tagged _test.go file is added beneath that module
      Then that outcome is stale
      But adding a _test.go file only beneath a nested Go module leaves that outcome fresh

    # slice-12, issue #40: a run with --since keeps what still holds of the
    # snapshot for the functions it does not judge, and an unjudged function
    # the snapshot holds no entry for gets none. Marking the kept entries
    # stale when a whole-suite input changed then read them by the file's own
    # function order, so a missing entry shifted every later one: the run
    # panicked ("index out of range") when the last kept entry moved past
    # the end, and otherwise marked the wrong function stale, a function it
    # had just judged among them. Issue #40's releases exited 70 on that
    # panic; today's panic handler exits 100, the internal-error exit. Each kept entry is marked by
    # the function it belongs to. The first function's missing entry is
    # written as raw snapshot JSON, which no run of today's product leaves
    # behind on its own; issue #40's came from an older itos-cc's snapshot.
    @slice-12 @ID-MUT-260
    Scenario: A run since a commit marks the unjudged function stale, past a function the snapshot never recorded
      Given a Go file with three functions, its snapshot holding whole-suite outcomes for the second and third and no entry for the first
      And a non-importing module test changed since "base"
      And only the second function changed since "base"
      When I run "itos-cc mutation run --since base --all-tests" for that file
      Then the exit code is not 100 and the output names no panic
      And the second function's outcomes are rerun
      And mutation check reports the third function stale and the second fresh
      And the snapshot still holds no entry for the first function

    @slice-12 @ID-MUT-261
    Scenario: A run since a commit leaves the function it judged fresh, past a function the snapshot never recorded
      Given a Go file with three functions, its snapshot holding whole-suite outcomes for the second and third and no entry for the first
      And a non-importing module test changed since "base"
      And only the third function changed since "base"
      When I run "itos-cc mutation run --since base --all-tests" for that file
      Then the third function's outcomes are rerun
      And mutation check reports the second function stale and the third fresh
      And the snapshot still holds no entry for the first function

  # Language parity with Go (the person, 2026-10-10: implement what exists
  # only for Go for TypeScript, Python and Kotlin too). These three slices
  # need no new toolchain in CI: their fixtures use --test-command scripts,
  # missing tools or plain files. The Go precedent decides their substance.
  # - strict-mixed-languages: strict Go coverage judges Go files only. A
  #   TypeScript, Python or Kotlin file in the same selection is outside the
  #   Go evidence inventory: it is never "missing" Go evidence nor an error
  #   for lack of a go.mod, and keeps whatever non-Go --fail-uncovered rules
  #   apply. Since strict-lines-typescript (q-40), that is TypeScript's own
  #   line evidence, missing where no TypeScript tool measures it.
  # - uncovered-fails-closed: under --fail-uncovered, a TypeScript, Python
  #   or Kotlin coverage plan that is unsupported (its tool missing), writes
  #   no report, or whose command fails is a measurement failure, reported
  #   with the existing coverage.* problems and their exit categories, and
  #   no mutant of that language is judged, as ADR-0017 makes Go fail closed.
  #   Without --fail-uncovered the current fallback (run every mutant) stays.
  #   Raw-report flags keep their behaviour for non-Go languages until strict
  #   statement evidence exists for them. README's "for non-strict runs"
  #   sentence changes accordingly.
  # - whole-suite-freshness-all: ADR-0016 for every language. An outcome
  #   recorded with all-tests or test-command scope also depends on every
  #   test file beneath its source's build root, as project discovery
  #   classifies tests, plus mutation.tests.support matches. Build roots:
  #   TypeScript the nearest package.json (excluding nested package.json
  #   trees and node_modules); Python the nearest pyproject.toml, setup.py
  #   or setup.cfg (excluding .venv, including conftest.py); Kotlin the
  #   Gradle build root (settings.gradle or settings.gradle.kts) or the
  #   Maven module. New outcomes record this evidence under one
  #   language-neutral snapshot key; Go's existing evidence key is read as
  #   equivalent, so no Go outcome goes stale from the rename. Non-Go
  #   whole-suite outcomes recorded before this lack the evidence and are
  #   stale once, as legacy Go ones were. Run, check, sample and the graph
  #   share the verdict. Own and listed scopes keep their rules.
  Rule: Language parity with Go for strict coverage and whole-suite freshness

    @strict-mixed-languages @ID-MUT-203
    Scenario: Strict Go coverage ignores the other languages of a mixed selection
      Given a project with a Go module and a TypeScript file that has no go.mod above it, and no TypeScript tool
      And the Go functions have fresh complete coverage evidence
      When I run "itos-cc mutation check --fail-uncovered --json" over both files
      Then the TypeScript function is reported as mutation.coverage-missing of TypeScript line evidence, never of Go evidence, and nothing is an error
      And the Go functions are judged as before
      When a TypeScript file sits beneath the Go module's directory
      Then it is still not judged for Go coverage evidence
      And a strict mutation run over a mixed selection can reuse fresh Go evidence instead of measuring Go every time

    @uncovered-fails-closed @ID-MUT-204
    Scenario Outline: Strict runs fail closed when another language measured nothing
      Given a <language> project whose coverage <failure>
      When I run "itos-cc mutation run --fail-uncovered --json" for one of its files
      Then the run fails with "<rule>" naming the language and directory, and the exit code is <exit>
      And no mutant of that file is judged or reported as covered
      But without --fail-uncovered the run falls back to running every mutant as before

      Examples:
        | language   | failure                              | rule                      | exit |
        | TypeScript | tool is missing                      | coverage.tool-missing     | 82   |
        | Python     | command exits 0 but writes no report | coverage.measured-nothing | 1    |
        | Kotlin     | command fails                        | coverage.measured-nothing | 1    |

      # slice-36, dart-support: a Flutter package fails closed as the others
      # do. It already did before Dart was strict, through the non-strict
      # --fail-uncovered path (run.go's unmeasuredToJudge), so this row is a
      # guardrail: it passes before slice-36 and holds that strict Dart
      # keeps failing closed.
      @slice-36
      Examples:
        | language | failure         | rule                  | exit |
        | Dart     | tool is missing | coverage.tool-missing | 82   |

    # slice-16, issue #46: under --fail-uncovered a coverage command that
    # fails proves nothing, even after writing its report, as
    # uncovered-fails-closed decided above: its build root is
    # coverage.measured-nothing, its message naming the command and its
    # exit status, and no mutant of it is judged. crap, and a run without
    # --fail-uncovered, still read what such a report measured (ID-COV-13).
    # Before slice-16 a failed run whose report named the file read as
    # measured and left its functions without proof, each
    # mutation.coverage-missing, whose fix asks for a test. A test that
    # fails only under coverage.py stands for any failure the command line
    # cannot clear, such as a Vitest threshold for a glob pattern. Both
    # drivers run it, but only the binary driver shows it red: the fake
    # measurer already reads a failed command as measuring nothing.
    @slice-16 @ID-MUT-270
    Scenario: A strict run whose coverage command fails after writing its report measured nothing
      Given a Python project whose test fails only when coverage.py runs it, after its file's every line has run
      When I run "itos-cc mutation run --fail-uncovered --json" for that file
      Then the run fails with "coverage.measured-nothing" naming the language and directory, and the exit code is 1
      And no mutant of that file is judged, and no problem is "mutation.coverage-missing"

    @whole-suite-freshness-all @ID-MUT-205
    Scenario Outline: A non-importing test change makes another language's whole-suite outcome stale
      Given a <language> project whose test <test> does not import the production file
      And a mutation run with <scope> recorded an outcome for that file with whole-suite evidence
      And only that test file changed since
      When I run "itos-cc mutation check --json" for the production file
      Then its function is stale and the "mutation.stale" message names the changed test file
      And no test, list or coverage command runs
      When I run mutation run with <scope> for the production file
      Then the outcome runs again rather than being reused

      Examples:
        | language   | test                    | scope                       |
        | TypeScript | e2e/cli.test.ts         | --all-tests                 |
        | Python     | tests/test_cli.py       | --test-command "./run.sh"   |
        | Kotlin     | src/test/kotlin/CliTest.kt | --all-tests              |

      # slice-36, dart-support: a Dart outcome's whole suite is every test
      # file beneath its nearest pubspec.yaml, a nested package's left to
      # it; an --all-tests one rests on those under its test/ folder, as
      # flutter test with no path runs them (slice-38). The non-importing
      # test runs the package's bin/ entry point with dart run, since
      # flutter_tester has no dart:mirrors to call a library it does not
      # import. Before slice-36 a Dart whole-suite outcome recorded no
      # test, failing at the Given's check that its evidence names
      # test/cli_test.dart.
      @slice-36
      Examples:
        | language | test               | scope       |
        | Dart     | test/cli_test.dart | --all-tests |

    @whole-suite-freshness-all @ID-MUT-206
    Scenario: Build roots, support files and legacy evidence are explicit for every language
      Given whole-suite outcomes in a TypeScript, a Python and a Kotlin project with support globs
      When a support file is added, changed or removed
      Then each outcome is stale and its message names the support file
      When a test file is added only beneath a nested package.json, a .venv or node_modules
      Then the outer outcome stays fresh
      Given a non-Go whole-suite outcome with matching source and importer hashes but no whole-suite evidence
      Then it is stale and must rerun before it can be reused
      But a Go outcome recorded with the existing Go evidence key stays fresh

    # bug-2: the T-78 conversion exposed a pre-existing artifact defect. A partial
    # run can stamp current suite_evidence onto an unjudged unchanged function
    # while stale:true still prevents reuse. ADR-0016 and this scenario require
    # preserving the original evidence, not merely retaining that stale marker.
    # Fix this separately from T-78, with an independent compiling public CLI
    # regression committed and failing at its evidence assertion before product
    # edits. Cover Go (MUT-154's precedent), TypeScript, Python and Kotlin; a
    # missing unit, empty mutant set or nil evidence is a fixture failure.
    # Compare recorded test/support hash maps, outcome and recorded scope before
    # and after a --since run that judges another function. Keep the unjudged
    # source hash, original evidence and judgment, permitting only the already
    # specified stale marker and source-movement positions. The judged function
    # must receive current evidence. Preserve independent executable-coverage
    # evidence, own/listed dependencies, missing-evidence invalidation and all
    # existing partial/fail-fast guards. Check/sample/graph still reject stale
    # unjudged outcomes, and a later full run must judge them rather than reuse.
    # No new flags, schema, dependencies, exit policy or broad freshness changes.
    @bug-2 @whole-suite-freshness-all @ID-MUT-207
    Scenario: Mixed scopes and partial runs keep their own dependencies in every language
      Given TypeScript, Python and Kotlin outcomes recorded with own, all-tests and test-command scopes
      And only a non-importing test file changed beneath each build root
      When their freshness is checked by mutation check, mutation run, mutation sample and the graph
      Then the own outcomes stay usable and every whole-suite outcome is stale
      And all four agree on each function's freshness
      And a --since run that judges one function never refreshes the evidence of an unjudged one


    # An independent black-box regression for bug-2, executable while T-78's
    # conversion remains shelved. This repeats the accepted preservation rule,
    # making the raw artifact difference explicit; it adds no product policy.
    @bug-2 @ID-MUT-247
    Scenario Outline: A partial run preserves an unjudged <language> outcome's original whole-suite evidence
      Given a committed <language> project with two functions whose whole-suite outcomes record nonempty test and support evidence
      And a non-importing test and a configured support file changed since those outcomes were recorded
      And only one of its functions changed since "base"
      When a --since run judges that changed function
      Then the unjudged function keeps its original test and support evidence, recorded scope, source hash and mutant outcomes
      And the judged function records current whole-suite evidence
      And mutation check reports the unjudged function stale and mutation sample excludes its outcomes

      Examples:
        | language   |
        | Go         |
        | TypeScript |
        | Python     |
        | Kotlin     |

      # slice-36, dart-support: as MUT-205's Dart row. Before slice-36 the
      # Dart outcomes record no test, failing at the Given's check that they
      # record nonempty test evidence.
      @slice-36
      Examples:
        | language |
        | Dart     |

  # Language parity, second batch: these need the real tools T-13 brings to
  # CI (Linux only; other OSes skip them, naming the missing tool).
  # - ts-coverage-scope: in mutation run, when Vitest or Jest judges the
  #   mutants with related tests (no --all-tests), coverage is measured by
  #   the same related selection into the run's own directory, never by the
  #   project's coverage script, so a line only an unrelated test reaches is
  #   uncovered, never a false survivor (ID-MUT-08's rule). If that related
  #   measurement cannot run (Vitest too old, @vitest/coverage-v8 missing),
  #   the plan is unsupported as today for that case: non-strict runs fall
  #   back to running every mutant, strict runs fail closed (ID-MUT-204).
  #   With --all-tests, and in crap, the coverage script keeps its role, and
  #   a project with neither Vitest nor Jest keeps its current plan.
  # - counted-languages: counted runs resolve each language's installed
  #   dependencies from the live project root, never from the frozen export:
  #   Python's .venv (or the interpreter VIRTUAL_ENV names), TypeScript's
  #   node_modules, Gradle's and Maven's local caches. They install nothing
  #   and download nothing: Gradle runs with --offline and Maven with -o, as
  #   Go runs with GOPROXY=off, and a dependency missing offline is a
  #   preparation failure naming the tool. The sources, tests and config
  #   judged still come from the frozen commit. README's "dependencies as
  #   installed" boundary names these per language.
  Rule: Language parity with Go for TypeScript coverage scope and counted runs

    # Binary-only for good: the coverage command and the folder it writes
    # into are the real collector's.
    @ts-coverage-scope @ID-MUT-208 @binary-only
    Scenario: TypeScript coverage comes from the related tests that judge the mutants
      Given a Vitest project with a coverage script that runs the whole suite
      And src/a.ts has a line that only an unrelated test, which does not import it, executes
      When I run "itos-cc mutation run --json src/a.ts"
      Then coverage runs vitest related for src/a.ts into the run's own directory, not the coverage script
      And the mutant on that line is uncovered and none of its trials runs
      But with --all-tests the coverage script measures the whole suite as before
      And crap still measures with the coverage script

    # slice-17, issue #45: itos-cc chose a TypeScript package's runner by
    # the names its package.json declares. Vitest that comes in through
    # another package (Vite+'s vite-plus, its tests importing
    # vite-plus/test and its script "vp test") is never named there, so
    # the package was taken for one with neither Vitest nor Jest. Coverage
    # looked for c8, found none, and measured nothing: every mutant ran,
    # and --fail-uncovered could not pass. The baseline ran <pm> run test,
    # the whole suite, though node_modules/.bin/vitest was installed.
    # Decided in the item: a vitest that resolves from the package's
    # node_modules/.bin, as NodeBin finds it, counts as Vitest, in
    # coverage's choice of runner and in mutation's test command alike.
    # Coordinator's calls, 2026-10-21: npm and yarn hoist every transitive
    # bin, so a resolvable vitest counts only when the package's test
    # script names vitest, vp or vite, or a vite.config.* or
    # vitest.config.* sits beside its package.json; a package that declares
    # jest keeps Jest, a resolvable jest alone does not count, and
    # itos-cc never reads vite-plus or any toolchain's name. The version is
    # read from the package the bin resolves into, any version admitted
    # when that cannot be read, as today. The fixture's package declares
    # @vitest/coverage-v8 and a package named toolchain, which nothing
    # installs, and its node_modules/.bin holds vitest; it has no c8, as
    # the issue's had none. Both drivers run it: the fake toolchain reads
    # the fixture's node_modules/.bin, and the binary driver links the
    # pinned Vitest. Before slice-17 the baseline is <pm> run test, failing
    # at the first Then in both drivers.
    @slice-17 @ID-MUT-280
    Scenario: A package whose Vitest comes through another package is measured and mutated with Vitest
      Given a TypeScript package whose package.json names neither vitest nor jest, whose node_modules/.bin holds vitest and @vitest/coverage-v8 installed through a toolchain package it depends on, whose test script is "vitest run", and no c8
      When I run "itos-cc mutation run src/area.ts"
      Then the test command is the installed vitest related --run --bail=1 on the file
      And stderr does not say "neither Vitest nor Jest"
      And stderr does not say "running every mutant"
      And the exit code is 0
      But when its test script is "node --test" and no vite or vitest config sits beside its package.json, the test command is its package manager's run test, as before

    # Real frozen exports, installed runtime dependencies and offline build caches.
    @binary-only @counted-languages @ID-MUT-209
    Scenario Outline: A counted run judges a <language> project through its installed tools, offline
      Given a committed <language> project with eligible mutation sites and its dependencies installed in <installed>
      When a count-one run judges it
      Then preparation measures coverage and runs the clean baseline with the project's installed tools
      And exactly one selected mutant is judged with a real outcome
      And no command downloads or installs anything
      And the live working tree is left unchanged

      Examples:
        | language   | installed                         |
        | TypeScript | node_modules at the project root  |
        | Python     | .venv at the project root         |
        | Kotlin     | the Gradle cache, run with --offline |

      # slice-37, dart-counted-ready, q-C (b): git ignores .dart_tool, so a
      # frozen export has no package_config.json, and before its coverage is
      # planned each Dart build root runs its command's Ready there, flutter
      # pub get --offline (slice-35), resolving the commit's pubspec.lock
      # from the pub cache alone; its coverage, baseline and trials then run
      # with --no-pub. An .fvmrc's SDK is the live project's .fvm/flutter_sdk,
      # linked into the export as node_modules and .venv are. Before slice-37
      # the export's Dart coverage plan is unsupported, its dependencies not
      # resolved, and the run stops as count.preparation-failed, failing at
      # the preparation step; the second row then still runs the PATH's
      # flutter until the link is made.
      @slice-37
      Examples:
        | language | installed                                                |
        | Dart     | the pub cache, run with --no-pub                         |
        | Dart     | the pub cache and the SDK its .fvmrc pins, run with --no-pub |

  # python-fresh-bytecode, found by T-13's Python smoke test: a worker's
  # baseline compiles the source into __pycache__, Python validates bytecode
  # by the source's mtime in whole seconds and its size, and a same-size
  # mutant (== to !=) written within that second runs the unmutated bytecode
  # and survives falsely. Decisions (the coordinator's, as a correctness fix
  # with one obvious remedy): every Python command itos-cc runs in a worker
  # copy or a frozen copy (baselines, mutants, listed selections, coverage)
  # runs with PYTHONDONTWRITEBYTECODE=1, and worker copies carry no
  # __pycache__ directories from the live tree, so no stale bytecode can be
  # read. The project's own tree is never touched. T-13's smoke test then
  # requires the kill again.
  Rule: A Python mutant always runs as mutated

    # Real interpreter bytecode and worker filesystem cache lifecycle.
    @binary-only @python-fresh-bytecode @ID-MUT-210
    Scenario: A same-size Python mutant written within the baseline's second is still killed
      Given a Python project whose test kills a mutant that replaces "==" with "!="
      And the baseline and the mutant are written within the same second
      When I run "itos-cc mutation run --json" for its file
      Then the mutant is killed, not survived
      And no __pycache__ directory is written beside the worker copy's sources
      And the project's own tree, its __pycache__ included, is left unchanged

  # counted-worktree-run: a linked worktree's private scratch directory is
  # what git rev-parse --git-path itos names, under the main repository's
  # .git/worktrees/<name>. Counted runs accept that directory as private,
  # as they accept .git/itos in a main checkout: created with owner-only
  # permissions, never a path a symlink redirects, and removed after the
  # run. Anything else outside the checkout and its git directory is still
  # refused, and an unusable scratch directory is a reported problem with a
  # fix, never an internal error. Complete runs are unchanged.
  Rule: Counted runs work in a linked git worktree

    # Physical linked Git metadata, host scratch and symlink isolation.
    @binary-only @counted-worktree-run @ID-MUT-212
    Scenario: A count-one run in a linked worktree judges its committed selection
      Given a committed Go project and a second checkout of it made with git worktree add
      When a count-one run judges it from the linked worktree
      Then exactly one selected mutant is judged with a real outcome and the report names the worktree's HEAD commit
      And its private scratch directory lies under the repository's git directory for that worktree and is removed afterwards
      And neither checkout's working tree is changed
      But a scratch path that escapes both the checkout and its git directory is refused with a problem and a fix, not an internal error

  # parity-batch-3 (coordinator's routine calls, 2026-10-10, from the
  # complete run's precedent):
  # - counted-unjudged-language: a complete strict run fails over a language
  #   its per-language commands measured nothing of only when that
  #   language has something to judge (unmeasuredToJudge). A counted run
  #   follows it: a file with no eligible site, in a language with nothing
  #   to judge, needs no measurement, so a tool missing for that language
  #   stops nothing. Strict Go still inventories every judged Go function,
  #   and a language with an eligible site still fails closed.
  # - python-no-pytest-cache: every pytest command itos-cc composes runs with
  #   -p no:cacheprovider, as every one already runs with
  #   PYTHONDONTWRITEBYTECODE=1, so measuring and judging leave no
  #   .pytest_cache in the project's tree. Plugin autoload stays the
  #   project's: a project's plugins may be what its tests need. A
  #   --test-command or --coverage-command is the user's and is unchanged.
  Rule: Counted and Python runs touch only what their judgments need

    @counted-unjudged-language @ID-MUT-213
    Scenario: A strict counted run needs no measurement of a language with nothing to judge
      Given a committed project with a Go module and a TypeScript file with no mutation site
      And no TypeScript coverage tool is installed
      When a strict count-one run judges both files
      Then its Go functions are judged for coverage evidence and one Go mutant is judged with a real outcome
      And no stage fails and no TypeScript coverage is measured
      But when the TypeScript file has an eligible site the run fails with "count.preparation-failed" at its coverage stage, as before

    # Real pytest cache lifecycle in original and disposable worker trees.
    @binary-only @python-no-pytest-cache @ID-MUT-214
    Scenario: Measuring and judging a Python project leaves no pytest cache in its tree
      Given a Python project with a pytest test that kills its mutant and no .pytest_cache directory
      When I run "itos-cc mutation run --json" for its file
      Then the mutant is killed
      And no .pytest_cache directory is written in the project's tree or beside the worker copy's sources
      But a run with --test-command runs that command exactly as given

  # transitive-test-reach, q-39 (answered 2026-10-10): a TypeScript, Python
  # or Kotlin test reaches every project module in the transitive closure of
  # its imports (Kotlin's same-package references included), matching what
  # vitest related and jest --findRelatedTests select. Those are the file's
  # tests wherever itos-cc uses them: the tests a result records and whose
  # changes make it stale (mutation check, run, sample, except and the
  # graph), and, once q-38's own-tests narrowing lands, the tests a file's
  # own command runs. Go keeps its rule: its own package and the packages it
  # imports, one hop. A result recorded under the old direct-import reach
  # may read stale once, and is re-run as any stale result is.
  Rule: TypeScript, Python and Kotlin tests reach what they import transitively

    @transitive-test-reach @ID-MUT-215
    Scenario Outline: A <language> test that reaches a file through another module is among its tests
      Given a <language> project whose test imports module A, where A imports module B and no test imports B directly
      And B's mutant has a recorded result
      When that test changes
      Then "itos-cc mutation check" reports B's result stale, naming the test
      And a test that reaches neither A nor B changing leaves B's result fresh
      But in a Go module a test still reaches only its own package and the packages it imports, not a package two imports away

      Examples:
        | language   |
        | typescript |
        | python     |
        | kotlin     |

      # slice-34, dart-support: as TypeScript, Python and Kotlin.
      @slice-34
      Examples:
        | language   |
        | dart       |

    # slice-34: how a Dart library names another. package:<own name>/x.dart
    # is lib/x.dart of the package whose pubspec.yaml has that name; an
    # export passes reach on; a part file is its library's, so reaching the
    # library reaches the part; a conditional import reaches each target.
    @slice-34 @ID-MUT-263
    Scenario Outline: A Dart test reaches a library through <link>
      Given a Flutter package "demo" whose test reaches lib/a.dart only through <link>
      And a mutant of lib/a.dart has a recorded result
      When that test changes
      Then "itos-cc mutation check" reports that result stale, naming the test
      And a test that reaches no library linking to lib/a.dart changing leaves that result fresh

      Examples:
        | link                                                                       |
        | an import of package:demo/a.dart                                           |
        | a relative import of ../lib/a.dart                                         |
        | lib/demo.dart, which it imports and which exports a.dart                   |
        | lib/b.dart, which it imports and which holds part 'a.dart'                 |
        | a conditional import of lib/b.dart, if (dart.library.io) lib/a.dart        |
        | package:core/c.dart of a pub workspace sibling "core", which imports package:demo/a.dart |

  # own-tests-python, own-tests-kotlin and own-tests-ts-fallback, q-38
  # (answered 2026-10-10): a file's own command, and the coverage that
  # decides which of its mutants run, are the tests that reach it
  # (transitive-test-reach), as Go runs its own package and TypeScript its
  # Vitest or Jest related tests:
  # - Python: python -m pytest -q -x -p no:cacheprovider <test files>, or
  #   python -m unittest <test modules> when pytest is not installed.
  # - Kotlin: gradle -p <module> test --fail-fast --tests <classes>, or
  #   mvn -q test -Dtest=<classes> -Dsurefire.failIfNoSpecifiedTests=false,
  #   the classes the reaching test files declare, per build module.
  # Coordinator's routine calls, from the Go precedent:
  # - A file no test reaches runs no test: its mutants are uncovered, as in
  #   a Go package with no tests, never judged against the whole suite.
  #   --all-tests and --test-command keep the whole suite.
  # - A TypeScript test script without Vitest or Jest cannot be narrowed,
  #   so its command stays the script, and its outcomes are recorded with
  #   whole-suite evidence (ADR-0016, suite_evidence), as a --test-command
  #   outcome is: any test change makes them stale.
  Rule: A file's own tests are the tests that reach it, in every language

    @own-tests-python @ID-MUT-216
    Scenario Outline: A Python file's own tests are the tests that reach it, with <runner>
      Given a Python project run with <runner> where test_a reaches a.py, test_b reaches only b.py, and test_b fails
      When I run "itos-cc mutation run --json" for a.py
      Then a.py's baseline passes and its mutant is judged by test_a alone, with coverage measured from test_a alone
      And test_b never runs
      But a Python file no test reaches has its mutants reported uncovered, and no test command runs for it

      Examples:
        | runner |
        | pytest |

      # Binary-only for good: its virtualenv is one a real command makes.
      @binary-only
      Examples:
        | runner   |
        | unittest |

    @own-tests-kotlin @ID-MUT-217
    Scenario Outline: A Kotlin file's own tests are the test classes that reach it, with <build>
      Given a Kotlin project built with <build> where ATest, in A.kt's package, uses A without importing it, BTest reaches only B.kt, and BTest fails
      When I run "itos-cc mutation run --json" for A.kt
      Then A.kt's baseline passes and its mutant is judged by ATest alone, with coverage measured from ATest alone
      And BTest never runs

      Examples:
        | build  |
        | gradle |
        | maven  |

    # slice-35: a Dart file's own tests are the test files that reach it,
    # as Python's are; integration_test/ never runs (q-76).
    @slice-35 @ID-MUT-264 @binary-only
    Scenario: A Dart file's own tests are the test files that reach it
      Given a Flutter package where test/a_test.dart reaches lib/a.dart, test/b_test.dart reaches only lib/b.dart and fails, and integration_test/app_test.dart imports lib/a.dart
      When I run "itos-cc mutation run --json" for lib/a.dart
      Then lib/a.dart's baseline passes and its mutant is judged by test/a_test.dart alone, with coverage measured from test/a_test.dart alone
      And test/b_test.dart and integration_test/app_test.dart never run
      But a Dart file no test reaches has its mutants reported uncovered, and no test command runs for it

    # slice-38, q-76: flutter test never runs integration_test/ or
    # test_driver/, so no change there can change a Dart outcome. The tests
    # a Dart outcome records, and whose changes make it stale, are those its
    # run runs: the test files under test/ that reach the file, as DartTests
    # keeps them, with the support files on their way; with --all-tests, the
    # suite under test/. A file under integration_test/ or test_driver/ is
    # never among them, though it imports the file. Both drivers run it, as
    # mutation check runs no test. Before slice-38 reach counts those files,
    # so the check reports the result stale, failing at the Then.
    @slice-38 @ID-MUT-274
    Scenario Outline: A Dart result recorded by <run> stays fresh when only tests flutter test never runs change
      Given a Flutter package where test/a_test.dart reaches lib/a.dart, and integration_test/app_test.dart and test_driver/app.dart import lib/a.dart
      And a mutant of lib/a.dart has a result recorded by "<run>"
      When integration_test/app_test.dart and test_driver/app.dart change, and nothing else
      Then "itos-cc mutation check --json" reports no result stale and exits 0
      But after test/a_test.dart changes, it reports that result stale

      Examples:
        | run                                  |
        | itos-cc mutation run                 |
        | itos-cc mutation run --all-tests     |

    # slice-35: a worker copy of a Flutter package resolves itself once,
    # offline, so every mutant's flutter test runs with --no-pub in its own
    # copy; platform folders stay out (q-75) and generated Dart stays in,
    # as tests compile against it (q-77). A pub workspace member needs the
    # workspace root's resolution and its sibling members beside it.
    @slice-35 @ID-MUT-265 @binary-only
    Scenario Outline: Workers run the mutants of a <package> offline, in copies of their own
      Given a resolved <package> with android and ios folders and a generated lib/user.g.dart its tests import
      When I run "itos-cc mutation run --workers 2" for one of its libraries
      Then each worker's copy holds no android or ios folder, and holds lib/user.g.dart
      And each worker runs flutter pub get --offline once, before its first mutant, and nothing reaches the network
      And every mutant's flutter test runs with --no-pub in its worker's copy
      And the mutants are judged as a single worker judges them

      Examples:
        | package                                |
        | Flutter app                            |
        | member of a pub workspace with a sibling member it depends on |

    @own-tests-ts-fallback @ID-MUT-218
    Scenario: A TypeScript test script without Vitest or Jest records whole-suite outcomes
      Given a TypeScript project whose test script runs Node's own test runner, with neither Vitest nor Jest installed
      And a mutant of one of its files has a result from "itos-cc mutation run"
      When a test that reaches no module of that file changes
      Then "itos-cc mutation check" reports that result stale, as it reports a --test-command outcome

  # test-support-reach (coordinator's routine call, 2026-10-10, from q-39's
  # transitive closure; found by own-tests-python as the idea
  # python-reach-through-test-helpers): test-support files, the files a
  # language counts as test code that are not themselves runnable tests
  # (a Python helper under tests/, a TypeScript helper under __tests__, a
  # Kotlin helper in src/test), pass reach on: a test reaches what the
  # test-support files it imports reach, transitively. A Python test also
  # reaches what every conftest.py in its directory and the directories
  # above it, up to its build root, reaches, since pytest loads those
  # without an import. The runnable test is what a file's own command runs
  # (own-tests-python's test_*.py and *_test.py) and what its freshness
  # names, beside the support files its reach passes through. Go is
  # unchanged.
  Rule: Tests reach code through the test-support files they use

    @test-support-reach @ID-MUT-219
    Scenario Outline: A <language> test reaches a file through <support>
      Given a <language> project whose test reaches module A only through <support>, which imports A
      And A's mutant has a recorded result
      When that test changes
      Then "itos-cc mutation check" reports A's result stale, naming the test
      And a test that uses no support reaching A changing leaves A's result fresh

      Examples:
        | language   | support                          |
        | python     | a helper module under tests/     |
        | python     | a conftest.py fixture above it   |
        | typescript | a helper module under __tests__/ |
        | kotlin     | a helper class in src/test       |

      # slice-34: flutter test loads the nearest flutter_test_config.dart at
      # or above a test file, up to its package root, as pytest loads
      # conftest.py.
      @slice-34
      Examples:
        | language   | support                                   |
        | dart       | a helper library under test/              |
        | dart       | a flutter_test_config.dart above it       |

    @test-support-reach @ID-MUT-220
    Scenario: A Python mutant reached only through conftest.py is judged by the test that uses the fixture
      Given a Python project whose test_a.py uses a conftest.py fixture that imports a.py, and no test imports a.py
      When I run "itos-cc mutation run --json" for a.py
      Then a.py's baseline runs test_a.py, its coverage is measured from test_a.py, and its mutant is killed, not reported uncovered

  # strict-lines-python, strict-lines-typescript, strict-lines-kotlin and
  # strict-scope-refusals, q-40 (answered 2026-10-10): ADR-0017, 0018 and
  # 0019 extend to TypeScript, Python and Kotlin. With --fail-uncovered,
  # every judged executable function of those languages, a function with no
  # mutation site included, needs fresh measured line coverage evidence:
  # - Precision is the format's: LCOV DA lines from coverage.py and from
  #   Vitest's or Jest's v8 or istanbul reports, JaCoCo or Kover XML lines.
  #   An executable line no reaching test executed is
  #   mutation.uncovered-statement (file, function, line), separate from
  #   mutant counts. A JaCoCo line with both missed and covered
  #   instructions is covered; branches are out of scope, as Go has none.
  # - Evidence comes only from the built-in per-language commands over the
  #   file's reaching tests (own-tests-*), or --all-tests' whole suite, and
  #   is stored per function, bound to its hash and to fingerprints of its
  #   sources, reaching tests, configured support and the coverage
  #   producer, so mutation check gives the run's verdict without running a
  #   test. A function without complete evidence is
  #   mutation.coverage-missing; evidence whose inputs changed is
  #   mutation.coverage-stale. Strict runs of these languages refuse
  #   --coverage-report, --use-existing-coverage and --coverage-command, as
  #   strict Go runs do.
  # - mutation.coverage-unsupported refuses scopes a root's fingerprints
  #   cannot prove, even when an older cache looks fresh: npm, pnpm or yarn
  #   workspaces and file:, link: or workspace: dependencies outside the
  #   root; Python path or editable dependencies outside the root; Gradle
  #   includeBuild and Maven parent relativePath or modules outside the
  #   root. itos-cc never follows them.
  # Without --fail-uncovered nothing changes.
  Rule: Strict runs prove executable line coverage in every language

    @strict-lines-python @ID-MUT-221
    Scenario: A strict Python run proves every executable line of every judged function
      Given a Python project whose judged file has a function with no mutation site that no test executes, and a reached function with one line no test executes
      When I run "itos-cc mutation run --fail-uncovered --json" for the file
      Then each unexecuted executable line is reported as "mutation.uncovered-statement" with its file, function and line, the mutant counts are unchanged, and the run fails
      And "itos-cc mutation check --fail-uncovered" reports the same findings without running a test
      And after a test that reaches the file changes, the check reports "mutation.coverage-stale" until a new run measures it again
      And a strict Python run refuses --coverage-report, --use-existing-coverage and --coverage-command
      But without --fail-uncovered the run and the check report as before

    @strict-lines-typescript @ID-MUT-222
    Scenario: A strict TypeScript run proves every executable line of every judged function
      Given a TypeScript project tested with Vitest whose judged file has a function with no mutation site that no test executes, and a reached function with one line no test executes
      When I run "itos-cc mutation run --fail-uncovered --json" for the file
      Then each unexecuted executable line is reported as "mutation.uncovered-statement" with its file, function and line, the mutant counts are unchanged, and the run fails
      And "itos-cc mutation check --fail-uncovered" reports the same findings without running a test
      And after a test that reaches the file changes, the check reports "mutation.coverage-stale" until a new run measures it again
      And a strict TypeScript run refuses --coverage-report, --use-existing-coverage and --coverage-command
      But without --fail-uncovered the run and the check report as before

    @strict-lines-kotlin @ID-MUT-223
    Scenario: A strict Kotlin run proves every executable line of every judged function
      Given a Kotlin project built with Gradle and JaCoCo whose judged file has a function with no mutation site that no test executes, a reached function with one line no test executes, and a line a test executes only one branch of
      When I run "itos-cc mutation run --fail-uncovered --json" for the file
      Then each unexecuted executable line is reported as "mutation.uncovered-statement" with its file, function and line, and the partly executed line is not, the mutant counts are unchanged, and the run fails
      And "itos-cc mutation check --fail-uncovered" reports the same findings without running a test
      And after a test that reaches the file changes, the check reports "mutation.coverage-stale" until a new run measures it again
      And a strict Kotlin run refuses --coverage-report, --use-existing-coverage and --coverage-command
      But without --fail-uncovered the run and the check report as before

    # slice-36, dart-support, q-40's rule for Dart: a strict Dart run's
    # evidence is flutter test --coverage's LCOV over the test files under
    # test/ that reach the file (slice-35), or test/ whole with --all-tests,
    # never integration_test/ (q-76). A function's lines run from its
    # signature, where the VM counts each call (q-D), and a line any of
    # whose code ran is covered, as branches are out of scope. Both drivers
    # run it: the fake measures what it declares, cd.yml's dart job the real
    # flutter test. Before slice-36 Dart is not strict, so no line is
    # reported and the run passes, failing at the first Then.
    @slice-36 @ID-MUT-271
    Scenario: A strict Dart run proves every executable line of every judged function
      Given a Flutter package whose judged library has a function with no mutation site that no test executes, a reached function with one line no test executes, and a line a test executes only one operand of
      When I run "itos-cc mutation run --fail-uncovered --json" for the file
      Then each unexecuted executable line is reported as "mutation.uncovered-statement" with its file, function and line, and the partly executed line is not, the mutant counts are unchanged, and the run fails
      And "itos-cc mutation check --fail-uncovered" reports the same findings without running a test
      And after a test that reaches the file changes, the check reports "mutation.coverage-stale" until a new run measures it again
      And a strict Dart run refuses --coverage-report, --use-existing-coverage and --coverage-command
      But without --fail-uncovered the run and the check report as before

    # slice-36, q-A (fail closed): flutter test's LCOV names only the
    # libraries a test loaded, and itos-cc runs nothing yet that lists a
    # library's lines without a test loading it, so a library no test loads
    # has no evidence: each of its functions is mutation.coverage-missing,
    # as a TypeScript file under Jest or c8 is, and its mutants stay
    # uncovered. Both drivers run it. Before slice-36 no function of it is
    # reported, failing at the Then.
    @slice-36 @ID-MUT-272
    Scenario: A strict Dart run fails closed on a library no test loads
      Given a Flutter package whose lib/lonely.dart no test imports, beside a library a test reaches
      When I run "itos-cc mutation run --fail-uncovered --json" for both libraries
      Then each function of lib/lonely.dart is reported as "mutation.coverage-missing" and none as "mutation.uncovered-statement", and the run fails
      And "itos-cc mutation check --fail-uncovered" reports the same findings without running a test
      But without --fail-uncovered the run and the check report as before

    # slice-36, q-B (b): a Dart package's evidence rests on its package
    # root, and a pub workspace member's on the whole workspace, as a Kotlin
    # module's rests on its whole build: every Dart file there (generated
    # Dart included, which tests compile against), each pubspec.yaml,
    # pubspec_overrides.yaml and dart_test.yaml, the pubspec.lock, the
    # .fvmrc and the flutter that runs. integration_test/, test_driver/,
    # build/, coverage/, hidden folders and a Flutter app's platform folders
    # are not inputs. Both drivers run it. Before slice-36 a strict Dart run
    # records no evidence, so the check reports nothing, failing at the
    # Then. slice-36 checks only the evidence in the But step, as reach
    # counted integration_test/app_test.dart among the library's tests and
    # so made its results stale; slice-38 restores the step's whole check,
    # the check exiting 0.
    @slice-36 @ID-MUT-273
    Scenario Outline: Strict Dart evidence goes stale when <changed> changes
      Given a <package> whose judged library a strict mutation run measured
      When <changed> changes, and nothing else
      Then "itos-cc mutation check --fail-uncovered --json" reports "mutation.coverage-stale" for each function of the judged library, naming <changed>, and runs no test
      But a change to integration_test/app_test.dart alone leaves its evidence fresh

      Examples:
        | package                                             | changed            |
        | pub workspace member app beside its dependency core | pubspec.lock       |
        | pub workspace member app beside its dependency core | core/lib/core.dart |
        | Flutter app                                         | pubspec.lock       |
        | Flutter app                                         | lib/user.g.dart    |

    # Fresh here is a complete matching adversarial cache, observed first from
    # the same supported plain project. It does not claim that an unsupported
    # scope admitted a measurement; only changed existing manifest hashes are
    # replaced before testing the lexical refusal.
    @strict-scope-refusals @ID-MUT-224
    Scenario Outline: Strict coverage refuses <scope>
      Given a <language> project with <scope>, and fresh line coverage evidence for its judged functions
      When I run "itos-cc mutation check --fail-uncovered --json" and "itos-cc mutation run --fail-uncovered --json"
      Then both report "mutation.coverage-unsupported" for its judged functions, naming the scope, and fail
      And itos-cc reads nothing outside the project root
      But without --fail-uncovered both report as before

      Examples:
        | language   | scope                                              |
        | typescript | an npm workspace                                   |
        | typescript | a file: dependency outside the root                |
        | python     | an editable dependency outside the root            |
        | kotlin     | a Gradle includeBuild outside the root             |
        | kotlin     | a Maven module outside the root                    |

      # slice-36, q-B (b): a Dart package's evidence rests on its package
      # root, or its pub workspace's root for a member, so a path
      # dependency, or a path override in pubspec.yaml or
      # pubspec_overrides.yaml, that leaves that root is refused, as is one
      # into a nested package that is not a member, which the fingerprints
      # leave out. Before slice-36's refusal, the fresh Dart evidence was
      # reused, failing at the Then.
      @slice-36
      Examples:
        | language | scope                                                   |
        | dart     | a path dependency outside the root                      |
        | dart     | a pubspec_overrides.yaml path override outside the root |

      # slice-18, issue #48: a pnpm workspace member that depends on
      # another member stays refused, as ADR-0025 says. The refusal now
      # names the dependency rather than the workspace file, since the file
      # alone no longer refuses anything. Before slice-18 it names
      # pnpm-workspace.yaml, failing at the Then.
      @slice-18
      Examples:
        | language   | scope                                                |
        | typescript | a pnpm workspace member with a workspace: dependency |

    # slice-18, issue #48: strict coverage refused every TypeScript
    # function under any pnpm-workspace.yaml from its package root up to
    # the project root, as a pnpm workspace, the moment the file existed.
    # pnpm keeps its settings (minimumReleaseAge, allowBuilds, ...) in that
    # file, so a single package with settings and no packages key was
    # refused, though the file sits in its package root and is already
    # fingerprinted there. --fail-uncovered could not be used anywhere
    # pnpm is set up that way. Decided in the item: a pnpm-workspace.yaml
    # with no packages key is a settings file, not a workspace.
    # Coordinator's calls, 2026-10-21: a packages key that is null or an
    # empty list counts as none; a settings-only file above the package
    # root, within the project root, is fingerprinted as the package's
    # input, as a workspace root's files are (MUT-282), not refused. Both
    # drivers run it: the fake scope reader and fingerprints copy the real
    # ones. Before slice-18 every function is
    # "mutation.coverage-unsupported", naming pnpm-workspace.yaml, and the
    # run exits 1, failing at the first Then.
    @slice-18 @ID-MUT-281
    Scenario: A pnpm-workspace.yaml with no packages key is the package's settings, not a workspace
      Given a TypeScript package tested with Vitest whose own pnpm-workspace.yaml holds "minimumReleaseAge: 1440" and no packages key
      When I run "itos-cc mutation run --fail-uncovered --json" for its source
      Then no function is "mutation.coverage-unsupported", and the exit code is 0
      And "itos-cc mutation check --fail-uncovered --json" exits 0 and runs no test
      And after pnpm-workspace.yaml's settings change, the check reports "mutation.coverage-stale" for each judged function, naming pnpm-workspace.yaml, and runs no test

    # slice-18, issue #48: a member of a real pnpm workspace was refused
    # too, though nothing it reads lies outside its package but its
    # workspace root's settings. Decided with the person on 2026-10-18,
    # amending ADR-0025: a member with no workspace:, file: or link:
    # dependency is fingerprinted like a standalone package, and a member
    # with one stays refused (MUT-224). Coordinator's calls, 2026-10-21:
    # a dependency naming another member by its package name with a plain
    # version is local too (pnpm links it under link-workspace-packages)
    # and refused; an override in pnpm-workspace.yaml whose value is link:
    # or file: outside the package root is refused; the workspace's own
    # root package, which holds the other members, stays refused. What a
    # member reads from the workspace root is fingerprinted beside its
    # package: the root's config files as a package root's are named
    # (package.json, pnpm-lock.yaml, pnpm-workspace.yaml, tsconfig*.json,
    # ...), its .npmrc and .pnpmfile.cjs, and the patch files
    # patchedDependencies names; nothing else of the root. Its sibling
    # members are no input, so a change there leaves its evidence fresh.
    # The refusal's fix text stays one sentence and names the scope, as
    # today. npm and yarn workspaces stay refused (MUT-224's npm row), as
    # the item names pnpm alone. The run's folder does not matter, as the
    # issue saw. Both drivers run it, as MUT-281. The member's
    # tsconfig.json does not extend the root's tsconfig.base.json, since a
    # worker's copy cannot reach it until slice-19. Before slice-18 every
    # function is "mutation.coverage-unsupported", naming
    # pnpm-workspace.yaml, failing at the first Then.
    @slice-18 @ID-MUT-282
    Scenario Outline: A pnpm workspace member with no local dependency is proven as a package of its own, beside its workspace root's settings
      Given a pnpm workspace whose root lists packages/* and holds pnpm-lock.yaml and tsconfig.base.json, with the members packages/label, tested with Vitest and depending on no workspace:, file: or link: package, and packages/other
      When I run "itos-cc mutation run --fail-uncovered --json" for packages/label/src/label.ts, from <folder>
      Then no function is "mutation.coverage-unsupported", and the exit code is 0
      And after the workspace root's <changed> changes, "itos-cc mutation check --fail-uncovered --json" reports "mutation.coverage-stale" for each judged function, naming <changed>, and runs no test
      But a change to packages/other/src/other.ts alone leaves its evidence fresh

      Examples:
        | folder           | changed             |
        | the project root | pnpm-lock.yaml      |
        | packages/label   | pnpm-workspace.yaml |
        | the project root | package.json        |
        | the project root | tsconfig.base.json  |
        | the project root | .npmrc              |

  # strict-changed-lines, issue #33, q-43 (answered 2026-10-11):
  # --fail-uncovered=lines, with --since, judges coverage only where the
  # range changed: an executable block or line is judged when it overlaps a
  # hunk of git diff --unified=0 <base>...HEAD, for every language and in
  # mutation run, counted runs (--count) and mutation check alike. The
  # coverage rules follow the lines: mutation.uncovered-statement,
  # mutation.coverage-missing and mutation.coverage-stale, and an uncovered
  # mutant fails only on a changed line. Survivors are judged as before,
  # over every admitted function. Evidence is measured and recorded as
  # before, whole functions, so a later --fail-uncovered check of the same
  # snapshot still judges them whole. --fail-uncovered alone is
  # --fail-uncovered=functions, ADR-0017's whole functions, unchanged.
  # Without --since, --fail-uncovered=lines is a usage error.
  Rule: With --since, --fail-uncovered=lines judges coverage only on the changed lines

    @strict-changed-lines @ID-MUT-225
    Scenario Outline: A <language> change is judged on the lines it changed, not the old code around them
      Given a <language> project whose function has an executable line no test runs, committed at a base
      And a commit since the base that changes another line of that function, which a test runs
      When I run "itos-cc mutation run --since <base> --fail-uncovered=lines --json" for its file
      Then the old unexecuted line is not reported and the run passes
      And "itos-cc mutation check --since <base> --fail-uncovered=lines" gives the same verdict
      But "itos-cc mutation run --since <base> --fail-uncovered" still reports the old line as "mutation.uncovered-statement"
      And a commit that adds a line no test runs is reported on that line, and the run fails

      Examples:
        | language |
        | go       |
        | python   |

    @strict-changed-lines @ID-MUT-226
    Scenario: Changed-line strictness needs a range and holds in a counted run
      When I run "itos-cc mutation run --fail-uncovered=lines" without --since
      Then it fails as a usage error, exit 80, naming --since
      And a counted Go run with --count 1 --since <base> --fail-uncovered=lines judges coverage only on the changed lines, as a complete run does

    # Slice-11 (issue #22): a function the range touched only in its
    # comments is not judged, so none of its lines asks evidence, though a
    # comment at the end of an executable line shares that line's hunk. A
    # change to another line no test can execute, such as a signature, still
    # asks it (the idea strict-lines-nonexecutable-changes). Strict Go
    # evidence is bound to the module's file bytes, so a check that judges
    # the whole function still finds it stale, as after any edit of the
    # module.
    @slice-11 @ID-MUT-259
    Scenario: A comment-only change asks no coverage evidence of its function
      Given a Go function with fresh results and fresh strict coverage evidence, committed at "base"
      And a commit after "base" only added a comment at the end of one of its executable lines
      When I run "itos-cc mutation check --since base --fail-uncovered=lines --json"
      Then it reports no problem and the exit code is 0
      And "itos-cc mutation check --since base --fail-uncovered" exits 0
      But "itos-cc mutation check --fail-uncovered --json" of its file reports "mutation.coverage-stale" for it, naming that file

  # except-renew, issue #32, q-42 (answered 2026-10-11): mutation except
  # --renew carries reviewed exceptions across a change that leaves their
  # mutant the same: a rename, a literal or comment edit, reformatting, a
  # line added elsewhere in the function. Run after mutation run, it
  # renews a stale exception only when its function's fresh results hold
  # exactly one site with the same original and replacement on a line
  # whose text is unchanged, and that mutant survived the fresh run: the
  # entry then names the function's current hash, line and column and
  # keeps its reason. Every renewal is reported with its old and new hash
  # and the site, so the commit shows what a person reviewed carrying over.
  # An exception whose mutant the fresh run killed, whose site changed,
  # whose match is ambiguous, or whose function has no fresh results is
  # not renewed; it is reported with why, and stays
  # mutation.exception-stale. --renew writes nothing else.
  # stale-fix-renew (issue #32): a stale exception whose function changed
  # names the renewal as its fix first, so a person tries it before
  # rewriting the entry by hand.
  Rule: A reviewed exception can be renewed when its mutant is unchanged

    @except-renew @ID-MUT-227
    Scenario: A renewed exception follows its unchanged mutant across a rename
      Given a Go project with a reviewed exception for an equivalent mutant in a function
      And a commit that renames a string literal on another line of that function
      And "itos-cc mutation run" has judged the function again and the mutant still survives
      When I run "itos-cc mutation except --renew --json"
      Then the exception names the function's current hash, line and column and keeps its reason
      And the renewal is reported with the old and new hash and the site
      And "itos-cc mutation check" passes
      But an exception whose mutant the fresh run killed, whose line changed, or whose function has no fresh results is not renewed, is reported with why, and stays "mutation.exception-stale"

    @stale-fix-renew @ID-MUT-242
    Scenario: A stale exception whose function changed names the renewal as its fix
      Given itos-cc.yaml excepts a survivor of "Board#count", and "Board#count" changed since the entry was written
      When I run "itos-cc mutation run src/board.ts --json"
      Then the "mutation.exception-stale" problem, with why "changed", has a fix naming "itos-cc mutation except --renew" before "--reason"
      And "itos-cc mutation check src/board.ts --json" gives it the same fix

    # slice-20, issue #55: --renew writes through the same codec as mutation
    # except (RenewDocument), so it restyled the same lines. It now changes
    # only the lines of the entries it renews. Both drivers run it. Before
    # slice-20 the whole document is re-encoded, failing at the Then.
    @slice-20 @ID-MUT-292
    Scenario: Renewing an exception changes only the lines of its entry
      Given a Go project with a reviewed exception for an equivalent mutant in a function
      And itos-cc.yaml is laid out by hand around that entry: a "---" line, blank lines between keys, `other: { each: "{id}", sep: "|" }`, and a comment two spaces after a value
      And a commit that renames a string literal on another line of that function
      And "itos-cc mutation run" has judged the function again and the mutant still survives
      When I run "itos-cc mutation except --renew --json"
      Then only the lines of the renewed entry changed in itos-cc.yaml

    # slice-22, issue #50: an exception whose own line changed is never
    # renewed, by design (MUT-227), and the only way on was to except the
    # mutant at its new site, typing the reason again, which added a second
    # entry and left the old one stale. run and check also called that entry
    # gone, at its old column, while --renew called it changed, and their
    # fix sent the person to edit itos-cc.yaml. Decided by the person on
    # 2026-10-18: keep --renew's line-text guard; excepting the same change
    # in the same function replaces the stale entry and keeps its reason;
    # run and check give the same why as --renew. Coordinator's calls,
    # 2026-10-21:
    # - An entry is re-pointed when it is stale and names the site's file,
    #   its function by namespace#name, and its original and replacement,
    #   and it is the only such entry. An entry that holds, one stale as
    #   killed, and one whose file moved are never re-pointed. With several,
    #   the site is excepted as a new entry, as today.
    # - --reason may be left out when the site re-points an entry, which
    #   then keeps its reason; given, it replaces it. Without --reason and
    #   nothing to re-point, the refusal is flags.value-missing, as now
    #   (MUT-87), even when the site is no survivor.
    # - The command reports "excepted again" and "replaced": true, as for
    #   the same site.
    # - run and check call an entry changed when its function, by
    #   namespace#name, still has a site of its change, and gone when the
    #   function or every such site is gone, as --renew does. The changed
    #   fix (MUT-242) applies, and --renew's fix for an entry it left says
    #   that excepting the new site replaces the entry.
    # Both drivers run it, as it needs no real tool. Before slice-22 the
    # command without --reason is flags.value-missing and the entry keeps
    # column 19, failing at the second Then of MUT-297; given --reason, a
    # second entry is added. run and check call the entry gone, failing at
    # the Then of MUT-298.
    @slice-22 @ID-MUT-297
    Scenario: Excepting the same change in the same function re-points its stale entry, keeping its reason
      Given itos-cc.yaml excepts the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      And "Board#count" became "count(x){ const n=x; return n<5; }", and "itos-cc mutation run src/board.ts" recorded its `<` → `<=` at src/board.ts:7:30 surviving
      When I run "itos-cc mutation except src/board.ts:7:30 --json"
      Then itos-cc.yaml has one entry under mutation.exceptions
      And it names "Board#count"'s current hash, line_in_function 1, column 30 and its line text, and keeps the reason "the loop only counts up"
      And the JSON says "replaced": true
      And "itos-cc mutation check src/board.ts --json" exits 0
      But given "--reason 'reworded'", the entry is re-pointed the same way, its reason "reworded"

    @slice-22 @ID-MUT-298
    Scenario: run, check and --renew give the same why for an entry whose change moved along its line
      Given itos-cc.yaml excepts the survivor `<` → `<=` at src/board.ts:7:19 in "Board#count"
      And "Board#count" became "count(x){ const n=x; return n<5; }", and "itos-cc mutation run src/board.ts" recorded its `<` → `<=` at src/board.ts:7:30 surviving
      When I run "itos-cc mutation check src/board.ts --json"
      Then the problem is "mutation.exception-stale", with why "changed"
      And the "mutation.exception-stale" problem, with why "changed", has a fix naming "itos-cc mutation except --renew" before "--reason"
      And "itos-cc mutation run src/board.ts --json" and "itos-cc mutation except --renew --json" name it "changed" too
      But once "Board#count" has no `<` left and mutation run has judged it, all three name it "gone"

  # slice-21, issue #51: an entry of itos-cc.yaml that is stale as gone,
  # killed or excluded fails every mutation run and check, and its fix said
  # to delete it by hand: mutation except can only add or replace an entry,
  # and --renew carries over only entries whose mutant is unchanged. Decided
  # in the item's why: mutation except --prune removes each entry whose why
  # is gone, killed or excluded, reports each removal, and keeps the file's
  # other keys and comments. Coordinator's calls, 2026-10-21:
  # - --prune, run after mutation run, judges every entry of itos-cc.yaml
  #   as a check of the whole project does. Gone and excluded need no
  #   results; killed needs fresh results that record its mutant killed,
  #   and an entry whose function has none is kept.
  # - It keeps an entry that is changed or moved: --renew or an edit of its
  #   file mends those, and mutation check still reports them.
  # - It writes as slice-20 does: only the removed entries' lines change,
  #   with the comment lines directly above each.
  # - Plain output is a line per removal, "pruned <file>[:<line>:<column>]
  #   `original` → `replacement` in <function> (<why>) from itos-cc.yaml";
  #   --json gives "pruned", a list of the entries with "why" and, when its
  #   site is found, "line". It exits 0 once it has written, even when
  #   entries it keeps are stale.
  # - It takes no site and no --reason, as --renew does, and is not given
  #   beside --renew.
  # - The fixes that said "Remove its entry from itos-cc.yaml." name
  #   'itos-cc mutation except --prune' instead.
  # Both drivers run it, as it needs no real tool. Before slice-21 --prune
  # is flags.unknown, exit 80, and itos-cc.yaml is left as it was, failing
  # at the first Then of each scenario.
  Rule: Stale exceptions that nothing can mend are pruned

    @slice-21 @ID-MUT-293
    Scenario Outline: --prune removes an entry that is <why>, and reports it
      Given itos-cc.yaml excepts the survivor of "Board#count", and every other mutant of src/board.ts is killed
      And <how>
      And "itos-cc mutation run src/board.ts" has run since
      When I run "itos-cc mutation except --prune --json"
      Then itos-cc.yaml holds no entry for "Board#count", and no other byte of it changed
      And the removal is reported with the entry's file, function, column, original, replacement, reason and why "<why>"
      And the exit code is 0
      And "itos-cc mutation check src/board.ts --json" reports no "mutation.exception-stale" problem
      And before it, "itos-cc mutation check src/board.ts --json" gave the entry a fix naming "itos-cc mutation except --prune"

      Examples:
        | why    | how                                                   |
        | gone   | "Board#count" was deleted, or no longer has that site |
        | killed | src/board.test.ts changed since, and now kills it     |

    @slice-21 @ID-MUT-294
    Scenario: --prune removes an entry naming a source the exclude leaves out
      Given itos-cc.yaml excepts a survivor in src/port/porttest/fake.go and excludes "**/porttest/**"
      When I run "itos-cc mutation except --prune --json"
      Then itos-cc.yaml holds no entry for src/port/porttest/fake.go, still excludes "**/porttest/**", and no other byte of it changed
      And the removal is reported with why "excluded"
      And "itos-cc mutation check --json" reports no "mutation.exception-stale" problem

    @slice-21 @ID-MUT-295
    Scenario: --prune keeps an entry that --renew or an edit can mend
      Given itos-cc.yaml excepts a survivor of "Board#count", and "Board#count" changed since the entry was written
      When I run "itos-cc mutation except --prune --json"
      Then it reports nothing pruned, and the exit code is 0
      And itos-cc.yaml is byte for byte as it was

    @slice-21 @ID-MUT-296
    Scenario: --prune takes no site and no reason
      When I run "itos-cc mutation except --prune src/board.ts:7:19"
      Then the problem is "args.unexpected", with argument "src/board.ts:7:19"
      And the exit code is 80
      And "itos-cc mutation except --prune --reason x" is "flags.conflict", with flag "--reason", exit 80
      And "itos-cc mutation except --prune --renew" is "flags.conflict", exit 80

  # except-from-identity, issue #66: a survivor a CI mutation run reports
  # (the per-push code proof's mutation run --count ... --json) could be
  # excepted only by hand: mutation except needs a fresh snapshot of its
  # function, from a local mutation run, which agents may not run here.
  # mutation except now also takes the survivor's identity as --count's JSON
  # prints it, matches it against the source as it is, by file, function name,
  # hash and site, and writes the entry a cached except writes, running
  # nothing. The reviewer's judgement that it is equivalent is the gate, as
  # before. A fresh snapshot that records the mutant killed or uncovered
  # still refuses it. Both drivers run every scenario except MUT-318, whose
  # Given runs a counted run and a complete run (Go). Before
  # except-from-identity the identity is args.invalid, exit 80, failing at
  # the first Then that reads itos-cc.yaml or the problem's rule.
  # The person (2026-10-24): one identity at a time, never a whole report,
  # each exception with its own reason; a counted run's survivor line names
  # its identity and the mutation.survived problem's fix spells out the
  # except command, since itos ci run prints a problem's rule and message,
  # never the --json, so the log is all an agent has (ID-MUT-324); and an
  # identity does not say the mutant survived, a killed one printing the
  # same, so the reviewer's approval is the gate (the coordinator, the person
  # reviewing the list weekly), the next complete run marking a wrong entry
  # stale as killed. The coordinator's calls: the function's hash is the one
  # checked, the site matched by its whole key; a missing or stale snapshot
  # is ignored and a fresh one recording the mutant killed or uncovered
  # refuses; the entry is the cached except's, field for field; output,
  # --json and exits are unchanged; the identity's file is read from the
  # project root; exception.identity-stale, exit 80, names why, changed or
  # gone.
  Rule: A survivor a CI report names is excepted from its identity, with no mutation run

    @slice-41 @ID-MUT-318
    Scenario Outline: An identity from a counted run is excepted with <snapshot>
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      And <snapshot>
      And "itos-cc mutation run --count 5 --json" reported the survivor `<` → `<=` at board/board.go:5:11 with its identity
      When I except that identity with --reason 'x is never 5'
      Then itos-cc.yaml has one entry under mutation.exceptions
      And it records board/board.go, "example.com/fx/board#Count", the identity's hash, line_in_function 2, column 11, `<`, `<=`, the reason and line_text "return x < 5"
      And the exit code is 0
      And once "itos-cc mutation run board/board.go" has recorded the survivor, "itos-cc mutation except board/board.go:5:11 --reason 'x is never 5'" says "excepted again" and leaves itos-cc.yaml byte for byte as it was
      Examples:
        | snapshot                                                                         |
        | .metrics/mutate holds no snapshot of board/board.go                              |
        | a snapshot of board/board.go is stale, a later commit having changed board/board_test.go |

    @slice-41 @ID-MUT-319
    Scenario: Excepting from an identity runs no command
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      And .metrics/mutate holds no snapshot of board/board.go
      When I run "itos-cc mutation except 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --reason 'x is never 5'" with no Go toolchain on the PATH
      Then its standard output says "excepted board/board.go:5:11 `<` → `<=` in example.com/fx/board#Count in itos-cc.yaml"
      And the exit code is 0
      And no test or coverage command ran, and .metrics/mutate was not written
      And with --json it prints {"exception": the entry itos-cc.yaml holds, "replaced": true}
    @slice-41 @ID-MUT-320
    Scenario: An identity whose function only moved since the report is still excepted
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      And a blank line was added above Count's comment since the report
      When I run "itos-cc mutation except 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --reason 'x is never 5'"
      Then its standard output says "excepted board/board.go:6:11 `<` → `<=` in example.com/fx/board#Count in itos-cc.yaml"
      And it records board/board.go, "example.com/fx/board#Count", the identity's hash, line_in_function 2, column 11, `<`, `<=`, the reason and line_text "return x < 5"
      And the exit code is 0

    @slice-41 @ID-MUT-321
    Scenario Outline: An identity the source no longer has is refused, naming why
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      And <change>
      When I run "itos-cc mutation except '<identity>' --reason 'x is never 5' --json"
      Then the problem is "exception.identity-stale", with file "<file>"
      And its "why" is "<why>", its "identity" the argument, and its fix names a report of the code as it is now
      And the exit code is 80
      And itos-cc.yaml was not written
      Examples:
        | change                             | identity                                                                 | file           | why     |
        | Count's "x < 5" became "x < 6"     | board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=   | board/board.go | changed |
        | Count was renamed Below            | board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=   | board/board.go | gone    |
        | nothing changed                    | board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:36:<><=   | board/board.go | gone    |
        | nothing changed                    | board/gone.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=    | board/gone.go  | gone    |

    @slice-41 @ID-MUT-322
    Scenario: An identity needs a reason, as a site does
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      When I run "itos-cc mutation except 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --json"
      Then the problem is "flags.value-missing", with flag "--reason"
      And the exit code is 80
      And given "--reason ''" it is "flags.value-invalid", exit 80, and given --renew "args.unexpected", exit 80
      But given "--reason 'x is never 5'" it writes the entry, exit 0
    @slice-41 @ID-MUT-323
    Scenario: A fresh snapshot that records the mutant killed still refuses its identity
      Given a committed Go module whose test also checks Count(5), so `<` → `<=` is killed
      And "itos-cc mutation run board/board.go" recorded it killed
      When I run "itos-cc mutation except 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --reason 'x is never 5' --json"
      Then the problem is "exception.no-survivor", with file "board/board.go"
      And its message says the mutant is recorded killed, at board/board.go:5:11
      And the exit code is 80

    @slice-41 @ID-MUT-324
    Scenario: A counted run's survivor names its identity and the except command that takes it
      Given a committed Go module whose "example.com/fx/board#Count" survives its test
      When I run "itos-cc mutation run --count 5"
      Then its standard error names the survivor `<` → `<=` at board/board.go:5:11 with its identity "board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><="
      And with --json, the "mutation.survived" problem's fix names "itos-cc mutation except 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --reason"

  # integration-coverage-python, integration-coverage-typescript and
  # integration-coverage-kotlin, q-41 (answered 2026-10-10): issue #14's
  # two parts, integration coverage (ID-MUT-117 to 120) and per-test
  # coverage of listed tests (ID-MUT-121 to 129), Go only until now, for the
  # other languages, each through its own collector, the project's own,
  # never downloaded:
  # - Python: coverage.py measures the Python processes a test starts
  #   (subprocess patching), and its data files are combined.
  # - TypeScript: NODE_V8_COVERAGE names a directory the Node processes a
  #   test starts write V8 coverage to, and the project's c8 reports it.
  # - Kotlin: the JaCoCo agent from the build's Gradle or Maven cache; a
  #   harness opts in, as Go's builds with -cover, by starting the JVMs its
  #   tests run with the agent itos-cc names in ITOS_CC_JACOCO_AGENT,
  #   writing under ITOS_CC_JACOCO_DESTDIR (the coordinator's names), and
  #   jacococli merges and reports what was written.
  # As in Go, the data lives under the run's own run-* directory and is
  # merged beside the in-process coverage, each covered mutant's "coverage"
  # says "in-process", "integration" or both, and a project whose processes
  # write nothing behaves as before. Per-test coverage of listed tests sets
  # the same collector to <ITOS_CC_TEST_COVERDIR>/<test-id> for a harness
  # that splits it, else runs each listed test alone with a collector of
  # its own (ID-MUT-123). A collector the project lacks (c8, the JaCoCo
  # agent or jacococli, a coverage.py without subprocess support) is
  # coverage.tool-missing under --fail-uncovered where that language has
  # mutants to judge, and otherwise a log line, the run going on without
  # integration coverage, as today.
  Rule: Coverage from tests that start other processes, in every language

    @integration-coverage-python @ID-MUT-228
    Scenario: Lines a Python test reaches through a subprocess are covered
      Given a Python project whose only test runs its module in a subprocess and checks one branch of its output
      When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
      Then the lines the subprocess ran are covered, with "coverage" listing "integration", and their mutants run
      And a mutant the test notices is killed, one it does not notice survives, and only mutants on lines never run are uncovered
      And with mutation.tests listing two tests that run different branches, each line is covered by the IDs of the tests that reached it

    @integration-coverage-typescript @ID-MUT-229
    Scenario: Lines a TypeScript test reaches through a child Node process are covered
      Given a TypeScript project with Vitest and c8 installed whose only test runs its CLI in a child Node process and checks one branch of its output
      When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
      Then the lines the child process ran are covered, with "coverage" listing "integration", and their mutants run
      And a mutant the test notices is killed, one it does not notice survives, and only mutants on lines never run are uncovered
      And with mutation.tests listing two tests that run different branches, each line is covered by the IDs of the tests that reached it
      But without c8 the run logs that integration coverage needs it and goes on, and a strict run fails with "coverage.tool-missing"

    @integration-coverage-kotlin @ID-MUT-230
    Scenario: Lines a Kotlin test reaches through a JVM it starts are covered
      Given a Kotlin project built with Gradle whose only test starts its main class in a JVM with the agent ITOS_CC_JACOCO_AGENT names, and checks one branch of its output
      When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
      Then the lines the started JVM ran are covered, with "coverage" listing "integration", and their mutants run
      And a mutant the test notices is killed, one it does not notice survives, and only mutants on lines never run are uncovered
      And with mutation.tests listing two tests that run different branches, each line is covered by the IDs of the tests that reached it
      But a test that starts its JVM without the agent leaves those lines uncovered, as before

  # Bug fixes before the CLI-guidance refactor (coordinator's routine
  # calls, 2026-10-11, each from the precedent named):
  # - unreached-baseline-not-run: a baseline that never ran is "not-run",
  #   as fail-fast already reports one, never "passed".
  # - counted-unreached-listed: a counted run judges a file no own test
  #   reaches by the listed tests that reach its lines, as a complete run
  #   does, instead of failing for a clean baseline it never needed.
  # - python-parallel-data: a project whose coverage.py config writes
  #   parallel data files (parallel = true, or patch = subprocess) is
  #   measured: itos-cc combines the run's in-process data files before it
  #   reports them, as it combines the subprocess ones. Under the project's
  #   own patch = subprocess, the processes a test starts write beside the
  #   in-process data, where nothing tells them apart, and count as
  #   in-process.
  # - mutate-workers-abandoned: worker copies a killed mutation run left in
  #   the temporary directory are removed by a later run, as coverage
  #   removes abandoned run- directories, never one still in use. A run
  #   holds a lock on owner.lock in its directory, which the OS releases
  #   when the process dies (flock, LockFileEx); a directory with no marker,
  #   from an older itos-cc, goes once unchanged for three days.
  # - check-removed-operator-sites: the reverse of ID-MUT-106. A recorded
  #   mutant whose site the function no longer holds, as when a newer
  #   itos-cc removes an operator, is not in the code: mutation check
  #   leaves it out of the verdict, and mutation run drops it from the
  #   entry, reusing the rest.
  # - renamed-snapshot-left-behind: a snapshot whose source file is gone
  #   is an orphan; a mutation run removes the orphans under the paths it
  #   was given, and mutation check names them, never as a function result.
  # - outside-root-snapshot-kept (q-45): a file outside the project root
  #   has its snapshot named by its absolute path, which maps back to a
  #   path under the root where nothing is. Whether a snapshot is an orphan
  #   is judged by the file the snapshot records, so one outside the root
  #   is no orphan while its file exists. Nothing else about outside-root
  #   runs changes.
  # - renamed-orphan-kept (q-46): a snapshot whose file git shows renamed
  #   to a file that exists, in the --since range (REF...HEAD) or
  #   uncommitted against HEAD (staged, as git mv leaves it), is no orphan:
  #   run keeps it and check does not name it, so the --since run over the
  #   new path still moves it. Before renamed-snapshot-left-behind it
  #   stayed until such a run; that fix removed it first. A file git shows
  #   deleted, or renamed to a path where nothing is, is still an orphan.
  #   Git sees a rename only once the new file is tracked: after a plain mv
  #   with nothing staged it sees a deletion and an untracked file, so the
  #   old snapshot is still an orphan and removed. The help, README and
  #   docs/CLI.md say so, and tell people to stage a move (git mv or git
  #   add) before a run.
  # - reused-baseline-not-run (coordinator's call, from the precedent of
  #   --fail-fast and ID-MUT-231): a file's "baseline" in mutation run
  #   --json says whether its own tests' baseline ran in this run. A file
  #   whose mutants were all reused from its snapshot, and whose own tests'
  #   command no other file of the run ran, ran no baseline: "not-run",
  #   where it said "passed". Files sharing a command (a Go package's)
  #   share its baseline: one ran for any of them is "passed" for all.
  #   Snapshots record no baseline, so nothing reads stale.
  # - check-left-out-note (coordinator's call): mutation check says what
  #   ID-MUT-235 leaves out. A function whose fresh entry records mutants at
  #   sites it no longer holds gets, in plain output, a line "left out
  #   <file>:<line> in <namespace#name>: <n> recorded mutants at sites gone,
  #   which the next mutation run drops", and in --json "left_out": <n> in
  #   its "functions" entry, present only when n > 0 so output without one
  #   keeps its keys (the "orphans" precedent). It is a note, not a
  #   problem: it changes no verdict and no exit code.
  Rule: Mutation results say what happened, and leave nothing stale behind

    @unreached-baseline-not-run-fix @ID-MUT-231
    Scenario: A file no test reaches reports its baseline as not run
      Given a Python project with a file no test reaches
      When I run "itos-cc mutation run --json" for that file
      Then its "baseline" is "not-run" and its mutants are uncovered
      And a file whose tests ran reports its baseline as "passed", as before

    @counted-unreached-listed-fix @ID-MUT-232
    Scenario: A counted run judges an unreached file by the listed tests that reach it
      Given a committed Python project whose file no own test reaches, but a listed test reaches one of its lines and kills its mutant
      When a count-one run judges that file
      Then its mutant is judged by the listed test and killed, as a complete run judges it
      And no "no clean baseline was prepared" problem is reported

    @python-parallel-data-fix @ID-MUT-233 @binary-only
    Scenario: A Python project that writes parallel coverage data is measured
      Given a Python project whose coverage.py configuration sets parallel = true, with a test that kills its mutant
      When I run "itos-cc mutation run --json" for its file
      Then its coverage is measured, not "coverage.measured-nothing", and its mutant is killed
      And nothing of the run's data files is left in the project's tree

    @mutate-workers-abandoned-fix @ID-MUT-234 @binary-only
    Scenario: A later run removes the worker copies a killed run left behind
      Given worker copies a mutation run left in the temporary directory when it was killed
      And worker copies of a mutation run still going
      When I run "itos-cc mutation run" for a file
      Then the killed run's worker copies are removed
      And the running one's are left alone

    @check-removed-operator-sites-fix @ID-MUT-235
    Scenario: A recorded mutant whose site is gone is left out
      Given fresh results for "Board#place" that record a surviving mutant at a site the function no longer holds, as when a newer itos-cc removes an operator, and every other mutant killed
      When I run "itos-cc mutation check src/board.ts"
      Then no "mutation.survived" is reported for that mutant, and the exit code is 0
      And "itos-cc mutation run src/board.ts" runs no mutant, and rewrites the entry without it

    @renamed-snapshot-left-behind-fix @ID-MUT-236
    Scenario: A snapshot whose source file is gone is removed by the next run over its directory
      Given a snapshot under .metrics/mutate for src/old.ts, a file that no longer exists
      When I run "itos-cc mutation check src"
      Then it names the orphaned snapshot, without failing any function for it
      When I run "itos-cc mutation run src"
      Then the orphaned snapshot is removed, and the snapshots of files that exist are kept

    @outside-root-snapshot-kept-fix @ID-MUT-237
    Scenario: A snapshot of a file outside the project root is kept while its file exists
      Given a snapshot that "itos-cc mutation run" wrote for a file outside the project root, which still exists
      When I run "itos-cc mutation check" at the project root
      Then it names no orphan
      When I run "itos-cc mutation run" at the project root
      Then that snapshot is kept

    @renamed-orphan-kept-fix @ID-MUT-238
    Scenario: A --since run whose paths miss a committed rename's new path keeps the old snapshot
      Given src/old.ts, with a snapshot, renamed to lib/new.ts in a commit since HEAD~1
      When I run "itos-cc mutation run --since HEAD~1 src"
      Then the snapshot of src/old.ts is kept, and no orphan is named
      When I run "itos-cc mutation run --since HEAD~1 lib"
      Then the snapshot moves to lib/new.ts, and src/old.ts's is gone

    @renamed-orphan-kept-fix @ID-MUT-239
    Scenario: A run before a rename is committed keeps the old snapshot
      Given src/old.ts, with a snapshot, renamed to lib/new.ts with git mv and not committed
      When I run "itos-cc mutation run src"
      Then the snapshot of src/old.ts is kept, and no orphan is named
      And "itos-cc mutation check src" names no orphan
      And once the rename is committed, "itos-cc mutation run --since HEAD~1 lib" moves the snapshot to lib/new.ts

    @reused-baseline-not-run-fix @ID-MUT-240
    Scenario: A file whose mutants are all reused reports its baseline as not run
      Given a file whose snapshot is fresh, so that a run reuses every one of its mutants
      When I run "itos-cc mutation run --json" for that file
      Then its "baseline" is "not-run", and it ran no mutant
      And after an edit to one of its functions, the next run reports its "baseline" as "passed"

    @check-left-out-count @ID-MUT-241
    Scenario: mutation check says how many recorded mutants it left out
      Given fresh results for "Board#place" that record a killed mutant at a site the function no longer holds, and every other mutant killed
      When I run "itos-cc mutation check src/board.ts --json"
      Then the "functions" entry of "Board#place" has "left_out" 1, and no other function has "left_out"
      And the exit code is 0
      And "itos-cc mutation check src/board.ts" prints a "left out" line naming "Board#place" and 1 mutant

  # strict-go-coverage, issue #23, q-25/q-26 and ADR-0017/0018:
  # - --fail-uncovered judges positive-weight Go executable coverage blocks
  #   in every selected, judged function, including functions with no sites.
  #   Empty/comment-only bodies have no executable obligation; prove that
  #   from source, not from the absence of a file's coverage records.
  # - Different measured column spans on one line remain distinct. Matching
  #   spans from successful built-in/integration/listed measurements are
  #   covered if any measured copy executed them. Function literals belong
  #   to their enclosing named Go function, as today's Go unit model does.
  #   Comments, braces and zero-weight records add no separate obligations.
  # - Persist a complete per-function block inventory and its independent
  #   provenance, including a measured-complete inventory. Missing, null,
  #   partial or legacy evidence is not proof of complete coverage.
  # - Evidence belongs to its recorded coverage producer, not to mutant
  #   scopes: --test-command alone does not change the coverage command.
  #   Fingerprint producer/options, relevant Go build environment, module
  #   .go source/test files (build-tagged files included, nested modules
  #   excluded), go.mod/go.sum and project config/support matches. Compare
  #   additions, removals and edits; ignore only the tool-owned annotation
  #   block to avoid making a run's own summary stale its evidence.
  # - Go raw-profile workflows (--coverage-report, --use-existing-coverage
  #   and --coverage-command) conflict with strict mode in this slice. They
  #   retain non-strict behavior. Do not attach today's fingerprints to an
  #   old, failed or unrelated measurement. Opaque inputs require support
  #   globs and are not inferred. No new attestation/sidecar format.
  # - q-27/ADR-0019 refuse an active Go workspace and a local replacement
  #   outside the actual inventoried source module in strict run/check.
  #   An excluded nested module is outside that inventory even if its path
  #   is lexically inside the source tree. Do not follow external trees to
  #   make the scope look supported. Existing evidence cannot bypass the
  #   refusal. GOWORK=off and inputs already in the inventory stay supported;
  #   non-strict commands retain their existing behavior. Report refusal as
  #   mutation.coverage-unsupported (exit 1), with file/function/line, naming
  #   the workspace or replacement responsible in its message.
  # - A fresh independent cache may avoid coverage measurement, including
  #   evidence from a broader recorded suite. Missing/stale evidence makes
  #   a strict run measure coverage even with no pending mutation sites.
  #   Non-strict/no-coverage runs must not manufacture or refresh evidence.
  # - check runs no tests/coverage/list command and writes nothing. Strict
  #   check reports mutation.coverage-missing or mutation.coverage-stale
  #   with file/function/line and exits 1 when it cannot trust the evidence.
  #   The stale message names changed inputs. A run that cannot establish
  #   complete evidence reports missing evidence, never invents covered or
  #   uncovered blocks; actual measurement errors retain CLI exit categories.
  # - Each uncovered span is mutation.uncovered-statement (exit 1), with
  #   current file, function and line. Optional column precision must not be
  #   lost internally. These findings have no original/replacement and do
  #   not change mutant counts. Keep mutation.uncovered for actual mutants,
  #   even when that also produces a statement finding. Survivor exceptions
  #   do not excuse uncovered statements. No new coverage exceptions.
  # - --since/paths keep today's judgment selection. Preserve original
  #   evidence for unjudged functions, never bless it with newly measured
  #   file-wide fingerprints. Pair by existing name/hash identity (including
  #   repeated init functions); diagnose current positions after movement.
  #   File/package moves that change measurement inputs stale coverage.
  # - Non-strict zero-site/reuse behavior, mutant counts, ordinary graph and
  #   sample freshness, survivor exceptions and other-language coverage
  #   behavior remain unchanged. The --no-coverage/--fail-uncovered usage
  #   conflict is universal, and is checked even for empty selections.
  Rule: Strict Go checks prove executable coverage without mutation sites

    @strict-go-coverage @ID-MUT-156
    Scenario: A killed mutant does not excuse an uncovered statement without a site
      Given a Go function with a tested comparison whose mutants are killed and an unexecuted identifier-conditioned branch returning a string with no mutation site
      When I run mutation run with --fail-uncovered --json for its source file
      Then the problem is "mutation.uncovered-statement", with file, function and the unexecuted block's line
      And no original or replacement is invented for that problem
      And the function's mutant counts remain killed mutants only, with zero uncovered mutants
      And the exit code is 1
      When I run mutation check with --fail-uncovered --json for the file
      Then it reports the same uncovered statement and exits 1 without running commands or writing files

    @strict-go-coverage @ID-MUT-157
    Scenario: Executable zero-site functions need coverage while empty bodies do not
      Given a Go function containing no mutation sites whose identifier-conditioned branch is never executed
      And an executable zero-site function in a package no test loads
      When I run mutation run with --fail-uncovered for each source
      Then each unexecuted executable block is an uncovered-statement failure
      But a strict run and cached check of a fully executed zero-site function both pass
      And empty or comment-only function bodies add no coverage obligation
      And a non-strict check of a zero-site function still needs no snapshot entry

    @strict-go-coverage @ID-MUT-158
    Scenario: Covered blocks cannot hide unexecuted same-line or closure blocks
      Given a Go function has covered and uncovered positive-weight blocks with distinct column spans on the same source line
      And another named Go function contains a never-called function literal
      When I run mutation run with --fail-uncovered
      Then the same-line uncovered span fails despite the covered span on that line
      And the literal's uncovered block is attributed to its enclosing named function
      And comments, brace-only lines and zero-statement-weight blocks create no separate findings

    @strict-go-coverage @ID-MUT-159
    Scenario: Reused mutants cannot bypass missing or stale coverage evidence
      Given a non-strict no-coverage run recorded all mutants killed and no independent statement evidence
      When I run mutation run with --fail-uncovered and fresh inputs
      Then coverage is measured even though those mutants can be reused
      And current independent evidence is recorded for every judged executable Go function
      When I repeat the strict run and cached check without any input change
      Then the fresh evidence can be reused without another coverage measurement
      And cached check runs no test, list or coverage command and writes nothing
      But stale independent evidence makes a strict run measure again even if its mutant outcomes remain reusable

    @strict-go-coverage @ID-MUT-160
    Scenario Outline: Incomplete coverage evidence cannot pass strict check
      Given a Go function's mutation outcomes are fresh but its independent coverage evidence is <evidence>
      When I run mutation check with --fail-uncovered --json
      Then its problem is "mutation.coverage-missing", with file, function and line
      And the exit code is 1
      And no test, list or coverage command runs and no file is written

      Examples:
        | evidence                                             |
        | absent in a legacy snapshot                           |
        | null                                                 |
        | missing its executable function's block inventory     |
        | supplied only by measurement of an unrelated Go module |

    @strict-go-coverage @ID-MUT-161
    Scenario Outline: Coverage depends on source and measurement inputs independently of mutants
      Given fresh strict coverage evidence for an unchanged Go function
      And a <input> input was <change> without changing that function
      When I run mutation check with --fail-uncovered --json for the function's file
      Then its problem is "mutation.coverage-stale", naming the changed input
      And the exit code is 1
      And a later strict run cannot bless the old profile with new fingerprints

      Examples:
        | input                          | change  |
        | module production helper       | changed |
        | non-importing module test      | added   |
        | configured support file        | removed |
        | module go.mod                  | changed |
        | recorded coverage build option | changed |

    @strict-go-coverage @ID-MUT-162
    Scenario: Partial judgment and moved functions preserve coverage identity
      Given two Go functions have fresh independent coverage evidence
      And a module test changed since, making both inventories stale
      And only one function changed in commits since "base"
      When I run mutation run with --since base --fail-uncovered
      Then only the changed function is judged and gets newly measured evidence
      And the unjudged function keeps its original stale evidence
      And strict check of the unjudged function cannot pass
      And moving a function down within its file reports uncovered blocks at their current lines after current measurement
      And repeated init functions retain the correct separate inventories when paired by name and hash

    @strict-go-coverage @ID-MUT-163
    Scenario: Strict coverage validates flags and does not widen other modes
      When I combine --fail-uncovered with --no-coverage, even for an empty selection
      Then the problem is "flags.conflict" for "--no-coverage" and the exit code is 80
      When strict Go run is combined with --coverage-report, --use-existing-coverage or --coverage-command
      Then the problem is "flags.conflict" naming the incompatible report flag and the exit code is 80
      And no report, coverage command or mutant is consumed to bless strict evidence
      But non-strict explicit-report and no-coverage runs retain their current behavior
      And other-language statement coverage, ordinary graph/sample freshness and survivor exceptions are unchanged
      And a survivor exception cannot excuse a strict Go uncovered-statement finding

    # Dependency following/admission excludes ordinary discovery inside the
    # project: the same nested module is discovered without a replace directive.
    # The separate outside-root target is observed through both refusals.
    @strict-go-coverage @ID-MUT-164
    Scenario: Unsupported module scopes cannot reuse strict coverage evidence
      Given a Go function has fresh strict coverage evidence
      When an active go.work is introduced for its measurement environment
      Then strict run and cached strict check fail with "mutation.coverage-unsupported", naming the workspace
      And cached check does not run coverage or tests or refresh its evidence
      But with GOWORK=off the same inventoried module remains supported
      When its go.mod names a local replacement outside the inventoried module
      Then strict run and cached strict check fail with "mutation.coverage-unsupported", naming the replacement
      And a replacement beneath an excluded nested module is refused too
      And no external dependency tree is followed or admitted as fresh coverage
      But replacement inputs already inside the actual inventory remain supported
      And non-strict commands retain their existing behavior

    # Issues #36 and #37 (slice-10): strict Go evidence recorded in one
    # checkout read coverage-stale for every function in any other, CI's
    # included. @go-env hashed the raw variables, so the GOTOOLCHAIN=local
    # setup-go exports differed from an unset one though the same Go built
    # the same code; and the inputs were every .go file on disk beneath the
    # module, inside .git/, testdata and _ directories, untracked and ignored
    # ones included. Now @go-env holds the values go env resolves, without
    # GOTOOLCHAIN, which only says how the version was chosen; the files are
    # go.mod, go.sum and the .go files git tracks beneath the module that a
    # build can include, never under testdata or a directory whose name
    # starts with _ or ., nor in a nested module; and the Go version is the
    # project's toolchain's, go env GOVERSION, not the one itos-cc was built
    # with, which no scenario can hold, having one Go. Shipping it changes the
    # inputs of all recorded Go evidence, which reads coverage-stale once
    # until a strict run measures it again; mutant outcomes stay fresh.
    # Outside a git repository the files are those go's own directory
    # rules take from the disk.
    # The fixture's go.mod names a Go no newer than the one installed, so
    # local and auto resolve the same toolchain and nothing is downloaded.
    @slice-10 @ID-MUT-254
    Scenario Outline: Strict Go evidence stays fresh when only how the same Go is chosen differs
      Given fresh strict coverage evidence for an unchanged Go function, recorded with <recorded>
      When I run mutation check with --fail-uncovered --json for the function's file, with <checked>
      Then no function is "mutation.coverage-stale" and the exit code is 0
      But with GOFLAGS=-buildvcs=false its problem is "mutation.coverage-stale", naming "@go-env"

      Examples:
        | recorded          | checked                      |
        | GOTOOLCHAIN unset | GOTOOLCHAIN=local            |
        | GOTOOLCHAIN unset | GOTOOLCHAIN=auto             |
        | GOTOOLCHAIN=local | GOTOOLCHAIN unset            |
        | GOOS unset        | GOOS set to the system's own |

    @slice-10 @ID-MUT-255
    Scenario Outline: A Go file no build of the checkout includes is no input of strict Go evidence
      Given fresh strict coverage evidence for an unchanged Go function, its snapshot committed
      And a Go file <file> was added, <how>
      When I run mutation check with --fail-uncovered --json for the function's file
      Then no function is "mutation.coverage-stale" and the exit code is 0
      But a committed Go file added beside the function's own file makes its problem "mutation.coverage-stale", naming that file

      Examples:
        | file          | how                                       |
        | .git/x/x.go   | inside the git directory                  |
        | _t/x.go       | untracked, in a directory go ignores      |
        | dist/x.go     | in a directory .gitignore ignores         |
        | testdata/x.go | committed, in a testdata directory        |
        | scratch.go    | untracked, beside the function's own file |

  # Issue #26's ownership prerequisite is split by the person's instructions:
  # T-9 extracts legacy execution; T-10 exposes internal context and lifecycle
  # seams; mutation-command-ownership implements Linux guarantees here;
  # mutation-windows-ownership implements and verifies Windows on a Windows
  # machine later. ADR-0021 still requires both platforms before fail-fast.
  # This split keeps the original Windows gap, not a cross-platform completion
  # claim after Linux tests pass. Windows scenarios remain @wip until verified.
  # The person now prioritizes fresh bounded execution in GitHub #27 over
  # fail-fast #26. Linux ownership is shared groundwork for #27 too; it does
  # not introduce #26's scheduler or flag. Windows work is tracked separately
  # in #29 and stays deferred until a Windows machine is used.
  # Local mutation execution must stay sparse: never mutate this repository's
  # sources, and use only minimal targeted fixture trials on this machine.
  # Exercise helper-process ownership directly where possible; broader mutant
  # trial matrices and full mutation-fixture suites run in CI, not locally.
  # q-35 chose the separate prerequisite after a throwaway spike. Its new
  # internal capabilities are not original-product behavioral-red evidence.
  # Against T-10, each ownership assertion must compile and show an actual
  # behavior failure before its implementation; passing guards are separate.
  # Completed nonzero exits as well as success must survive later cancellation;
  # later cancellation must not overwrite an already observed own timeout.
  # Real lifecycle tests cover cleanup on acquisition/start/wait errors, not
  # just the spike's successful post-Wait cleanup hook. The spike proves no
  # descendant containment, output-drain guarantee or Windows Job ownership.
  # Delivery evidence: cab1f76 committed timeout, normal-return and startup
  # checks before d30f99e. Only normal return demonstrated original behavioral
  # red (WaitDelay expired before I/O complete); timeout/startup were passing
  # guards. Cancellation, exit-precedence and cleanup-error tests arrived after
  # implementation, despite q-35's requirement; do not claim full red-first
  # chronology or relabel older-revision checks as original evidence.
  # Coordinator review found that normal cleanup consumed the shared run-abort
  # budget. 5fcd83c demonstrated actual new red for normal completion and own
  # timeout against d30f99e; f779e4c reserves the shared deadline for parent
  # aborts. All these local checks used helper processes, zero mutant trials.
  # - Supervised commands own ordinary, non-detaching child/grandchild
  #   processes. Windows uses a kill-on-close Job Object with ownership
  #   established before children can escape; Unix uses a process group.
  #   Intentionally daemonizing/session-escaping Unix commands are outside
  #   this contract, not a hidden promise of container-strength isolation.
  # - A command scope closes on normal completion, mutant deadline, parent
  #   cancellation or execution error. Terminate remaining owned processes
  #   and join output/process waiters before returning or restoring/removing
  #   its worker copy. Bound cleanup with one five-second deadline; later
  #   fail-fast callers share that deadline across scopes, not per worker.
  # - A mutant's own deadline still means timeout/killed. Parent/run abort
  #   is distinct cancellation, never a successful kill/timeout judgment.
  #   Actual completed exit status must not be replaced by a later abort.
  # - Startup, containment or cleanup failures return explicit errors; do
  #   not silently run unsupervised, record a kill or erase the error.
  #   Target only captured owned handles/groups, never command-name matches
  #   or unrelated orphaned processes. Ownership establishment must be
  #   race-safe on Windows, not assignment after an unrestricted launch.
  # - Provide reusable context-aware supervision for the next slice's
  #   mutation, baseline, selection, list and coverage command paths. Worker
  #   timeout/normal-return cleanup is verified here; fail-fast scheduling
  #   and mode-specific preparation integration remain the next slice.
  # - This slice exposes no public --fail-fast flag and changes no aggregate
  #   scheduling, mutation scope, valid outcome/count, freshness or baseline
  #   timeout policy. Ordinary worker execution must retain its valid results
  #   while no longer leaving owned descendants behind. Sample's shared
  #   execution path must retain its contract too.
  Rule: Linux mutation commands return only after their owned process trees are cleaned up

    @binary-only @mutation-command-ownership @ID-MUT-165
    Scenario: A mutant deadline cleans up children and grandchildren
      Given mutation commands run on Linux
      And a test command starts ordinary owned child and grandchild processes that keep running and hold output open
      When the mutant reaches its own timeout
      Then all owned processes are terminated and output waiters are joined before the command returns
      And the mutant is still counted killed by timeout
      And cleanup is bounded and the worker copy can be restored and removed
      And unrelated processes remain untouched

    @binary-only @mutation-command-ownership @ID-MUT-166
    Scenario: Normal command completion also closes owned descendants
      Given mutation commands run on Linux
      And a baseline or mutant test command starts an ordinary owned descendant and then exits successfully
      And that descendant could continue work after the parent exits, even with its output streams closed
      When the command scope returns
      Then the descendant has stopped and cannot continue fixture work after return
      And the parent's actual successful exit is retained, not invented as timeout or a failed baseline
      # Adapter ordering contract for MUT166, preserved verbatim:
      # And all owned process/output waiters are joined before worker copy removal
      And after the run returns, its owned fixture peers have terminated and worker copies are removed

    @binary-only @mutation-command-ownership @ID-MUT-167
    Scenario: Parent cancellation is not a mutant deadline
      Given mutation commands run on Linux
      And a supervised command and its owned child are running under a parent cancellation context
      When the parent cancels without the mutant's own deadline expiring
      Then command admission stops and the owned tree is terminated within the shared cleanup deadline
      And process/output waiters are joined before return
      And the result identifies cancellation, not a killed or timed-out mutant
      But a judgment completed before the cancellation retains its actual result

    # Adapter contract (original public prose retained verbatim).
    # Linux private lifecycle faults remain held by command_runner_ownership_linux_test.go and shared Unix helpers
    # T88 moves them intact, T101 never deletes them as CLI duplicates.
    # @mutation-command-ownership @ID-MUT-168
    # Scenario: Supervision failures do not permit an unsafe fallback
    #   Given mutation commands run on Linux
    #   And a supervised command cannot establish ownership, start or finish cleanup safely
    #   When its execution is attempted
    #   Then the corresponding error is returned rather than a passing or killed judgment
    #   And no command runs unrestricted after an ownership-establishment failure
    #   And only captured owned handles or groups can be targeted for cleanup
    #   And the same supervisor can be used by worker and preparation commands without changing their argument, environment or output contracts
    #   And intentionally detaching Unix commands are documented as unsupported, not claimed to be contained

  # Windows is a separate, pending part of ADR-0021. Implement and exercise
  # these guarantees on a Windows machine, not by cross-compilation alone.
  # Preserve existing Windows command behavior while shared prerequisites and
  # Linux work land. No Windows Job implementation is required in those items.
  # Race-safe ownership must precede any unrestricted child execution; assigning
  # an already running command to a Job is not sufficient. A Job setup failure
  # must never fall back to an unrestricted launch. The shared five-second
  # cleanup, accurate outcomes, waiter joins and no-unrelated-kills rules above
  # apply here too. No public fail-fast flag is introduced by either platform.
  Rule: Windows mutation commands return only after their owned process trees are cleaned up

    @wip @mutation-windows-ownership @ID-MUT-169
    Scenario: A Windows mutant deadline cleans up its owned Job tree
      Given mutation commands run on Windows
      And a test command starts ordinary owned child and grandchild processes that keep running and hold output open
      When the mutant reaches its own timeout
      Then all owned processes are terminated and output waiters are joined before the command returns
      And the mutant is still counted killed by timeout
      And cleanup is bounded and the worker copy can be restored and removed
      And unrelated processes remain untouched

    @wip @mutation-windows-ownership @ID-MUT-170
    Scenario: Normal Windows command completion also closes owned descendants
      Given mutation commands run on Windows
      And a baseline or mutant test command starts an ordinary owned descendant and then exits successfully
      And that descendant could continue work after the parent exits, even with its output streams closed
      When the command scope returns
      Then the descendant has stopped and cannot continue fixture work after return
      And the parent's actual successful exit is retained, not invented as timeout or a failed baseline
      And all owned process/output waiters are joined before worker copy removal

    @wip @mutation-windows-ownership @ID-MUT-171
    Scenario: Windows parent cancellation is not a mutant deadline
      Given mutation commands run on Windows
      And a supervised command and its owned child are running under a parent cancellation context
      When the parent cancels without the mutant's own deadline expiring
      Then command admission stops and the owned tree is terminated within the shared cleanup deadline
      And process/output waiters are joined before return
      And the result identifies cancellation, not a killed or timed-out mutant
      But a judgment completed before the cancellation retains its actual result

    @wip @mutation-windows-ownership @ID-MUT-172
    Scenario: Windows Job failures do not permit an unsafe fallback
      Given mutation commands run on Windows
      And a supervised command cannot establish Job ownership, start or finish cleanup safely
      When its execution is attempted
      Then the corresponding error is returned rather than a passing or killed judgment
      And no command runs unrestricted after an ownership-establishment failure
      And ownership is established before the command can execute and spawn descendants
      And only captured owned handles can be targeted for cleanup
      And the same supervisor can be used by worker and preparation commands without changing their argument, environment or output contracts

  # macOS is the next part of ADR-0021, after v0.7.0 shipped counted mode
  # Linux-only (the person's choice, 2026-10-09). Decisions for the slice:
  # - One Unix supervisor. The Linux owned-command code (process group, output
  #   pipes, group kill, waiter joins, the run-abort-only shared five-second
  #   deadline and local deadlines for normal returns and mutant timeouts) is
  #   shared by Linux and macOS; only the probe that a killed group has no live
  #   member stays per OS. Linux keeps its /proc scan unchanged. macOS waits
  #   for kill(-pgid, 0) to report ESRCH, since launchd reaps orphaned zombies;
  #   if CI shows zombies keeping a group alive past the deadline, use the
  #   kern.proc.pgrp sysctl ignoring SZOMB entries, in the standard library if
  #   possible, and stop to propose any new module dependency. Never parse ps
  #   output or match processes by name.
  # - Counted mode (ID-MUT-173..186) is admitted on darwin and refused only on
  #   Windows (#29). Its live scenarios run on macos-latest as on ubuntu-latest,
  #   with no macOS-only weakening of an assertion: lift the Linux-only skips
  #   and build tags, using a portable process-liveness check in the steps.
  # - Behaviour on Windows stays exactly as it is. Intentionally detaching or
  #   session-escaping commands stay unsupported on macOS, as on Linux.
  # - Evidence is from CI: this machine is Linux. Red is shown by the
  #   red-first test commit's own CI run on macos-latest (a draft pull request
  #   from a branch holding only that commit, closed once seen), never claimed
  #   from Linux runs or cross-compilation.
  # - Delivery evidence: 1c5f12b's steps, with only their macOS skips lifted
  #   on a throwaway branch (draft PR #31, run 38023389204), failed on
  #   macos-latest at 188's normal return, 189's cancellation result, 190's
  #   captured-group cleanup and 191's platform refusal; 187's timeout, 189's
  #   completed-first and 190's startup/cleanup errors already held (guards).
  Rule: macOS mutation commands return only after their owned process trees are cleaned up

    @binary-only @macos-counted-run @ID-MUT-187
    Scenario: A macOS mutant deadline cleans up children and grandchildren
      Given mutation commands run on macOS
      And a test command starts ordinary owned child and grandchild processes that keep running and hold output open
      When the mutant reaches its own timeout
      Then all owned processes are terminated and output waiters are joined before the command returns
      And the mutant is still counted killed by timeout
      And cleanup is bounded and the worker copy can be restored and removed
      And unrelated processes remain untouched

    @binary-only @macos-counted-run @ID-MUT-188
    Scenario: Normal macOS command completion also closes owned descendants
      Given mutation commands run on macOS
      And a baseline or mutant test command starts an ordinary owned descendant and then exits successfully
      And that descendant could continue work after the parent exits, even with its output streams closed
      When the command scope returns
      Then the descendant has stopped and cannot continue fixture work after return
      And the parent's actual successful exit is retained, not invented as timeout or a failed baseline
      # Adapter ordering contract for MUT188, preserved verbatim:
      # And all owned process/output waiters are joined before worker copy removal
      And after the run returns, its owned fixture peers have terminated and worker copies are removed

    @binary-only @macos-counted-run @ID-MUT-189
    Scenario: macOS parent cancellation is not a mutant deadline
      Given mutation commands run on macOS
      And a supervised command and its owned child are running under a parent cancellation context
      When the parent cancels without the mutant's own deadline expiring
      Then command admission stops and the owned tree is terminated within the shared cleanup deadline
      And process/output waiters are joined before return
      And the result identifies cancellation, not a killed or timed-out mutant
      But a judgment completed before the cancellation retains its actual result

    # Adapter contract (original public prose retained verbatim).
    # macOS startup/cleanup/captured-group faults remain in TestMacOSSupervisionFailuresDoNotPermitUnsafeFallback
    # Linux zombie-only cleanup remains in TestLinuxProbeCountsAZombieOnlyGroupEmpty. T88 retains all guards.
    # @macos-counted-run @ID-MUT-190
    # Scenario: macOS supervision failures do not permit an unsafe fallback
    #   Given mutation commands run on macOS
    #   And a supervised command cannot establish ownership, start or finish cleanup safely
    #   When its execution is attempted
    #   Then the corresponding error is returned rather than a passing or killed judgment
    #   And no command runs unrestricted after an ownership-establishment failure
    #   And only captured owned process groups can be targeted for cleanup
    #   And Linux keeps its existing supervision unchanged

    @binary-only @macos-counted-run @ID-MUT-191
    Scenario: Counted execution is admitted on macOS and still refused on Windows
      Given a committed project with eligible mutation sites
      When a count-one run judges it on macOS
      Then it runs under owned supervision and reports its sampled judgment without a platform refusal
      And an interrupted macOS counted run reports partial work and cleans up as on Linux
    # Comparative contract (native public proof is ID-MUT-248):
    # But on Windows counted mode still fails clearly before launching commands
      And the help names Linux and macOS as the supported platforms
    # Documentation contract retained in TestTheDocsDescribeCountedAndFailFastPlatforms:
    # And the help, README and docs name Linux and macOS as the supported platforms

  # GitHub #27, now the person's priority, is fresh bounded execution, not
  # cached mutation sample and not #26 fail-fast. Linux ownership is shared
  # groundwork; T-11 supplies committed-input/static planning and T-12 supplies
  # supervised, fail-closed preparation before this public integration slice.
  # - Opt in with mutation run --count N; N is a positive integer and is ONE
  #   budget across every file/function/worker. --seed TEXT requires --count;
  #   its default is the resolved HEAD commit ID. Cached mutation sample stays
  #   unchanged. The initial public bounded mode is Linux-only; other platforms
  #   refuse it clearly before launching commands. Windows support is #29.
  # - Resolve one Git repository and HEAD once. Counted execution uses frozen
  #   version-controlled source/test/config/support inputs, not current dirty,
  #   staged or untracked project code. --since selects committed changed
  #   functions and explicit paths narrow that set; without --since, discover
  #   the selected committed HEAD paths. No additional checkout or session move
  #   is used. External tools/dependencies/environment remain a declared boundary,
  #   not container-strength or hermetic-build assurance. Reject unsupported
  #   repository/build scopes rather than silently executing live project code.
  # - Enumerate static sites without result-cache admission. Rank globally using
  #   SHA-256 and the TEXT seed, logical root-relative slash paths and identities
  #   distinguishing same-name units/sites, with deterministic tie-breaking.
  #   Report commit, seed and selection algorithm version. Select min(N, eligible)
  #   sites before scheduling; coverage does not silently redraw that selection.
  # - Selected covered sites run freshly even when a matching cached kill exists.
  #   Reused outcomes never spend this budget. One trial is ONE selected mutant's
  #   final judgment, including its own and applicable listed stages; baseline,
  #   list, coverage and conversion commands are not mutant trials. Reapplying
  #   that same mutant after a clean listed baseline is not a second trial.
  # - Count is an upper bound on actual trials, not a latency or test-command
  #   bound. Selected measured-uncovered sites remain selected and reported,
  #   with zero execution for those sites and no redraw. Preserve valid exception,
  #   own/listed, timeout and --fail-uncovered semantics. No applicable test
  #   command or failed/missing/malformed measurement is not a site-free success.
  #   Strict Go executable coverage, including zero-site functions, stays strict.
  # - Prepare fresh required coverage/listed reach on the frozen inputs without
  #   any local measurement/cache prerequisite. Every actual preparation command
  #   must succeed; the presence of a report never hides command/conversion/provider
  #   failures. Do not silently drop configured listed integration tests to save
  #   discovery cost. Refuse incompatible raw/existing coverage inputs; explicit
  #   no-coverage is permitted only where it does not bypass strict coverage or
  #   configured applicable listed-test discovery. Complete-mode flags retain
  #   their existing behavior. Persistent per-scenario coverage reuse is separate.
  # - This first mode publishes NO mutation snapshots, source annotations or
  #   persistent coverage cache. Completed outcomes live in the current report;
  #   existing complete-cache files remain untouched. A sampled pass proves only
  #   its actual judgments, never a new complete-cache proof. Existing genuine
  #   complete proof is a separate fact, not invalidated just by a sampled run.
  # - JSON remains one object. Add a sampling object with budget, eligible,
  #   selected, executed and omitted site counts, seed/algorithm/commit provenance,
  #   sampled assurance and actual completion/stop information. List selected
  #   site identities and outcomes, judged and omitted subjects, and actual stage
  #   states. Undecided/cancelled/blocked sites have no fabricated mutation outcome.
  #   Preserve completed progress in output on interruption/infrastructure error.
  #   Errors fail the command; genuine no-sites is explicitly not applicable.
  # - Own all counted-mode worker and preparation process trees, and join them
  #   before returning/removing private inputs. A run abort shares ONE five-second
  #   cleanup deadline; normal command completion and a mutant's own timeout must
  #   not start or consume that run-abort deadline. Ordinary aggregate counted
  #   execution does not become #26 fail-fast on a survivor.
  # - Local mutant trials stay sparse: none over this repository; at most one or
  #   two tiny selected fixture trials per focused behavioral case where needed.
  #   Static planning, raw snapshots and direct helper processes need no trials.
  #   Broader concurrency/multi-mutant regressions run in CI, not on this machine.
  #   Unknown-flag failures are CLI-admission red, not scheduler-red evidence.
  # - T-12 built the internal preparation (cmd/itos-cc fresh_preparation.go):
  #   build on it, do not duplicate it. Its strict Go inventory currently errors
  #   when an admitted executable function lacks complete evidence (a file only
  #   another OS builds, an unloaded package). Counted mode reports that as the
  #   same mutation.coverage-missing finding complete mode reports, failing the
  #   run with completed stages intact; a failed measurement command stays a
  #   preparation failure. Listed selection baselines depend on each selected
  #   mutant's reach, so the counted scheduler runs them (mutation-counted-listed),
  #   not preparation. The slice is split: this one holds ID-MUT-173 to 177, 180
  #   and 184 to 186; mutation-counted-listed and mutation-counted-interrupt hold
  #   the rest, and #27 is released only after all three land.
  # - mutation-counted-run landed (b487e3c) with an interim guard: a selected
  #   survivor of its own tests that listed tests reach is reported blocked and
  #   fails with count.listed-unsupported. mutation-counted-listed replaces that
  #   guard with the real listed stages (the selection's clean listed baseline,
  #   then the mutant under the listed tests, one trial in all) and removes the
  #   rule from the help and docs/CLI.md. It also applies valid matching
  #   exceptions to counted survivors as complete mode does (a stale one fails).
  #   Counted mode keeps refusing --no-coverage outright: that stays within the
  #   rule that it never bypasses strict coverage or listed discovery.
  # - mutation-counted-interrupt (ID-MUT-183): in counted mode SIGINT and SIGTERM
  #   (what CI cancellation sends) abort the run through one cancelled context:
  #   no new command is admitted, the active judgment ends cancelled with no
  #   mutation outcome, and every owned worker and preparation process group is
  #   killed and joined within the one shared five-second post-abort deadline
  #   before the private frozen inputs are removed. A second signal does not skip
  #   that join. The run still prints its partial report (completion
  #   interrupted, completed judgments kept) and exited 75 with the rule
  #   count.interrupted until decision 38: it now reports interrupted, exit 130,
  #   as every interrupted command does, complete runs included (ID-MUT-249 to
  #   253), and a second signal waits only while owned groups are killed.
  Rule: Fresh counted mutation judges a bounded committed selection without claiming full proof

    @mutation-counted-run @ID-MUT-173
    Scenario: Counted execution has explicit admission and platform boundaries
      Given the complete mutation run and cached mutation sample commands are available
      When mutation run is given a nonpositive or noninteger count, or seed without count
      Then it fails with a usage error before any mutation trial
      And counted mode requires one supported Git repository and a resolvable HEAD
      # Comparative platform contract, preserved verbatim from MUT173 and
      # executed natively on Windows by MUT248 rather than a platform override:
      # And on an unsupported platform counted mode fails clearly before launching commands
      But existing complete and cached-sample behavior remains unchanged

    @mutation-counted-run @ID-MUT-174
    Scenario: Counted execution evaluates frozen committed inputs
      Given a committed project has a source function, tests and measurement settings
      And those files have different staged, unstaged or untracked working-tree versions
      When a count-one run judges a committed selection
      Then discovery, coverage, clean baselines and mutation tests use the resolved committed inputs
      And its report names that commit and logical source paths
      And the live working tree is not rewritten or mistaken for committed proof
      And unsupported scopes fail rather than silently following live project inputs

    # slice-26, issue #41: a counted run judges HEAD's committed inputs and
    # names that commit (MUT-174), but a change not yet committed to one of
    # those inputs goes unjudged without a word: the issue's agent reran
    # with new tests, saw the same survivors and hunted a bug that was not
    # there. A counted run now warns on stderr, naming the commit it judged
    # and each in-scope path whose working tree differs from it (staged,
    # unstaged, or untracked and not ignored), and its JSON's sampling
    # object lists those paths as "uncommitted", an empty list when there
    # are none. In scope are the inputs it freezes: its project's sources,
    # tests and measurement settings; a change elsewhere in the repository
    # is not named. The warning changes neither the judgment nor the exit
    # code. Before slice-26 nothing is warned, failing at the Then.
    @slice-26 @ID-MUT-275
    Scenario: A counted run warns that uncommitted changes in its scope go unjudged
      Given a committed project whose test has an unstaged change and whose source has a staged change, beside an untracked test file and an unstaged change to a README that is neither source, test nor setting
      When I run "itos-cc mutation run --count 1 --json"
      Then stderr warns that the run judged HEAD's commit and that the test, the source and the untracked test file go unjudged, naming each
      And the JSON's sampling object lists those three paths as "uncommitted", and not the README
      And the judgment and the exit code are those of the same run with a clean working tree
      But with no uncommitted change in scope, nothing is warned and "uncommitted" is an empty list

    @mutation-counted-run @ID-MUT-175
    Scenario: Fresh selection is reproducible across paths and same-name units
      Given committed functions include same-name units and more eligible sites than the budget
      When two count-one runs use the same TEXT seed and committed inputs
      Then they select the same fully identified site regardless of discovery ordering
      And a default seed is the resolved HEAD commit ID
      And reports include the seed and deterministic selection algorithm version
      And alternate-seed selection can be checked without executing mutation trials

    @mutation-counted-run @ID-MUT-176
    Scenario: The global fresh budget holds even when every selected mutant is killed
      Given several committed files and functions have eligible sites but no mutation cache
      And the worker count exceeds the count-one budget
      When a counted run successfully judges its selected covered mutant
      Then exactly one distinct mutation trial executes across the entire selection
      And omitted sites and subjects are reported rather than executed
      And no full mutation prepass prepares the selection
      And the same fresh discovery admits changed functions with missing or stale cache entries
      And a matching prior cached kill never substitutes for the fresh trial

    @mutation-counted-run @ID-MUT-177
    Scenario: Counted mode leaves complete and cached-sample contracts intact
      Given complete-run and cached-sample fixtures have their existing results
      When commands run without the count opt-in
      Then complete scheduling, scope, cache publication and source annotation behavior are unchanged
      And cached mutation sample still rechecks only its eligible recorded outcomes
      And its existing nothing-to-sample behavior is not the counted-mode no-cache behavior

    @mutation-counted-listed @ID-MUT-178
    Scenario: Applicable listed integration tests finish a single fresh judgment
      Given a selected committed mutant survives its file's own tests
      And listed integration tests reach its line and kill it
      When count-one execution judges it
      Then own tests run before the applicable listed tests
      And the listed selection baseline runs without the mutant before its mutation stage
      And the final outcome is killed with the correct listed attribution
      And the own and listed mutation stages consume one trial, not two
      And no unselected site executes

    @mutation-counted-listed @ID-MUT-179
    Scenario: Fresh outcomes preserve survivor, exception and own-timeout meanings
      Given a counted selection has one covered mutation site
      When its complete applicable test sequence passes on the mutant
      Then the run fails for that selected survivor unless its matching exception is valid
      And an exception must not exclude the site from fresh selection or skip its trial
      But a killed mutant or its own test deadline is a valid completed judgment
      And stale exceptions fail without being trusted as successful outcomes
      And counted mode does not silently become fail-fast scheduling

    @mutation-counted-run @ID-MUT-180
    Scenario: Measured uncovered selection is explicit and is never redrawn
      Given a seeded selected site is reached by neither own nor listed tests
      When count-one preparation measures its coverage successfully
      Then that site is reported uncovered with zero mutation trials executed
      And selection remains unchanged rather than drawing another site
      And the run is not described as a range with no eligible sites
      And fail-uncovered fails that judgment
      And strict Go still checks executable zero-site functions and exempts empty bodies

    # Binary-only for good: its per-test coverage metadata is what the real
    # collector writes, and its preparation stages run the real commands.
    @mutation-counted-listed @ID-MUT-181 @binary-only
    Scenario: Counted preparation never converts failed measurement into successful assurance
      Given fresh counted preparation requires list, coverage or provider commands
      When a required command fails, or its report is missing or malformed
      Then the command fails and reports the actual failed stage
      And a report produced by a failed command does not erase its failure
      And per-test conversion and coverage failures are not silently ignored
      And no mutation trial starts after failed preparation
      And no preexisting local measurement or per-scenario coverage map is required for successful preparation
      And raw or existing coverage overrides cannot silently bypass required fresh evidence or listed tests

    @mutation-counted-listed @ID-MUT-182
    Scenario: Clean baseline failure is not a mutation kill
      Given one selected fresh mutant needs own or applicable listed baselines
      When an unmutated required baseline fails
      Then counted execution fails with its actual baseline problem
      And that site has no invented killed, timeout or survived outcome
      And later stages do not report a baseline as passed when it never ran
      And already completed valid judgments remain visible in the current report

    # Physical SIGINT and owned process lifetime; logical states stay eligible.
    @binary-only @mutation-counted-interrupt @ID-MUT-183
    Scenario: Interruption reports partial work and cleans up owned commands
      Given a count-two selection has one completed judgment and one active judgment
      When the counted run is interrupted while the second judgment is active
      Then the completed result remains visible
      And the unfinished judgment is cancelled or undecided, not a killed or timed-out mutant
      And no further command is admitted after the run abort
      # Adapter ordering contract for MUT183, preserved verbatim:
      # And all owned worker and preparation descendants and waiters finish before private input removal
      And after the interrupted run returns, its owned fixture peers have terminated and private inputs are removed
      And one shared five-second post-abort cleanup deadline applies without killing unrelated processes

    # Decision 38 retires count.interrupted (75): a counted run stopped by
    # SIGINT or SIGTERM ends as every interrupted command does.
    @binary-only @slice-7 @slice-8 @ID-MUT-249
    Scenario Outline: An interrupted counted run reports interrupted, not count.interrupted
      Given a count-two selection has one completed judgment and one active judgment
      When the counted run is <stopped> while the second judgment is active
      Then the completed result remains visible
      And the unfinished judgment is cancelled or undecided, not a killed or timed-out mutant
      And its problems in --json are:
        | rule        | exit |
        | interrupted | 130  |
      And it ends as <ended>, which a shell reads as <status>
      And after the interrupted run returns, its owned fixture peers have terminated and private inputs are removed

      Examples:
        | stopped      | ended       | status |
        | interrupted  | interrupted | 130    |
        | sent SIGTERM | terminated  | 143    |

    # Binary-only for good: the product runs as a process of its own, which
    # the scenario stops while selected work runs.
    @mutation-counted-run @ID-MUT-184 @binary-only
    Scenario: Machine and text output distinguish sampled work from population completeness
      Given a counted selection omits some sites and may stop before all selected work completes
      When its report is emitted
      Then JSON is one object with budget, eligible, selected, executed and omitted counts
      And it lists selected identities and actual outcomes with commit, seed and algorithm provenance
      And judged and omitted subjects and real preparation or baseline stages are explicit
      And cancelled, blocked and unattempted selected work has no fabricated mutation outcome
      And a sampled pass is described as assurance about sampled judgments only
      And count does not claim to bound discovery, baseline commands or total latency

    @mutation-counted-run @ID-MUT-185
    Scenario: Sampled execution publishes no persistent complete proof
      Given a multi-site committed function has no complete mutation results
      When a count-one run completes successfully
      Then no mutation snapshot, source annotation or persistent coverage cache is published
      And complete mutation check still reports its missing or stale complete proof without running tests
      And preexisting snapshot and annotated source bytes remain unchanged
      And any genuinely valid complete cache remains a separate unchanged source of proof

    @mutation-counted-run @ID-MUT-186
    Scenario: Only a genuinely site-free range is not applicable
      Given a supported committed selection contains no eligible mutation sites
      When counted execution completes its applicable admission and strict obligations
      Then the report explicitly says not applicable with zero eligible and selected sites
      And it does not run an avoidable mutation prepass
      But absent cache, missing tests, unsupported input scope and failed coverage or baseline are not site-free success

  # GitHub #26: opt-in fail-fast for the local fix-and-retry loop. ADR-0020
  # (stop at the earliest observed final failure, cancel at once, one shared
  # five-second cleanup deadline), ADR-0021 (owned process trees) and ADR-0022
  # (preserve valid partial evidence, report incomplete work) hold, with q-36:
  # Linux and macOS first, Windows refused (fail-fast.platform, exit 82) until
  # #29. Decisions for both slices:
  # - mutation run --fail-fast is opt-in; without it scheduling, output,
  #   snapshots and annotations are exactly as before. It applies to complete
  #   runs (paths, --changed, --since, --mutate-all, --all-tests,
  #   --test-command); it conflicts with --count (flags.conflict), since
  #   counted mode stays aggregate. Interrupt signals in complete mode stay as
  #   they are.
  # - A trigger is an actionable FINAL judgment: an unexcepted survivor once
  #   its applicable listed tests have also failed to kill it, an uncovered
  #   mutant with --fail-uncovered, a mutation.exception-stale, a failing file
  #   baseline (mutation.baseline-failed) or listed selection baseline
  #   (tests.selection-failed). Killed, timed-out, validly excepted and
  #   listed-killed mutants never trigger. Failures known before any mutant
  #   runs (stale exceptions, strict Go coverage findings, uncovered mutants
  #   with --fail-uncovered) stop the run before mutant execution where the
  #   planning already knows them; list and coverage preparation keep their
  #   current scope, with no total-runtime promise.
  # - The first observed trigger, in completion order rather than source
  #   order, publishes one stop: no further command is admitted, in-flight
  #   judgments are cancelled at once with no mutation outcome (never killed
  #   or timed out), and every owned process group is killed and joined within
  #   one shared five-second deadline from the stop, before worker copies are
  #   removed. The run exits as its trigger's rule says (1 for every trigger
  #   above), reporting that rule as it would without --fail-fast.
  # - Output: JSON stays one object and gains a stop object (stopped, the
  #   trigger's rule and subject) and disjoint counts of completed, cancelled,
  #   unattempted and blocked work; every selected file is listed with its
  #   state, and a baseline that never ran is not reported as passed. Plain
  #   output says the run stopped early and why. Completed counts include
  #   trusted reuse and measured coverage judgments, not only test runs.
  # - fail-fast-run publishes a file only when every selected site of its
  #   judged functions was decided; a file the stop cut short gets no snapshot
  #   write and no annotation, as an undecided file today. fail-fast-partial
  #   then preserves that file's valid judgments per ADR-0022.
  # - Local mutant trials stay sparse: tiny fixtures, one or two trials per
  #   focused case, no runs over this repository; broader matrices in CI.
  Rule: mutation run --fail-fast stops at the first actionable failure

    @fail-fast-run @ID-MUT-192
    Scenario: The first actionable survivor stops admission of later mutants
      Given a run with several selected mutants and one worker
      And the first judged mutant survives its own tests and has no applicable listed tests or valid exception
      When mutation run --fail-fast judges them
      Then it fails with mutation.survived for that mutant
      And no later mutant is started, and each is reported unattempted with no outcome
      But without --fail-fast every mutant is judged and the survivor is reported among them

    @binary-only @fail-fast-run @ID-MUT-193
    Scenario: In-flight judgments are cancelled and their owned processes cleaned up
      Given two workers, one judging a mutant whose test command keeps owned descendants running
      When the other worker's mutant becomes an actionable survivor
      Then the running judgment is cancelled with no killed, timed-out or survived outcome
      And its owned processes are terminated and joined within one shared five-second deadline from the stop
      # Adapter ordering contract for MUT193, preserved verbatim:
      # And worker copies are removed only after that join
      And after the stopped run returns, its owned fixture peers have terminated and worker copies are removed
      And unrelated processes remain untouched

    @fail-fast-run @ID-MUT-194
    Scenario: Non-actionable judgments never stop the run
      Given selected mutants that are killed, time out, are validly excepted survivors, or survive their own tests but are killed by applicable listed tests
      When mutation run --fail-fast judges them
      Then every selected mutant is judged and the run succeeds
      And a survivor waits for its applicable listed tests before it can stop the run

    @fail-fast-run @ID-MUT-195
    Scenario: Known policy failures stop before avoidable mutant work
      Given a selection with a stale exception, or with an uncovered mutant or a strict Go coverage finding under --fail-uncovered
      When mutation run --fail-fast runs it
      Then it fails with the same rule the aggregate run reports
      And no mutant trial starts when planning already knew the failure
      But an uncovered mutant without --fail-uncovered does not stop the run

    @fail-fast-run @ID-MUT-196
    Scenario: A failing baseline stops the run without inventing outcomes
      Given a selected file whose own baseline fails, or a survivor whose listed selection baseline fails without any mutant
      When mutation run --fail-fast reaches it
      Then it fails with mutation.baseline-failed or tests.selection-failed
      And the affected mutants have no invented killed, timed-out or survived outcome
      And no stage reports a baseline as passed that never ran

    # Binary-only for good: the stop cancels a mutant still running in a
    # process the run owns.
    @fail-fast-run @ID-MUT-197 @binary-only
    Scenario: Output distinguishes completed work from cancelled and unattempted work
      Given a fail-fast run that stops with completed, cancelled, unattempted and blocked work
      When its report is emitted as JSON and as text
      Then JSON is one object with a stop naming the trigger's rule and subject
      And it gives disjoint completed, cancelled, unattempted and blocked counts and every selected file's state
      And plain output says the run stopped early and why
      But a run without --fail-fast keeps its existing output exactly

    @fail-fast-run @ID-MUT-198
    Scenario: A file the stop cut short publishes no partial proof
      Given a fail-fast run that completes one file and stops inside another
      When it returns
      Then the completed file's snapshot and annotation are written as without --fail-fast
      And the stopped file's annotated source bytes are unchanged, and its snapshot records no outcome for a site the stop left undecided
      And mutation check still reports the stopped file's missing or stale results without running tests
      And a later run reuses only outcomes whose inputs are still fresh

    @fail-fast-run @ID-MUT-199
    Scenario: Fail-fast admission boundaries
      Given the complete and counted mutation run commands
      When --fail-fast is combined with --count
      Then --count with --fail-fast is a flags.conflict usage error
    # Comparative contract (native public proof is ID-MUT-248):
    # And on Windows it fails with fail-fast.platform before launching commands
      And the help describes --fail-fast, its rule and its platforms
    # Documentation contract retained in TestTheDocsDescribeCountedAndFailFastPlatforms:
    # And the help, README and docs/CLI.md describe --fail-fast, its rule and its platforms

    @native-platform-admission @ID-MUT-248
    Scenario Outline: Unsupported Windows execution is refused before project commands run
      Given a committed eligible mutation fixture on Windows
      When I run "itos-cc mutation run <mode> --json" in that fixture
      Then the problem is "<rule>" for Windows and the exit code is 82
      And no test, list or coverage command runs and no snapshot is written

      Examples:
        | mode        | rule               |
        | --count 1   | count.platform     |
        | --fail-fast | fail-fast.platform |

  # fail-fast-counts: a fail-fast file's "ran" and "reused" count this run's
  # trials and reuses of that file in every state, blocked included, and
  # agree with its work counts; results only kept from before are counted
  # in neither, as fail-fast-partial decided. Aggregate output is unchanged.
  Rule: A fail-fast file's counts agree with its work

    @fail-fast-counts @ID-MUT-211
    Scenario: A file blocked by a failing listed selection counts what it ran and reused
      Given a fail-fast run that ran one mutant of a file and reused another before a failing listed selection blocked a third
      When its report is emitted as JSON and as text
      Then that file reports "ran" 1 and "reused" 1
      And those agree with its completed work count
      But a run without --fail-fast reports the same file as before

  # fail-fast-partial: ADR-0022 for the files fail-fast-run leaves unwritten.
  Rule: A fail-fast stop keeps the valid judgments of the files it cut short

    # Binary-only for good: the stop cancels mutants still running in
    # processes the run owns, as for the next two.
    @fail-fast-partial @ID-MUT-200 @binary-only
    Scenario: Valid judgments in a stopped file are preserved, unfinished sites stay unjudged
      Given a fail-fast run judges some mutants of a file and the stop cancels or leaves the rest
      When it returns
      Then the file's snapshot keeps each completed or reused judgment with its original scope and freshness evidence
      And cancelled, blocked and unattempted sites get no outcome and no new freshness proof
      And mutation check still fails the functions with missing valid site entries
      And the file's annotated source bytes are unchanged

    @fail-fast-partial @ID-MUT-201 @binary-only
    Scenario: A cancelled forced rerun leaves a fresh prior cache usable
      Given a file with fresh complete mutation results
      When a fail-fast --mutate-all rerun of it is cancelled by a stop elsewhere
      Then its prior fresh results still satisfy mutation check
      And the run's report says its own work was incomplete, separately from the cache's completeness

    @fail-fast-partial @ID-MUT-202 @binary-only
    Scenario: A later run finishes only what the stop left
      Given a stopped fail-fast run preserved some judgments of a file
      When the same run is repeated with unchanged inputs
      Then the preserved judgments are reused and only the unjudged sites run
      But a preserved judgment whose source, tests or support inputs changed runs again

    # q-37: a cleanup that fails is its own problem, mutation.cleanup-failed,
    # with the internal exit, which outranks the check's no that stopped the
    # run; the partial report is kept, not lost to the failure.
    @binary-only @slice-7 @slice-8 @ID-MUT-250
    Scenario: A fail-fast stop whose cleanup fails exits 100 and keeps its partial report
      Given two workers, one judging a mutant whose owned cleanup cannot finish within the shared five-second deadline
      When the other worker's mutant becomes an actionable survivor
      Then the judgment whose cleanup failed is cancelled, with no killed, timed-out or survived outcome
      And JSON is one object with a stop naming the trigger's rule and subject
      And its problems in --json are:
        | rule                    | exit |
        | mutation.survived       | 1    |
        | mutation.cleanup-failed | 100  |
      And the mutation.cleanup-failed problem names the owned process group and its command
      And the exit code is 100

  # slice-6, issue #16 and decision 45: itos-cc.yaml's exclude leaves the
  # sources it names out of mutation run, list, check, sample and run
  # --count. The selection drops them before a --since range is
  # intersected, so a gate's command stays the same. Old records of a
  # source newly excluded are orphans: its snapshot is named under
  # "orphans", each entry of which gains "why", "gone" or "excluded" (an
  # added key), and the next mutation run under its paths removes it; an
  # exception naming it is stale with why excluded, whose fix is to remove
  # the entry, and mutation except refuses its sites as exception.excluded,
  # exit 80. --count reads the itos-cc.yaml HEAD commits.
  Rule: Sources itos-cc.yaml excludes are not mutated

    @slice-6 @ID-MUT-243
    Scenario: A gate judges no excluded source the range changed
      Given itos-cc.yaml excludes "**/porttest/**"
      And the only commit since main changes src/port/porttest/fake.go, whose mutants no test reaches
      When I run "itos-cc mutation run --since main --fail-uncovered"
      Then stderr says "itos-cc: no source files to mutate", and the exit code is 0
      And "itos-cc mutation check --since main --fail-uncovered" exits 0
      And neither "itos-cc mutation list" nor "itos-cc mutation sample --count 5" names a site in fake.go

    @slice-6 @ID-MUT-244
    Scenario: The snapshot of a source newly excluded is an orphan
      Given a snapshot under .metrics/mutate for src/port/porttest/fake.go, and fresh results for every other source
      And itos-cc.yaml then excludes "**/porttest/**"
      When I run "itos-cc mutation check --json"
      Then its "orphans" list src/port/porttest/fake.go with "why" "excluded", and the exit code is 0
      When I run "itos-cc mutation run --json"
      Then the snapshot is removed, and its "orphans" list it with "why" "excluded"

    @slice-6 @ID-MUT-245
    Scenario: An exception naming an excluded source is stale, and none can be added
      Given itos-cc.yaml excepts a survivor in src/port/porttest/fake.go and excludes "**/porttest/**"
      When I run "itos-cc mutation check --json"
      Then it reports "mutation.exception-stale" with "why" "excluded"
      When I run "itos-cc mutation except src/port/porttest/fake.go:7:9 --reason x"
      Then the problem is "exception.excluded", with file "src/port/porttest/fake.go"
      And the exit code is 80

    @slice-6 @ID-MUT-246
    Scenario: A counted run selects no site in an excluded source
      Given the itos-cc.yaml HEAD commits excludes "**/porttest/**"
      When I run "itos-cc mutation run --count 20 --json"
      Then no selected site is in src/port/porttest/fake.go

    # slice-44, counted-config-own-fix (2026-10-24, after bug-8): a counted run reads
    # the itos-cc.yaml HEAD commits, so its config.invalid fix opens with
    # "Fix it in a commit: a counted run reads the itos-cc.yaml HEAD holds".
    # bug-8 gave a refusal of an anchor, an alias or a merge key a fix of its
    # own (write the value in place), and on a counted run that fix took the
    # commit hint's place, so a person who fixed only the working copy met
    # the same refusal again with no word why. The counted run's fix is now
    # the commit hint, then the refusal's own fix. Before this item the fix
    # held no commit hint, failing at the fix step. A run that is not
    # counted reads the working copy and keeps its fix as it is.
    @slice-44 @ID-MUT-333
    Scenario: A counted run's refusal of an anchor keeps the hint to commit the fix
      Given the itos-cc.yaml HEAD commits holds '{x: &e [], exclude: *e}'
      When I run "itos-cc mutation run --count 1"
      Then the problem is "config.invalid", with key "x"
      And its fix says "Fix it in a commit: a counted run reads the itos-cc.yaml HEAD holds", then "Write the value in place"
      And the exit code is 80

  # Decision 38 and q-37: an interrupt stops a complete mutation run as a
  # fail-fast stop does (ADR-0020 to ADR-0022): no command starts after it,
  # the judgment running is cancelled with no outcome, every owned process
  # group is killed and joined within the shared five-second deadline, and
  # the judgments it finished are kept, a file it cut short getting no
  # summary comment. A second interrupt waits for that cleanup, up to the
  # deadline (ADR-0021). A cleanup that fails is mutation.cleanup-failed,
  # exit 100, beside interrupted, and the run still exits 130; its subject
  # gives process_group and command, one problem for each group. An
  # interrupted complete run's --json gives no stop object: the interrupted
  # problem says why it ended. crap's coverage commands own their process
  # groups as mutation's do, and a cleanup of theirs that fails is
  # coverage.cleanup-failed, exit 100, held by unit tests.
  Rule: An interrupt stops a mutation run, keeping what it finished

    # Binary-only for good, as the rest of this Rule: the signal reaches
    # itos-cc as a process of its own while processes it owns run.
    @binary-only @slice-8 @ID-MUT-251
    Scenario: An interrupted complete run keeps the judgments it finished and leaves no owned process
      Given a complete run with one worker has one completed judgment and one active judgment
      When the complete run is interrupted while the second judgment is active
      Then the unfinished judgment is cancelled or undecided, not a killed or timed-out mutant
      And the file's snapshot keeps each completed or reused judgment with its original scope and freshness evidence
      And cancelled, blocked and unattempted sites get no outcome and no new freshness proof
      And the file's annotated source bytes are unchanged
      And its problems in --json are:
        | rule        | exit |
        | interrupted | 130  |
      And it ends as interrupted, which a shell reads as 130
      And after the stopped run returns, its owned fixture peers have terminated and worker copies are removed
      And unrelated processes remain untouched

    @binary-only @slice-7 @slice-8 @ID-MUT-252
    Scenario Outline: An interrupt whose cleanup fails exits 130 and reports both
      Given a <run> is judging a mutant whose owned cleanup cannot finish within the shared five-second deadline
      When the run is interrupted while that judgment is active
      Then the judgment whose cleanup failed is cancelled, with no killed, timed-out or survived outcome
      And its problems in --json are:
        | rule                    | exit |
        | interrupted             | 130  |
        | mutation.cleanup-failed | 100  |
      And the mutation.cleanup-failed problem names the owned process group and its command
      And it ends as interrupted, which a shell reads as 130

      Examples:
        | run          |
        | complete run |
        | counted run  |
      # a counted run said only count.interrupted (75) before, hiding the
      # failed cleanup

    @binary-only @slice-8 @ID-MUT-253
    Scenario: A second interrupt waits while the owned process groups are killed
      Given a complete run is judging a mutant whose owned cleanup cannot finish within the shared five-second deadline
      When the run is interrupted, and interrupted again once its owned processes have been killed
      Then it returns only once the shared five-second cleanup deadline has passed
      And its problems in --json are:
        | rule                    | exit |
        | interrupted             | 130  |
        | mutation.cleanup-failed | 100  |
      And it ends as interrupted, which a shell reads as 130
      # at any other time a second interrupt ends the command at once, which
      # unit tests hold with fake signals
