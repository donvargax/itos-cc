package features

import (
	"errors"
	"testing"

	"github.com/cucumber/godog"
)

// IntegrationDeclarationContract rejects contradictory fixture facts before tool setup.
func IntegrationDeclarationContract(t *testing.T, d Driver) {
	t.Helper()
	for _, c := range []Coverage{{Language: "go", WithoutC8: true}, {Language: "typescript", WithoutC8: true, IntegrationReport: "SF:cli.ts\nDA:1,1\nend_of_record\n"}} {
		if err := d.Coverage("/unused-invalid-fixture", c); err == nil || errors.Is(err, godog.ErrSkip) {
			t.Fatalf("contradictory declaration %+v: %v", c, err)
		}
	}
}
