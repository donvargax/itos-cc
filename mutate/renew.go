package mutate

import (
	"context"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
)

const (
	RenewNoResults = mutants.RenewNoResults
	RenewAmbiguous = mutants.RenewAmbiguous
	RenewUncovered = mutants.RenewUncovered
	MatchLine      = mutants.MatchLine
	MatchPlace     = mutants.MatchPlace
	MatchOnly      = mutants.MatchOnly
)

type Renewal = mutants.Renewal
type Unrenewed = mutants.Unrenewed

func Renew(ctx context.Context, store snapport.Store, exceptions []config.Exception, sources []string, tests func(string) []string, support map[string]string) ([]Renewal, []Unrenewed, error) {
	return RenewWithInputs(ctx, store, exceptions, sources, tests, support, nil, nil)
}
func RenewWithInputs(ctx context.Context, store snapport.Store, exceptions []config.Exception, sources []string, tests func(string) []string, support map[string]string, tools mutants.Toolchain, fingerprints mutants.Fingerprints) ([]Renewal, []Unrenewed, error) {
	r, left, err := mutants.Renew(ctx, exceptions, sources, legacyCheckOptions(store, nil, tests, support, nil, tools, fingerprints), workdir.SourceEdits{})
	return r, left, runError(err)
}
func LineText(src []byte, line int) string { return mutants.LineText(src, line) }
