//go:build bashy_scratch && !linux

package schedule

import (
	"errors"
	"testing"
)

func TestScratchHostLoadAverageUnsupported(t *testing.T) {
	load, err := HostLoadAverage()
	if load != 0 || !errors.Is(err, errScratchLoadAverageUnsupported) {
		t.Fatalf("HostLoadAverage() = (%v, %v), want (0, %v)", load, err, errScratchLoadAverageUnsupported)
	}
}
