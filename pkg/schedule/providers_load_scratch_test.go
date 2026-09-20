//go:build bashy_scratch

package schedule

import (
	"strings"
	"testing"
)

func TestParseProcLoadAverage(t *testing.T) {
	tests := []struct {
		name string
		data string
		want float64
	}{
		{name: "kernel format", data: "1.25 0.75 0.50 2/100 1234\n", want: 1.25},
		{name: "whitespace", data: "\t0.00   1.00 2.00\n", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseProcLoadAverage([]byte(test.data))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("load average = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseProcLoadAverageRejectsInvalidInput(t *testing.T) {
	for _, data := range []string{"", "not-a-number 0.1 0.2", "-0.1 0.1 0.2", "NaN 0.1 0.2", "+Inf 0.1 0.2"} {
		if _, err := parseProcLoadAverage([]byte(data)); err == nil || !strings.Contains(err.Error(), "/proc/loadavg") {
			t.Errorf("parseProcLoadAverage(%q) error = %v, want /proc/loadavg diagnostic", data, err)
		}
	}
}
