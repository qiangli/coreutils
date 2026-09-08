//go:build !linux && !darwin && !windows

package resources

import (
	"context"
	"fmt"
)

func collectProcessSamples(context.Context) ([]processSample, ObservationCoverage, error) {
	return nil, ObservationCoverage{Reason: "native process source unsupported"}, fmt.Errorf("native process observations unavailable on this OS")
}
