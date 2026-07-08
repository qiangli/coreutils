package tool

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestParseLongOptionAbbreviations(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample [OPTION]..."}

	tests := []struct {
		name      string
		args      []string
		configure func(*testing.T, *pflag.FlagSet)
		check     func(*testing.T, *pflag.FlagSet, []string, int, string, string)
	}{
		{
			name: "unique prefix expands",
			args: []string{"--lin", "file"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.Bool("lines", false, "print line count")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != -1 || err != "" || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				if got, _ := fs.GetBool("lines"); !got {
					t.Fatalf("lines flag = false, want true")
				}
				if want := []string{"file"}; !reflect.DeepEqual(operands, want) {
					t.Fatalf("operands = %v, want %v", operands, want)
				}
			},
		},
		{
			name: "exact match wins over longer candidate",
			args: []string{"--color"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.Bool("color", false, "color output")
				fs.Bool("colorize", false, "colorize output")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != -1 || err != "" || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				if got, _ := fs.GetBool("color"); !got {
					t.Fatalf("color flag = false, want true")
				}
				if got, _ := fs.GetBool("colorize"); got {
					t.Fatalf("colorize flag = true, want false")
				}
			},
		},
		{
			name: "ambiguous prefix reports GNU-style candidates",
			args: []string{"--r"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.Bool("reverse", false, "reverse order")
				fs.Bool("random-sort", false, "shuffle")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != 2 || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				want := "sample: option '--r' is ambiguous; possibilities: '--random-sort' '--reverse'\n"
				if err != want {
					t.Fatalf("err = %q, want %q", err, want)
				}
			},
		},
		{
			name: "terminator stops expansion",
			args: []string{"--", "--lin"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.Bool("lines", false, "print line count")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != -1 || err != "" || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				if got, _ := fs.GetBool("lines"); got {
					t.Fatalf("lines flag = true, want false")
				}
				if want := []string{"--lin"}; !reflect.DeepEqual(operands, want) {
					t.Fatalf("operands = %v, want %v", operands, want)
				}
			},
		},
		{
			name: "long option value that looks like a flag is not rewritten",
			args: []string{"--label", "--lin"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.String("label", "", "label")
				fs.Bool("lines", false, "print line count")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != -1 || err != "" || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				if got, _ := fs.GetString("label"); got != "--lin" {
					t.Fatalf("label = %q, want --lin", got)
				}
				if got, _ := fs.GetBool("lines"); got {
					t.Fatalf("lines flag = true, want false")
				}
			},
		},
		{
			name: "equals value suffix is preserved",
			args: []string{"--out=result.txt"},
			configure: func(t *testing.T, fs *pflag.FlagSet) {
				fs.String("output", "", "output file")
			},
			check: func(t *testing.T, fs *pflag.FlagSet, operands []string, code int, out, err string) {
				if code != -1 || err != "" || out != "" {
					t.Fatalf("Parse code=%d out=%q err=%q", code, out, err)
				}
				if got, _ := fs.GetString("output"); got != "result.txt" {
					t.Fatalf("output = %q, want result.txt", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewFlags(tl.Name)
			tt.configure(t, fs)

			var out, errb bytes.Buffer
			rc := &RunContext{Ctx: context.Background(), Stdio: Stdio{Out: &out, Err: &errb}}
			operands, code := Parse(rc, tl, fs, tt.args)
			tt.check(t, fs, operands, code, out.String(), errb.String())
		})
	}
}

func TestParseLongOptionAbbreviationLeavesUnknownForPflag(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample [OPTION]..."}
	fs := NewFlags(tl.Name)
	fs.Bool("lines", false, "print line count")

	var out, errb bytes.Buffer
	rc := &RunContext{Ctx: context.Background(), Stdio: Stdio{Out: &out, Err: &errb}}
	_, code := Parse(rc, tl, fs, []string{"--unknown"})
	if code != 2 || out.Len() != 0 {
		t.Fatalf("Parse code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "unknown flag: --unknown") {
		t.Fatalf("err = %q, want pflag unknown flag error", errb.String())
	}
}

func TestParseLongOptionAbbreviationIgnoresUniversalAliasNames(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample [OPTION]..."}
	fs := NewFlags(tl.Name)

	var out, errb bytes.Buffer
	rc := &RunContext{Ctx: context.Background(), Stdio: Stdio{Out: &out, Err: &errb}}
	_, code := Parse(rc, tl, fs, []string{"--help-s"})
	if code != 2 || out.Len() != 0 {
		t.Fatalf("Parse code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "unknown flag: --help-s") {
		t.Fatalf("err = %q, want pflag unknown flag error", errb.String())
	}
}
