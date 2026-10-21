// Package dry is the dry command, a slice named for it: its kong struct and
// Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to Handle with the context cmd made. Handle
// only wires: the selection domain chooses the files through the Tree port
// it was given and parses each one, read through the same port and parsed
// by lang, asking the Surroundings port it was given what surrounds it, and
// the duplication domain finds the pairs and groups; the snapshot domain
// names each file from the project root, and its Store port records what
// was found. Run writes it, each file named from the working directory. The
// slice reads no file itself. A failure is the selection domain's or the
// snapshot domain's, which cmd's classifiers turn into problems, or lang's,
// an internal one; Run reports it through the UI. It has no classifier of
// its own.
package dry

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	duplication "github.com/donvargax/itos-cc/internal/slices/dry/domain"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// CLI is the dry command's command line: the files it selects, as every
// command selects them (cli.Selection, first), the similarity a pair must
// reach, the smallest functions it compares, and whether it prints lines or
// the one object a script reads. Its defaults are duplication.Defaults, a
// test holds.
type CLI struct {
	cli.Selection
	Threshold float64 `placeholder:"N" default:"0.82" help:"minimum similarity, 0–1"`
	MinLines  int     `name:"min-lines" placeholder:"N" default:"4" help:"skip functions shorter than N lines"`
	MinNodes  int     `name:"min-nodes" placeholder:"N" default:"20" help:"skip functions with fewer than N normalized syntax nodes"`
	JSON      bool    `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what dry's help says beyond its flags: what it finds, the
// --json object a script reads, and every exit code it can return. A
// scenario checks the help and a script reads it.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Finds functions in the same language whose normalized structure is similar
enough to review as duplicates, grouping those linked by similar pairs.
Local names, field names, and literals do not count; called functions,
operators, and the shape of the code do.

With paths or --changed, those files are compared against every source under
the working directory, so a change is checked against the whole project.
The sources itos-cc.yaml's exclude names are left out of both, however they
are chosen. Writes .metrics/dry.json.`,
		JSON: `"threshold", "candidates": [{"score", "language", "left", "right"}],
   "groups": [{"language", "min_score", "max_score", "members": [{"file",
   "start_line", "end_line", "namespace", "name", "nodes"}]}]`,
		Rules: []string{
			"config.invalid  itos-cc.yaml cannot be read: file, and key for an exclude glob",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success, duplicates or not"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag or path, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "--changed outside a git repository"},
		},
		Examples: []string{
			"itos-cc dry",
			"itos-cc dry --changed --json",
		},
	}
}

// Query is what the dry command asks: the duplicates among the sources
// paths and changed select, with paths or changed checked against every
// source under the working directory, by opt.
type Query struct {
	Paths   []string
	Changed bool
	Options duplication.Options
}

// Handler is the dry command's application: the working tree it selects
// and reads files through, what surrounds those files, which lang asks
// about to parse them, the store its snapshot is recorded in, Name, the
// name a person reads a file by, from the working directory, and Exclude,
// the sources the project's settings leave out. cmd gives each, choosing
// the adapters behind them and reading the settings. No value of a command
// line is read here.
type Handler struct {
	Tree         port.Tree
	Surroundings langport.Surroundings
	Store        snapport.Store
	Name         func(path string) string
	Exclude      selection.Exclusion
}

// Found is what Handle found: the pairs at or above the threshold, best
// first, and the groups they link. NoSources is set when paths or changed
// selected no source to check, so nothing was compared or recorded.
// Excluded is the count of the sources the paths and changed chose that
// Exclude left out.
type Found struct {
	Candidates []duplication.Candidate
	Groups     []duplication.Group
	NoSources  bool
	Excluded   int
}

// Handle finds the duplicates q asks for and records them in dry.json,
// passing ctx on to the domains, lang and the ports, the sources the
// handler's Exclude names left out of the files checked and of the whole
// project they are checked against alike. Each side names its file by
// Name, and dry.json by its path from the project root (snapshot.Key). A
// failure to select or read stops it with nothing found; a failure to
// record what was found returns it beside the failure.
func (h Handler) Handle(ctx context.Context, q Query) (Found, error) {
	chosen, err := selection.Select(ctx, h.Tree, q.Paths, q.Changed, h.Exclude)
	if err != nil {
		return Found{}, err
	}
	all := chosen.Sources
	focus := map[string]bool{}
	if len(q.Paths) > 0 || q.Changed {
		for _, f := range chosen.Sources {
			focus[f] = true
		}
		if len(focus) == 0 {
			return Found{NoSources: true, Excluded: chosen.Excluded}, nil
		}
		whole, err := selection.Select(ctx, h.Tree, nil, false, h.Exclude)
		if err != nil {
			return Found{}, err
		}
		all = union(whole.Sources, chosen.Sources)
	}
	root, failure := h.Store.Root(ctx)
	if failure != nil {
		return Found{}, failure
	}
	var forms []duplication.Form
	keys := map[string]string{}
	for _, path := range all {
		f, err := selection.Parse(ctx, h.Tree, h.Surroundings, path)
		if err != nil {
			return Found{}, err
		}
		name := h.Name(path)
		keys[name] = snapshot.Key(root, path)
		forms = append(forms, duplication.Forms(f, name, q.Options)...)
		f.Close()
	}
	found := Found{Candidates: []duplication.Candidate{}, Excluded: chosen.Excluded}
	if c := duplication.Compare(forms, focus, q.Options); c != nil {
		found.Candidates = c
	}
	found.Groups = duplication.Groups(found.Candidates)
	candidates, groups := fromRoot(found.Candidates, found.Groups, keys)
	if failure := h.Store.Write(ctx, "dry.json", recorded{Version: snapshot.Version, Threshold: q.Options.Threshold,
		Candidates: candidates, Groups: groups}); failure != nil {
		return found, failure
	}
	return found, nil
}

// recorded is dry.json: the format version, the threshold, and the pairs
// and groups with each file named from the project root.
type recorded struct {
	Version    int                     `json:"version"`
	Threshold  float64                 `json:"threshold"`
	Candidates []duplication.Candidate `json:"candidates"`
	Groups     []duplication.Group     `json:"groups"`
}

// union merges sorted path lists without duplicates; paths named outside the
// working directory still take part.
func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// fromRoot is candidates and groups as dry.json records them: each file by
// its key, from the project root, while output names it from the working
// directory.
func fromRoot(candidates []duplication.Candidate, groups []duplication.Group, keys map[string]string) ([]duplication.Candidate, []duplication.Group) {
	side := func(s duplication.Side) duplication.Side {
		s.File = keys[s.File]
		return s
	}
	var cs []duplication.Candidate
	if candidates != nil {
		cs = []duplication.Candidate{}
	}
	for _, c := range candidates {
		c.Left, c.Right = side(c.Left), side(c.Right)
		cs = append(cs, c)
	}
	var gs []duplication.Group
	if groups != nil {
		gs = []duplication.Group{}
	}
	for _, g := range groups {
		members := make([]duplication.Side, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, side(m))
		}
		g.Members = members
		gs = append(gs, g)
	}
	return cs, gs
}

// result is the object --json prints beside schema and ok: the threshold,
// the pairs and the groups, empty lists rather than none.
type result struct {
	Threshold  float64                 `json:"threshold"`
	Candidates []duplication.Candidate `json:"candidates"`
	Groups     []duplication.Group     `json:"groups"`
}

// Run finds the duplicates the command line asks for, through h and with
// the context cmd made, and prints each group, or the object, or reports
// the failure it met, with what was found before it in the object, and
// returns the exit code the UI gives it.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	opt := duplication.Options{Threshold: c.Threshold, MinLines: c.MinLines, MinNodes: c.MinNodes}
	found, err := h.Handle(ctx, Query{Paths: c.Paths, Changed: c.Changed, Options: opt})
	out := result{Threshold: opt.Threshold, Candidates: []duplication.Candidate{}, Groups: []duplication.Group{}}
	if found.Candidates != nil {
		out.Candidates = found.Candidates
	}
	if found.Groups != nil {
		out.Groups = found.Groups
	}
	if err != nil {
		return ui.Emit(c.JSON, out, ui.Problems(err))
	}
	if found.Excluded > 0 {
		ui.Line(fmt.Sprintf("%d sources left out by exclude in itos-cc.yaml", found.Excluded))
	}
	if found.NoSources {
		ui.Line("no source files to check")
		return ui.Emit(c.JSON, out, nil)
	}
	if !c.JSON {
		for _, g := range out.Groups {
			score := fmt.Sprintf("%.2f", g.MaxScore)
			if g.MinScore != g.MaxScore {
				score = fmt.Sprintf("%.2f–%.2f", g.MinScore, g.MaxScore)
			}
			_, _ = fmt.Fprintf(ui.Stdout, "DUPLICATE score=%s %s\n", score, g.Language)
			for _, s := range g.Members {
				_, _ = fmt.Fprintf(ui.Stdout, "  %s:%d-%d  %s#%s\n", s.File, s.StartLine, s.EndLine, s.Namespace, s.Name)
			}
		}
	}
	return ui.Emit(c.JSON, out, nil)
}
