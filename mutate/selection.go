package mutate

import (
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func timeoutOf(elapsed time.Duration, factor float64) time.Duration {
	return mutants.TimeoutOf(elapsed, factor)
}
