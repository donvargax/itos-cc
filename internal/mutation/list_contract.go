package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract preserves the list command’s existing help contract.
func (List) Contract() cli.Contract {
	return cli.Contract{
		About: `
Lists each mutation site of the production sources chosen, the changes
mutation run makes one at a time, without running any test, as
"file:line:column ` + "`original` → `replacement`" + ` in namespace#name",
each file's sites in line and column order. The sources itos-cc.yaml's
exclude names are left out, however they are chosen.`,
		JSON: `"sites": [{"file", "line", "column", "function", "original", "replacement"}]`,
		Rules: []string{
			"config.invalid  itos-cc.yaml cannot be read: file, and key for an exclude glob",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: 2, Meaning: "a usage or config error: a bad flag or path, or an itos-cc.yaml that cannot be read"},
			{Code: 3, Meaning: "--changed outside a git repository"},
		},
		Examples: []string{
			"itos-cc mutation list src/billing/invoice.ts",
			"itos-cc mutation list --changed --json",
		},
	}
}
