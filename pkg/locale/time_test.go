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

func TestTimeFormatterFormatLocalizedNamesAndIssue7Conversions(t *testing.T) {
	at := time.Date(2024, time.March, 1, 14, 5, 6, 0, time.UTC)
	for _, tc := range []struct {
		env  []string
		want string
	}{
		{[]string{"LC_TIME=de_DE.UTF-8"}, "Fr|Freitag|Mär|März|Fr 01 Mär 2024 14:05:06 UTC|01.03.2024|14:05:06||"},
		{[]string{"LC_TIME=de_DE.ISO-8859-1"}, "Fr|Freitag|M\xe4r|M\xe4rz|Fr 01 M\xe4r 2024 14:05:06 UTC|01.03.2024|14:05:06||"},
		{[]string{"LC_TIME=de_DE.UTF-8", "LC_ALL=C"}, "Fri|Friday|Mar|March|Fri Mar  1 14:05:06 2024|03/01/24|14:05:06|02:05:06 PM|PM"},
	} {
		formatter, err := ResolveTime(tc.env)
		if err != nil {
			t.Fatal(err)
		}
		got, err := formatter.Format(at, "%a|%A|%b|%B|%c|%x|%X|%r|%p")
		if err != nil || got != tc.want {
			t.Fatalf("env=%v format=%q err=%v, want %q", tc.env, got, err, tc.want)
		}
	}
	formatter, _ := ResolveTime(nil)
	if _, err := formatter.Format(at, "%Q"); err == nil {
		t.Fatal("unsupported conversion unexpectedly accepted")
	}
	if _, err := formatter.Format(at, "%Ot"); err == nil {
		t.Fatal("invalid alternative modifier unexpectedly accepted")
	}
}
