package mutate

import (
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// Site is the mutation domain's value; legacy execution migrates separately.
type Site = mutants.Site

func Sites(f *lang.File) []Site                 { return mutants.Sites(f) }
func LineOrder(a, b Site) int                   { return mutants.LineOrder(a, b) }
func UnitHash(f *lang.File, u lang.Unit) string { return mutants.UnitHash(f, u) }
