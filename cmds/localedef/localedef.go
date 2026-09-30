// Package localedefcmd implements POSIX localedef: it parses and validates a
// locale definition and compiles it into the Go-owned locale store, where
// pkg/locale, locale(1) and the consuming utilities read it back.
//
// The store is LOCPATH-style: $LOCPATH when set, else $HOME/.bashy/locale.
// Nothing outside that directory is touched — a pure-Go localedef has no
// business writing into a libc locale archive it cannot describe.
package localedefcmd

import (
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/localedef"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "localedef",
	Synopsis: "Compile a locale definition into the locale store.",
	Usage:    "localedef [-c] [-f charmap] [-i sourcefile] [-u codeset] name",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// Options carry the parsed command line. CodeSet is parsed so unsupported
// -u conversion is diagnosed explicitly rather than silently relabeling bytes.
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
		fmt.Fprintf(rc.Out, "Usage: %s\n%s\n-c  continue despite warnings\n-f  character map file\n-i  locale source file (default: stdin)\n-u  target codeset conversion (not supported)\n", cmd.Usage, cmd.Synopsis)
		return 0
	}
	fail := func(err error) int { fmt.Fprintf(rc.Err, "localedef: %v\n", err); return 4 }
	o, err := parseOptions(args)
	if err != nil {
		return fail(err)
	}
	if o.CodeSet != "" {
		fmt.Fprintf(rc.Err, "localedef: -u %s: codeset conversion not supported\n", o.CodeSet)
		return 2
	}
	storeEnv := locale.StoreEnvAt(rc.Env, rc.Path)
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
	// POSIX.1-2017: errors never create permanent output. Warnings
	// create output with -c; without it this implementation refuses output.
	if code == 4 || (code == 1 && !o.Force) {
		return 4
	}
	// A "copy" directive is resolved against the store: the only locale we can
	// honour a copy of is one we compiled ourselves.
	resolve := func(name string) (*locale.Compiled, bool) { return locale.LookupCompiled(storeEnv, name) }
	compiled, err := localedef.CompileWithCopy(o.Name, src, cm, resolve)
	if err != nil {
		fmt.Fprintf(rc.Err, "localedef: %s: %v\n", label, err)
		return 4
	}
	dir, err := locale.DefaultStoreDir(storeEnv)
	if err != nil {
		return fail(err)
	}
	if err := locale.Save(dir, compiled); err != nil {
		return fail(err)
	}
	return code
}
