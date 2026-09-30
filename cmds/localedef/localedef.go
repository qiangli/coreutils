// Package localedefcmd provides the parse/validate stage of POSIX localedef.
// Locale-store compilation is deliberately deferred to the next story.
package localedefcmd

import (
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/coreutils/pkg/localedef"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "localedef",
	Synopsis: "Parse and validate locale definitions (no locale output yet).",
	Usage:    "localedef [-c] [-f charmap] [-i sourcefile] [-u codeset] name",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// Options are retained separately for the subsequent compilation stage. In
// this parse-only stage Force and CodeSet do not cause output or transcoding.
type Options struct {
	Force                          bool
	Charmap, Source, CodeSet, Name string
}

func parseOptions(args []string) (Options, error) {
	var o Options
	for len(args) > 0 {
		a := args[0]
		args = args[1:]
		if a == "--" {
			break
		}
		if len(a) < 2 || a[0] != '-' {
			args = append([]string{a}, args...)
			break
		}
		for i := 1; i < len(a); i++ {
			flag := a[i]
			if flag == 'c' {
				o.Force = true
				continue
			}
			if flag != 'f' && flag != 'i' && flag != 'u' {
				return o, fmt.Errorf("unknown option -%c", flag)
			}
			value := a[i+1:]
			if value == "" {
				if len(args) == 0 {
					return o, fmt.Errorf("option -%c requires an argument", flag)
				}
				value = args[0]
				args = args[1:]
			}
			switch flag {
			case 'f':
				o.Charmap = value
			case 'i':
				o.Source = value
			case 'u':
				o.CodeSet = value
			}
			break
		}
	}
	if len(args) == 0 || args[0] == "" {
		return o, fmt.Errorf("missing locale name")
	}
	if len(args) != 1 {
		return o, fmt.Errorf("extra operand %q", args[1])
	}
	o.Name = args[0]
	return o, nil
}
func run(rc *tool.RunContext, args []string) int {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintf(rc.Out, "Usage: %s\n%s\n-c  continue despite warnings\n-f  character map file\n-i  locale source file (default: stdin)\n-u  target codeset (retained for compilation)\n", cmd.Usage, cmd.Synopsis)
		return 0
	}
	fail := func(err error) int { fmt.Fprintf(rc.Err, "localedef: %v\n", err); return 4 }
	o, err := parseOptions(args)
	if err != nil {
		return fail(err)
	}
	if rc.FS == nil {
		rc.FS = tool.NewLocalFS()
	}
	cm := localedef.DefaultCharmap()
	if o.Charmap != "" {
		f, err := rc.FS.Open(rc.Path(o.Charmap))
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		var input io.Reader = f
		if strings.HasSuffix(o.Charmap, ".gz") {
			z, err := gzip.NewReader(f)
			if err != nil {
				return fail(fmt.Errorf("%s: %w", o.Charmap, err))
			}
			defer z.Close()
			input = z
		}
		cm, err = localedef.ParseCharmap(input)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", o.Charmap, err))
		}
	}
	input := rc.In
	label := "stdin"
	if o.Source != "" {
		f, err := rc.FS.Open(rc.Path(o.Source))
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		input = f
		label = o.Source
	}
	src, err := localedef.ParseSource(input)
	if err != nil {
		return fail(fmt.Errorf("%s: %w", label, err))
	}
	code := 0
	for _, d := range localedef.Validate(src, cm) {
		severity := "error"
		if d.Warning {
			severity = "warning"
			if code == 0 {
				code = 1
			}
		} else {
			code = 4
		}
		fmt.Fprintf(rc.Err, "localedef: %s: %s: %s\n", label, severity, d.Error())
	}
	return code
}
