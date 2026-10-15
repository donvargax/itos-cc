package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const sampleBoardTS = `export class Board {
place(i: number): boolean {
// past five
// the boundary is deliberate
return i>5;
}
clear(i: number): boolean {
return i<3;
}
}
`

const sampleTraceWriter = `import {appendFileSync,readFileSync,realpathSync} from "node:fs";
import {createHash} from "node:crypto";
const trace = @TRACE@;
if(trace) appendFileSync(trace,JSON.stringify({cwd:realpathSync(process.cwd()),original_root:realpathSync(@ROOT@),source_sha256:createHash("sha256").update(readFileSync("src/board.ts")).digest("hex")})+"\n");
`

const sampleTestTS = sampleTraceWriter + `import {expect,test} from "vitest";
import {Board} from "./board";
const control = JSON.parse(readFileSync(@CONTROL@,"utf8"));
test("place",async()=>{
 if(control.fail) throw new Error("fixture baseline broken");
 const b=new Board();
 if(!control.place_weak) expect(b.place(5)).toBe(false);
 expect(b.place(6)).toBe(true);
});
test("clear",()=>{
 const b=new Board();
 expect(b.clear(2)).toBe(true);
 if(control.clear_strong) expect(b.clear(3)).toBe(false);
});
`

const sampleE2ETS = `import {expect,test} from "vitest";
import {pathToFileURL} from "node:url";
import {join} from "node:path";
import {readFileSync} from "node:fs";
test("end to end boundary",async()=>{
 const control=JSON.parse(readFileSync(@CONTROL@,"utf8"));
 const {Board}=await import(pathToFileURL(join(process.cwd(),"src","board.ts")).href);
 if(!control.e2e_weak) expect(new Board().place(5)).toBe(false);
 expect(new Board().place(6)).toBe(true);
});
`

const sampleUnrelatedTS = `import {test} from "vitest";
import {readFileSync} from "node:fs";
test("unrelated whole suite",()=>{
 const control=JSON.parse(readFileSync(@CONTROL@,"utf8"));
 if(control.all_fail) throw new Error("fixture whole suite broken");
});
`

type sampleMutation struct {
	s                        *sinceMutation
	id                       string
	external, control, trace string
	controls                 map[string]bool
	files                    []string
	original                 string
	observe                  func() ([]string, error)
	firstTrials              []string
}

func (p *sampleMutation) project() error {
	s, w := p.s, p.s.m.w
	if s.made {
		return nil
	}
	var err error
	p.external, err = w.d.Folder()
	if err != nil {
		return err
	}
	p.control = filepath.Join(p.external, "control.json")
	p.trace = filepath.Join(p.external, "executions.jsonl")
	p.controls = map[string]bool{}
	if slices.Contains([]string{"73", "111"}, p.id) {
		p.controls["place_weak"] = true
	}
	if slices.Contains([]string{"109", "110", "112", "113"}, p.id) {
		p.controls["clear_strong"] = true
	}
	if err := p.writeControl(); err != nil {
		return err
	}
	s.tsSource = sampleBoardTS
	s.tsTests = strings.NewReplacer("@CONTROL@", strconv.Quote(filepath.ToSlash(p.control)), "@TRACE@", strconv.Quote(p.tracePath()), "@ROOT@", strconv.Quote(filepath.ToSlash(w.dir))).Replace(sampleTestTS)
	s.tsSurvivors = []string{}
	if p.id == "74" {
		source := `import {setTimeout as delay} from "node:timers/promises";
export class Board {
async place(still: boolean): Promise<boolean> {
if (!still) await delay(3600000);
return still;
}
clear(i: number): boolean { return i<3; }
}
`
		s.tsSource = source
		test := `import {expect,test} from "vitest";
import {Board} from "./board";
test("place",async()=>expect(await new Board().place(true)).toBe(true),120000);
test("clear",()=>{expect(new Board().clear(2)).toBe(true);expect(new Board().clear(3)).toBe(false);});
`
		s.tsTests = test
		p.controls["clear_strong"] = true
		s.tsReport = "SF:src/board.ts\nDA:4,1\nDA:5,1\nDA:7,1\nend_of_record\n"
	}
	if p.id == "75" {
		s.tsReport = "SF:src/board.ts\nDA:5,1\nDA:6,0\nDA:8,1\nDA:11,0\nend_of_record\n"
		s.tsSource = strings.Replace(sampleBoardTS, "return i>5;", "if (i>9) {\nreturn false;\n}\nreturn i>5;", 1)
	}
	if err := s.project(true); err != nil {
		return err
	}
	p.files = []string{s.file}
	p.original = s.m.read(s.file)
	if err := w.write("package.json", `{"name":"board","type":"module","scripts":{"test":"vitest run --bail=1"},"devDependencies":{"vitest":"5.0.2","@vitest/coverage-v8":"5.0.2"}}`+"\n"); err != nil {
		return err
	}
	makefile := "test:\n\tnode_modules/.bin/vitest run --bail=1\n"
	if p.id == "112" {
		logger := strings.NewReplacer("@TRACE@", strconv.Quote(filepath.ToSlash(p.trace)), "@ROOT@", strconv.Quote(filepath.ToSlash(w.dir))).Replace(sampleTraceWriter)
		if err := w.write("record-make.mjs", logger); err != nil {
			return err
		}
		makefile = "test:\n\tnode record-make.mjs\n\tnode_modules/.bin/vitest run --bail=1\n"
	}
	if err := w.write("Makefile", makefile); err != nil {
		return err
	}
	if p.id == "111" {
		text := strings.ReplaceAll(sampleE2ETS, "@CONTROL@", strconv.Quote(filepath.ToSlash(p.control)))
		if err := w.write("e2e/board.test.ts", text); err != nil {
			return err
		}
	}
	if p.id == "113" {
		text := strings.ReplaceAll(sampleUnrelatedTS, "@CONTROL@", strconv.Quote(filepath.ToSlash(p.control)))
		if err := w.write("e2e/unrelated.test.ts", text); err != nil {
			return err
		}
	}
	p.sampleOptions()
	if err := p.declare(); err != nil {
		return err
	}
	if err := s.commit("record sample fixture"); err != nil {
		return err
	}
	return w.git("tag", "-f", "base")
}

func (p *sampleMutation) tracePath() string {
	if p.id == "71" {
		return filepath.ToSlash(p.trace)
	}
	return ""
}

func (p *sampleMutation) sampleOptions() {
	w := p.s.m.w
	previous := w.prepare
	w.prepare = func(args []string) []string {
		if previous != nil {
			args = previous(args)
		}
		if slices.Contains(args, "sample") && !slices.ContainsFunc(args, func(a string) bool { return a == "--workers" || strings.HasPrefix(a, "--workers=") }) {
			args = append(args, "--workers", "2")
		}
		return args
	}
}

func (p *sampleMutation) writeControl() error {
	b, err := json.Marshal(p.controls)
	if err != nil {
		return err
	}
	return p.s.m.w.d.Write(p.control, b)
}

func (p *sampleMutation) declare() error {
	s := p.s
	own := Tests{Language: "typescript", Command: vitestCommand + s.file}
	if p.controls["fail"] {
		own.Baseline = "fixture baseline broken"
	}
	sites, err := s.m.sitesOf(s.file)
	if err != nil {
		return err
	}
	for _, site := range sites {
		mu := Site{File: s.file, Line: site.line, Original: site.original, Replacement: site.replacement}
		if p.id == "74" && site.original == "!" {
			own.TimingOut = append(own.TimingOut, mu)
			continue
		}
		if p.id == "75" && site.line == 5 {
			own.Surviving = append(own.Surviving, mu)
		}
		if sinceFunction(site, s.place) && p.controls["place_weak"] || sinceFunction(site, s.clear) && !p.controls["clear_strong"] {
			own.Surviving = append(own.Surviving, mu)
		}
	}
	if p.tracePath() != "" {
		own.Trace = &TestTrace{File: p.trace, Source: s.file}
	}
	all := own
	all.Command = "node_modules/.bin/vitest run --bail=1"
	if p.id == "111" {
		all.Surviving = nil
		for _, mu := range own.Surviving {
			if !p.controls["e2e_weak"] && mu.Original == ">" {
				continue
			}
			all.Surviving = append(all.Surviving, mu)
		}
	}
	if p.controls["all_fail"] {
		all.Baseline = "fixture whole suite broken"
	}
	makeTests := all
	makeTests.Command = "make test"
	if p.id == "112" {
		makeTests.Trace = &TestTrace{File: p.trace, Source: s.file}
	}
	absOwn, absAll := own, all
	absOwn.Command = filepath.ToSlash(filepath.Join(s.m.w.dir, own.Command))
	absAll.Command = filepath.ToSlash(filepath.Join(s.m.w.dir, all.Command))
	for _, tests := range []Tests{own, all, absOwn, absAll, makeTests} {
		if err := s.m.w.d.Tests(s.m.w.dir, tests); err != nil {
			return err
		}
	}
	return nil
}

func (p *sampleMutation) record(flags string) error {
	if err := p.project(); err != nil {
		return err
	}
	w := p.s.m.w
	if err := w.runWith(w.env(), "mutation run --json "+flags+" src/board.ts"); err != nil {
		return err
	}
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	p.s.before = snap
	p.s.sourceBefore = p.s.m.read(p.s.file)
	p.s.m.held = map[string]string{}
	for _, name := range []string{p.s.file, "src/board.test.ts", ".metrics/mutate/" + p.s.file + ".json"} {
		p.s.m.held[name] = p.s.m.read(name)
	}
	return nil
}

func (p *sampleMutation) setOutcome(function, original, outcome string) error {
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	units := p.s.units(snap, function)
	if len(units) != 1 {
		return fmt.Errorf("no unique snapshot entry for %s", function)
	}
	u := units[0]
	found := false
	u["killed"], u["survived"], u["uncovered"] = float64(0), float64(0), float64(0)
	for _, raw := range u["mutants"].([]any) {
		m := raw.(map[string]any)
		if original == "" || m["original"] == original {
			m["outcome"] = outcome
			found = true
		}
		key := fmt.Sprint(m["outcome"])
		if key == "timeout" {
			key = "killed"
		}
		u[key] = u[key].(float64) + 1
	}
	if !found {
		return fmt.Errorf("no recorded %s mutant of %s", original, function)
	}
	return (&checkMutation{s: p.s}).writeSnapshot(snap)
}

func (p *sampleMutation) fifty(git bool) error {
	s, w := p.s, p.s.m.w
	if s.made {
		return nil
	}
	s.made = true
	s.file = "src/part1.go"
	s.m.file = s.file
	if err := w.write("go.mod", mutateModule); err != nil {
		return err
	}
	if err := w.write("README.md", "# Fifty mutants\n"); err != nil {
		return err
	}
	tests := Tests{Language: "go", Command: "go version"}
	for f := 1; f <= 5; f++ {
		file := fmt.Sprintf("src/part%d.go", f)
		p.files = append(p.files, file)
		text := "package board\n"
		for n := 1; n <= 10; n++ {
			text += fmt.Sprintf("func f%d(a int) bool { return a > %d }\n", (f-1)*10+n, n+1)
		}
		if err := w.write(file, text); err != nil {
			return err
		}
		sites, err := s.m.sitesOf(file)
		if err != nil {
			return err
		}
		for _, site := range sites {
			tests.Surviving = append(tests.Surviving, Site{File: file, Line: site.line, Original: site.original, Replacement: site.replacement})
		}
	}
	if err := w.d.Tests(w.dir, tests); err != nil {
		return err
	}
	if git {
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
			if err := w.git(args...); err != nil {
				return err
			}
		}
	}
	p.sampleOptions()
	if err := w.runWith(w.env(), "mutation run --json --test-command 'go version' --workers 2 --no-annotate --no-coverage"); err != nil {
		return err
	}
	obj, err := w.json()
	if err != nil {
		return err
	}
	total := float64(0)
	for _, raw := range obj["files"].([]any) {
		total += raw.(map[string]any)["survived"].(float64)
	}
	if total != 50 {
		return fmt.Errorf("recorded %v survivors, want fifty\n%s", total, w.report())
	}
	return nil
}
