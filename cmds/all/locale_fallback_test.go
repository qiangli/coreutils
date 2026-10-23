package all_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/tool"
)

func TestUnavailableLocaleFallsBackAcrossCommands(t *testing.T) {
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		args  []string
		input string
	}{
		{"awk", []string{"{print $1}"}, "a b\n"},
		{"comm", []string{"left", "right"}, ""},
		{"cp", []string{"-i", "left", "right"}, "n\n"},
		{"csplit", []string{"-", "/b/"}, "a\nb\n"},
		{"cut", []string{"-c", "1"}, "abc\n"},
		{"date", []string{"-u", "-d", "@0"}, ""},
		{"ed", nil, "q\n"},
		{"expand", nil, "a\tb\n"},
		{"expr", []string{"1", "+", "1"}, ""},
		{"file", []string{"sample"}, ""},
		{"fold", []string{"-w", "2"}, "abcd\n"},
		{"grep", []string{"a"}, "a\n"},
		{"iconv", []string{"-f", "UTF-8"}, "abc\n"},
		{"join", []string{"left", "right"}, ""},
		{"locale", []string{"decimal_point"}, ""},
		{"ls", []string{"-1", "sample"}, ""},
		{"more", nil, "abc\n"},
		{"mv", []string{"-i", "left", "right"}, "n\n"},
		{"od", []string{"-An", "-c"}, "a\n"},
		{"paste", []string{"-"}, "a\n"},
		{"pax", []string{"-w", "-f", "archive", "sample"}, ""},
		{"pr", []string{"-h", "title", "sample"}, ""},
		{"ps", []string{"-p", strconv.Itoa(os.Getpid()), "-o", "pid="}, ""},
		{"rm", []string{"-i", "sample"}, "n\n"},
		{"sed", []string{"s/a/b/"}, "a\n"},
		{"sort", nil, "b\na\n"},
		{"strings", []string{"-n", "1"}, "abc\n"},
		{"time", []string{"-f", "%C", testBinary, "-test.run=^$"}, ""},
		{"tr", []string{"a", "b"}, "a\n"},
		{"unexpand", nil, "        x\n"},
		{"uniq", nil, "a\na\n"},
		{"wc", []string{"-w"}, "a b\n"},
		{"who", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "ps" && runtime.GOOS != "linux" {
				t.Skip("live process inspection is Linux-only")
			}
			for _, setting := range []string{"LANG", "LC_ALL"} {
				for _, fallback := range []struct{ requested, baseline string }{
					{"en_US.UTF-8", "C.UTF-8"},
					{"xx_YY.FOO", "C"},
				} {
					run := func(value string) (int, string, string) {
						dir := t.TempDir()
						for name, body := range map[string]string{"left": "a one\n", "right": "a two\n", "sample": "abc\n"} {
							if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
								t.Fatal(err)
							}
						}
						stamp := time.Date(2024, time.March, 1, 2, 3, 0, 0, time.UTC)
						if err := os.Chtimes(filepath.Join(dir, "sample"), stamp, stamp); err != nil {
							t.Fatal(err)
						}
						var out, errOut bytes.Buffer
						rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
							Env:   []string{"TZ=UTC", "LANG=C", "LC_ALL=", setting + "=" + value},
							Stdio: tool.Stdio{In: strings.NewReader(tc.input), Out: &out, Err: &errOut}}
						return tool.Lookup(tc.name).Run(rc, tc.args), out.String(), errOut.String()
					}
					wantCode, wantOut, wantErr := run(fallback.baseline)
					code, out, errOut := run(fallback.requested)
					if wantCode != 0 || (wantErr != "" && tc.name != "time" && tc.name != "cp" && tc.name != "mv" && tc.name != "rm") {
						t.Fatalf("%s=%s baseline: code=%d stdout=%q stderr=%q", setting, fallback.baseline, wantCode, wantOut, wantErr)
					}
					if code != wantCode || out != wantOut || errOut != wantErr {
						t.Errorf("%s=%s: (%d, %q, %q), want (%d, %q, %q)", setting, fallback.requested, code, out, errOut, wantCode, wantOut, wantErr)
					}
				}
			}
		})
	}
}

func TestLocaleFallbackKeepsUnsupportedFlagsFatal(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Env: []string{"LC_ALL=xx_YY.FOO"}, Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := tool.Lookup("grep").Run(rc, []string{"--no-such-option"}); code != 2 || errOut.Len() == 0 {
		t.Fatalf("unsupported flag: code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}
