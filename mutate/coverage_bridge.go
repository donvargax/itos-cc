package mutate

import (
	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
)

func coverageEvidence(f *lang.File, u lang.Unit, r *coverage.Report, p string, i map[string]string) *CoverageEvidence {
	return mutants.CoverageEvidenceFor(project.FromRoot(f.Path), f, u, r, p, i)
}
func lineCoverageEvidence(f *lang.File, u lang.Unit, l []coverage.Line, proven bool, p string, i map[string]string) *CoverageEvidence {
	return mutants.LineCoverageEvidence(project.FromRoot(f.Path), f, u, l, proven, p, i)
}
func goCoverageEvidence(f *lang.File, u lang.Unit, b []coverage.GoBlock, p string, i map[string]string) *GoCoverageEvidence {
	return mutants.GoCoverageEvidenceFor(project.FromRoot(f.Path), f, u, b, p, i)
}
