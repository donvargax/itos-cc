package main

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/config"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

// adapters are what the moved commands reach the world through, which main
// gives run: the working directory, the ports of the domains they use and
// itos-cc.yaml. A test runs itos-cc in process with porttest's fakes and a
// file held in memory instead, which enter cmd through its _test.go files
// alone (decision 44). The hand-parsed commands reach the disk themselves
// until they move.
type adapters struct {
	// Dir is the working directory, absolute, which the UI names the files
	// a person reads from; "" when it cannot be told.
	Dir string
	// Tree is selection's port, the working tree the commands select from.
	Tree selport.Tree
	// Surroundings is lang's port, what surrounds a file whose units it
	// names.
	Surroundings langport.Surroundings
	// Store is the snapshot port, the snapshots under .metrics/.
	Store snapport.Store
	// Config is itos-cc.yaml.
	Config configFile
}

// configFile is itos-cc.yaml as the moved commands read it.
type configFile interface {
	// Read is the project root, where itos-cc.yaml is, and the file's text
	// there, nil when there is none, or the error that kept it from being
	// read.
	Read(ctx context.Context) (root string, text []byte, err error)
}

// diskAdapters are this process's: its working directory,
// internal/workdir's tree and surroundings, internal/metricsdir's store and
// itos-cc.yaml at the project root on the disk.
func diskAdapters() adapters {
	dir, _ := os.Getwd()
	return adapters{Dir: dir, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Store: metricsdir.Store{}, Config: diskConfig{}}
}

// diskConfig is itos-cc.yaml at the project root of the working directory.
type diskConfig struct{}

// Read is config.Read at project.Root. Neither asks anything a context
// could cancel.
func (diskConfig) Read(context.Context) (string, []byte, error) {
	root := project.Root()
	text, err := config.Read(root)
	return root, text, err
}

// exclusionOf is the sources itos-cc.yaml's exclude leaves out, its globs
// from the root file gives with the file's text, read under ctx: a file
// that cannot be read as a config is config.invalid.
func exclusionOf(ctx context.Context, file configFile) (selection.Exclusion, error) {
	root, text, err := file.Read(ctx)
	if err != nil {
		return selection.Exclusion{}, err
	}
	cfg, err := config.Parse(text)
	if err != nil {
		return selection.Exclusion{}, invalidConfig(err)
	}
	return selection.Exclusion{Root: root, Globs: cfg.Exclude}, nil
}
