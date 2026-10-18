package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The pressure scrap records across runs, the verdict it gives and the
// warnings it prints.

// pressureFixtures are single TypeScript examples by the pressure scrap
// gives their file, the score less one: decisions add 2.5, mocks beyond two
// 1.5 each, lines beyond fifteen a fifth each.
var pressureFixtures = map[float64]scrapMetrics{
	10:  {Lines: 8, Decisions: 4, Assertions: 1},
	4:   {Lines: 8, Decisions: 1, Mocks: 3, Assertions: 1},
	3.2: {Lines: 16, Mocks: 4, Assertions: 1},
	3:   {Lines: 8, Mocks: 4, Assertions: 1},
}

func pressureFixture(pressure float64) (string, error) {
	m, ok := pressureFixtures[pressure]
	if !ok {
		return "", fmt.Errorf("no fixture has the pressure %v", pressure)
	}
	return tsExample("pressed", m)
}

// pressureFile is the file whose pressure the progress scenarios follow.
const pressureFile = "pressed.test.ts"

// recordedPressure writes the file with a fixture of the pressure and runs
// scrap, which records it in .metrics/scrap.json, and checks it recorded
// that pressure.
func (d *scrapState) recordedPressure(pressure float64) error {
	before, err := pressureFixture(pressure)
	if err != nil {
		return err
	}
	d.before = before
	if err := d.writeFile(pressureFile, before); err != nil {
		return err
	}
	if _, err := d.scrapIn(d.w.dir); err != nil {
		return err
	}
	snapshot, err := d.snapshot()
	if err != nil {
		return err
	}
	if got := snapshot[pressureFile].Pressure; got != pressure {
		return fmt.Errorf(".metrics/scrap.json recorded %v for %s, not %v", got, pressureFile, pressure)
	}
	d.beforePressure = pressure
	return nil
}

// runWithPressure rewrites the file with a fixture of the pressure, runs
// itos-cc with args, and checks the run measured that pressure.
func (d *scrapState) runWithPressure(args string, pressure float64) error {
	after, err := pressureFixture(pressure)
	if err != nil {
		return err
	}
	d.after = after
	if err := d.w.write(pressureFile, after); err != nil {
		return err
	}
	if err := d.w.runWith(d.w.env(), args); err != nil {
		return err
	}
	r, err := d.its()
	if err != nil {
		return err
	}
	if r.Pressure != pressure {
		return fmt.Errorf("the run measured %s's pressure as %v, not %v", r.File, r.Pressure, pressure)
	}
	return nil
}

// verdictIs is whether the run judged the file so: in its line, a verdict
// other than unchanged with the previous pressure; and in --json's
// compare, the same two over again in a folder of their own.
func (d *scrapState) verdictIs(verdict string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	var line []string
	for _, f := range files {
		if f.fields[1] == pressureFile {
			line = f.fields
		}
	}
	if line == nil {
		return fmt.Errorf("%s is not printed\n%s", pressureFile, d.w.report())
	}
	if verdict == "unchanged" && line[7] != "" || verdict != "unchanged" && (line[7] != verdict || line[8] != fmt.Sprintf("%.1f", d.beforePressure)) {
		return fmt.Errorf("%q gives no verdict %s from %.1f", line[0], verdict, d.beforePressure)
	}
	again := filepath.Join(d.w.dir, "again")
	if err := d.w.writeIn(again, pressureFile, d.before); err != nil {
		return err
	}
	if _, err := d.scrapIn(again); err != nil {
		return err
	}
	if err := d.w.writeIn(again, pressureFile, d.after); err != nil {
		return err
	}
	reports, err := d.scrapIn(again)
	if err != nil {
		return err
	}
	if c := reports[pressureFile].Compare; c == nil || c.Verdict != verdict || c.PreviousPressure != d.beforePressure {
		return fmt.Errorf("--json compares %s as %+v, not %s from %v", pressureFile, c, verdict, d.beforePressure)
	}
	return nil
}

// lineEndsWith is whether the file's line ends with the words and the
// previous pressure.
func (d *scrapState) lineEndsWith(ending string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("%s %.1f", ending, d.beforePressure)
	for _, f := range files {
		if f.fields[1] == d.file {
			if !strings.HasSuffix(f.line, want) {
				return fmt.Errorf("%q does not end with %q", f.line, want)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not printed\n%s", d.file, d.w.report())
}

// warningSays is whether a warning under the file's line says to do what
// the sentence says before keeping the change.
func (d *scrapState) warningSays(what string) error {
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] != d.file {
			continue
		}
		for _, warning := range f.warnings {
			if saysInOrder(warning, what+" before keeping") {
				return nil
			}
		}
		return fmt.Errorf("no warning under %q says to %s before keeping the change: %q", f.line, what, f.warnings)
	}
	return fmt.Errorf("%s is not printed\n%s", d.file, d.w.report())
}

// holdsReports writes the two test files and runs scrap, which records
// both, then changes the second, so a report of it measured again would
// differ from the one kept.
func (d *scrapState) holdsReports(first, second string) error {
	one, _ := tsExample("adds", scrapMetrics{Lines: 3, Assertions: 1})
	two, _ := tsExample("removes", scrapMetrics{Lines: 4, Assertions: 1})
	if err := d.writeFiles(map[string]string{first: one, second: two}); err != nil {
		return err
	}
	if _, err := d.scrapIn(d.w.dir); err != nil {
		return err
	}
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return err
	}
	var snapshot struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	for _, f := range snapshot.Files {
		var r scrapReport
		if json.Unmarshal(f, &r) == nil && r.File == second {
			d.kept = f
		}
	}
	if d.kept == nil {
		return fmt.Errorf(".metrics/scrap.json holds no report of %s\n%s", second, raw)
	}
	changed, _ := tsExample("removes", scrapMetrics{Lines: 4})
	return d.w.write(second, two+"\n"+strings.Replace(changed, "removes", "removes again", 1))
}

// stillHolds is whether .metrics/scrap.json holds the report of file it
// held before the run, unchanged, beside the run's own.
func (d *scrapState) stillHolds(file string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return err
	}
	var snapshot struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	held, others := false, 0
	for _, f := range snapshot.Files {
		var r scrapReport
		if json.Unmarshal(f, &r) != nil {
			continue
		}
		if r.File == file {
			var got, want any
			_ = json.Unmarshal(f, &got)
			_ = json.Unmarshal(d.kept, &want)
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				return fmt.Errorf(".metrics/scrap.json holds\n%s\nof %s, not\n%s", a, file, b)
			}
			held = true
		} else {
			others++
		}
	}
	if !held || others == 0 {
		return fmt.Errorf(".metrics/scrap.json holds %s %v, beside %d others\n%s", file, held, others, raw)
	}
	return nil
}

// runCompletes is whether the run exited 0, printed the file, and wrote a
// snapshot that can be read.
func (d *scrapState) runCompletes() error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	if _, ok := reports[d.file]; !ok {
		return fmt.Errorf("the run does not report %s\n%s", d.file, d.w.report())
	}
	return nil
}
