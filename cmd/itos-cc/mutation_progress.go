package main

import (
	"io"

	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutationslice "github.com/donvargax/itos-cc/internal/mutation"
	"github.com/donvargax/itos-cc/project"
)

// RunProgress presents domain observations through the slice's progress
// wording, folders named from the working directory, until run moves.
type RunProgress struct{ Log io.Writer }

func (p RunProgress) Report(e mutation.RunEvent) {
	mutationslice.Progress{Log: p.Log, Rel: project.Rel}.Report(e)
}
