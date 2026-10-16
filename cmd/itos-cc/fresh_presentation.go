package main

import (
	"github.com/donvargax/itos-cc/config"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
)

type FreshPreparation = mutation.FreshPreparation

// PreparationStage is the counted report representation of one observed stage.
type PreparationStage struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// freshPreparationMessage retains the existing committed-config wording at UI.
func freshPreparationMessage(failure mutation.FreshFailure) string {
	switch f := failure.(type) {
	case mutation.FreshInvalid:
		return "read frozen committed config: " + config.File + ": " + f.Reason
	case mutation.FreshRefused:
		return f.Cause.Error()
	case mutation.FreshFailed:
		return f.Cause.Error()
	case mutation.FreshCancelled:
		return f.Cause.Error()
	}
	return ""
}
