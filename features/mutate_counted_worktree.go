package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (q *countedMutation) worktreeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a committed Go project and a second checkout of it made with git worktree add$`, q.linkedProject)
	sc.Step(`^a count-one run judges it from the linked worktree$`, q.judge)
	sc.Step(`^exactly one selected mutant is judged with a real outcome and the report names the worktree's HEAD commit$`, q.linkedJudgment)
	sc.Step(`^its private scratch directory lies under the repository's git directory for that worktree and is removed afterwards$`, q.linkedScratch)
	sc.Step(`^neither checkout's working tree is changed$`, q.linkedUnchanged)
	sc.Step(`^a scratch path that escapes both the checkout and its git directory is refused with a problem and a fix, not an internal error$`, q.escapedScratch)
}
func (q *countedMutation) linkedProject(ctx context.Context) error {
	if err := q.project(ownedOneSource); err != nil {
		return err
	}
	w := q.e.m.w
	q.mainRoot = w.dir
	parent, err := w.d.Folder()
	if err != nil {
		return err
	}
	q.worktreeParent = parent
	q.linkedRoot = filepath.Join(parent, "linked")
	if err = w.d.Git(q.mainRoot, "worktree", "add", "-qb", "T81-linked", q.linkedRoot); err != nil {
		return err
	}
	w.dir = q.linkedRoot
	if err = w.write("main.go", ownedOneSource+"\n// linked fixture commit\n"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "add", "main.go"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "linked fixture HEAD"); err != nil {
		return err
	}
	q.head, err = w.d.Head(ctx, w.dir)
	if err != nil {
		return err
	}
	mainHead, err := w.d.Head(ctx, q.mainRoot)
	if err != nil {
		return err
	}
	if q.head == mainHead {
		return fmt.Errorf("linked fixture lacks independently committed HEAD")
	}
	pointer, err := w.d.Read(filepath.Join(w.dir, ".git"))
	if err != nil {
		return err
	}
	q.texts["git-pointer"] = Result{Stdout: string(pointer)}
	if !strings.HasPrefix(string(pointer), "gitdir: ") {
		return fmt.Errorf("linked git pointer: %s", pointer)
	}
	w.variables = append(w.variables, "T81_ORIGINAL="+w.dir)
	if err = q.setMode("covered"); err != nil {
		return err
	}
	if err = q.originalHashes("main.go"); err != nil {
		return err
	}
	q.before, err = q.tree(ctx, q.mainRoot)
	if err != nil {
		return err
	}
	linked, err := q.tree(ctx, w.dir)
	if err != nil {
		return err
	}
	q.e.before = linked
	return nil
}
func (q *countedMutation) linkedJudgment() error {
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	p := countedSampling(q.primary)
	if q.primary.result.Exit != 0 || p["commit"] != q.head || p["selected"] != float64(1) || p["executed"] != float64(1) || s["file"] != "main.go" || s["state"] != "judged" || s["outcome"] != "killed" {
		return fmt.Errorf("linked committed judgment: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) linkedScratch() error {
	original := countedSHA(ownedOneSource + "\n// linked fixture commit\n")
	mutated := countedSHA(strings.Replace(ownedOneSource, ">", ">=", 1) + "\n// linked fixture commit\n")
	baseline, trial := 0, 0
	private := ""
	for _, r := range q.primary.traces {
		if r["git_pointer"] != q.texts["git-pointer"].Stdout {
			return fmt.Errorf("execution did not retain linked Git pointer: %v", r)
		}
		gitdir, ok := r["git_directory"].(string)
		if !ok || !strings.Contains(filepath.ToSlash(gitdir), "/.git/worktrees/") {
			return fmt.Errorf("canonical linked git directory missing: %v", r)
		}
		cwd, ok := r["cwd"].(string)
		if !ok {
			return fmt.Errorf("linked execution cwd missing")
		}
		if r["source_sha256"] == original {
			rel, err := filepath.Rel(filepath.Join(gitdir, "itos"), cwd)
			if err != nil || strings.HasPrefix(rel, "..") || !strings.HasPrefix(filepath.ToSlash(rel), "fresh-plan-") {
				return fmt.Errorf("linked private input escaped git directory: %v", r)
			}
			root := strings.Split(filepath.ToSlash(rel), "/")[0]
			private = filepath.Join(gitdir, "itos", root)
			if r["coverage"] != true {
				baseline++
			}
		} else if r["source_sha256"] == mutated {
			trial++
		} else {
			return fmt.Errorf("linked execution source differs from HEAD/selected mutant: %v", r)
		}
	}
	if baseline != 1 || trial != 1 || private == "" || q.e.m.w.d.Exists(private) {
		return fmt.Errorf("linked private execution/removal population baseline%d trial%d private%s", baseline, trial, private)
	}
	q.texts["gitdir"] = Result{Stdout: filepath.Dir(filepath.Dir(private))}
	return nil
}
func (q *countedMutation) linkedUnchanged(ctx context.Context) error {
	if err := q.sameTree(ctx, q.mainRoot, q.before); err != nil {
		return err
	}
	return q.sameTree(ctx, q.linkedRoot, q.e.before)
}
func (q *countedMutation) escapedScratch(ctx context.Context) error {
	w := q.e.m.w
	target, err := w.d.Folder()
	if err != nil {
		return err
	}
	q.e.extraDirs = append(q.e.extraDirs, target)
	if err = w.d.Write(filepath.Join(target, "preserved.txt"), []byte("outside fixture marker\n")); err != nil {
		return err
	}
	before, err := q.tree(ctx, target)
	if err != nil {
		return err
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(q.texts["git-pointer"].Stdout, "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(q.linkedRoot, gitdir)
	}
	scratch := filepath.Join(gitdir, "itos")
	if err = w.d.Remove(scratch); err != nil {
		return err
	}
	if err = w.d.Link(target, scratch); err != nil {
		return err
	}
	o, err := q.count("--count", "1", "--workers", "1")
	if err != nil {
		return err
	}
	if err = countedProblem(o, "count.preparation-failed", 1); err != nil {
		return err
	}
	p := checkProblems(o.object)[0]
	if p["stage"] != "scratch" || strings.TrimSpace(fmt.Sprint(p["fix"])) == "" || strings.Contains(fmt.Sprint(p["rule"]), "internal") || len(o.traces) != 0 {
		return fmt.Errorf("escaped scratch refusal lacks stage/fix or ran commands: %v", o.object)
	}
	if err = strictNoCommands(o.result.Stderr); err != nil {
		return err
	}
	if err = q.sameTree(ctx, target, before); err != nil {
		return err
	}
	return q.linkedUnchanged(ctx)
}
