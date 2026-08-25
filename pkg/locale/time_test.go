package locale

import (
	"testing"
	"time"
)

func TestResolveTimeBoundedProviders(t *testing.T) {
	at := time.Date(2024, time.March, 1, 2, 3, 0, 0, time.UTC)
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"default", nil, "Mar  1 02:03"},
		{"C UTF-8", []string{"LC_TIME=C.UTF-8"}, "Mar  1 02:03"},
		{"German UTF-8", []string{"LC_TIME=de_DE.UTF-8"}, "Mär  1 02:03"},
		{"German default Latin-1", []string{"LC_TIME=de_DE"}, "M\xe4r  1 02:03"},
		{"German Latin-1", []string{"LC_TIME=de_DE.ISO-8859-1"}, "M\xe4r  1 02:03"},
		{"LC_ALL precedence", []string{"LC_TIME=POSIX", "LC_ALL=de_DE.UTF-8"}, "Mär  1 02:03"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ResolveTime(tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.FormatMonthDayTime(at); got != tc.want {
				t.Fatalf("FormatMonthDayTime = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveTimeUnsupportedFailsClosed(t *testing.T) {
	if _, err := ResolveTime([]string{"LC_TIME=fr_FR.UTF-8"}); err == nil {
		t.Fatal("unsupported LC_TIME unexpectedly accepted")
	}
}
