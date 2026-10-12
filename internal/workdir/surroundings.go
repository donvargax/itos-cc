package workdir

import (
	"context"

	langport "github.com/donvargax/itos-cc/lang/port"
	"github.com/donvargax/itos-cc/project"
)

// Surroundings are what surrounds the files of this process's working
// directory, as lang's port asks for them: project's answers from the disk,
// so a slice names units through them without reaching project itself.
type Surroundings struct{}

var _ langport.Surroundings = Surroundings{}

// Exists says whether path is there, as project.Surroundings stats it.
func (Surroundings) Exists(ctx context.Context, path string) bool {
	return project.Surroundings{}.Exists(ctx, path)
}

// Read is what the file at path holds, as project.Surroundings reads it.
func (Surroundings) Read(ctx context.Context, path string) ([]byte, bool) {
	return project.Surroundings{}.Read(ctx, path)
}

// WorkingDir is the working directory, as project.Surroundings tells it.
func (Surroundings) WorkingDir(ctx context.Context) (string, bool) {
	return project.Surroundings{}.WorkingDir(ctx)
}

// Abs is path made absolute, as project.Surroundings makes it.
func (Surroundings) Abs(ctx context.Context, path string) (string, bool) {
	return project.Surroundings{}.Abs(ctx, path)
}
