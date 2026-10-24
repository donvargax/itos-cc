package settings

import (
	"context"
	"errors"
	"io/fs"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Source supplies current raw configuration at its actual project root.
// Cmd selects the disk or memory byte source; Reader owns the single codec.
// Its error is what kept the file there from being read.
type Source interface {
	Read(ctx context.Context) (root string, text []byte, err error)
}

// Reader adapts the existing config codec to mutation's settings capability
// and to the selection engine's, so both are read through one codec and
// classify a file alike. It performs no eager read and stores no context or
// parsed snapshot: each read observes the source once. The source owns
// cancellation of its actual read; a completed read is still parsed in the
// old order, including disk reads which cannot be interrupted by a context.
// A file the source could not read is invalid settings, as one that cannot be
// parsed is (bug-4), unless the context's cancellation stopped the read.
type Reader struct{ Source Source }

var (
	_ mutants.SettingsReader = Reader{}
	_ selport.SettingsReader = Reader{}
)

// Read is mutation's settings, the selection settings' root the one the
// source read the file at.
func (r Reader) Read(ctx context.Context) (*mutants.Config, mutants.SettingsFailure) {
	if r.Source == nil {
		return nil, mutants.InputFailed{Cause: errors.New("mutation settings byte source binding is missing")}
	}
	root, text, err := r.Source.Read(ctx)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, context.Cause(ctx)) {
			return nil, mutants.InputCancelled{Cause: context.Cause(ctx)}
		}
		return nil, mutants.SettingsInvalid{Reason: unreadable(err)}
	}
	config, err := Parse(text)
	if err != nil {
		var invalid *InvalidError
		if errors.As(err, &invalid) {
			return nil, mutants.SettingsInvalid{Reason: invalid.Reason, Key: invalid.Key}
		}
		return nil, mutants.InputFailed{Cause: err}
	}
	config.Selection.Root = root
	return config, nil
}

// ReadSelection is the selection engine's settings, read as Read reads
// mutation's, from one observation of the source, each failure the
// selection port's of the same kind.
func (r Reader) ReadSelection(ctx context.Context) (selport.Settings, selport.SettingsFailure) {
	config, failure := r.Read(ctx)
	switch failure := failure.(type) {
	case nil:
		return config.Selection, nil
	case mutants.SettingsInvalid:
		return selport.Settings{}, &selport.SettingsInvalid{Reason: failure.Reason, Key: failure.Key}
	case mutants.InputCancelled:
		return selport.Settings{}, &selport.Cancelled{Cause: failure.Cause}
	case mutants.InputFailed:
		return selport.Settings{}, &selport.SettingsFailed{Cause: failure.Cause}
	}
	return selport.Settings{}, &selport.SettingsFailed{Cause: failure}
}

// unreadable is the reason itos-cc.yaml could not be read, err saying why:
// the system's words without the path, which the failure names as
// itos-cc.yaml.
func unreadable(err error) string {
	var path *fs.PathError
	if errors.As(err, &path) {
		err = path.Err
	}
	return "cannot be read: " + err.Error()
}
