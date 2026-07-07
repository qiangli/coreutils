// Package truncatecmd implements truncate(1) per the GNU coreutils
// manual: shrink or extend the size of each FILE to the specified size.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/truncate (BSD-3-Clause).
// Changes: rewired to tool framework; size parsing rewritten for the
// GNU suffix table (K/M/G/... powers of 1024, KB/MB/... powers of 1000,
// KiB/MiB/... powers of 1024) and the +/- relative prefixes; truncate
// errors are reported instead of ignored.
package truncatecmd

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "truncate",
	Synopsis: "Shrink or extend the size of each FILE to the specified size.",
	Usage:    "truncate OPTION... FILE...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	noCreate := fs.BoolP("no-create", "c", false, "do not create any files")
	size := fs.StringP("size", "s", "", "set or adjust the file size by SIZE bytes")
	reference := fs.StringP("reference", "r", "", "base the size on RFILE")
	ioBlocks := fs.BoolP("io-blocks", "o", false, "treat SIZE as number of 512-byte I/O blocks")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *size == "" && *reference == "" {
		return tool.UsageError(rc, cmd, "you must specify either '--size' or '--reference'")
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	var value int64
	var rel int
	if *size != "" {
		var perr error
		value, rel, perr = parseSize(*size)
		if perr != nil {
			if errors.Is(perr, errSizePrefix) {
				return tool.NotSupported(rc, cmd, fmt.Sprintf("the '%c' size prefix", (*size)[0]))
			}
			return tool.UsageError(rc, cmd, "invalid number: '%s'", *size)
		}
		if *ioBlocks {
			if value > (1<<63-1)/512 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", *size)
			}
			value *= 512
		}
	}
	refSize := int64(0)
	if *reference != "" {
		fi, err := os.Stat(rc.Path(*reference))
		if err != nil {
			fmt.Fprintf(rc.Err, "truncate: cannot stat '%s': %v\n", *reference, reason(err))
			return 1
		}
		refSize = fi.Size()
		if *size == "" {
			value = refSize
		}
	}

	exit := 0
	for _, name := range operands {
		path := rc.Path(name)
		st, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				fmt.Fprintf(rc.Err, "truncate: cannot stat '%s': %v\n", name, reason(err))
				exit = 1
				continue
			}
			if *noCreate {
				continue // GNU: missing file with -c is silently skipped
			}
			f, cerr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
			if cerr != nil {
				fmt.Fprintf(rc.Err, "truncate: cannot open '%s' for writing: %v\n", name, reason(cerr))
				exit = 1
				continue
			}
			f.Close()
			if st, err = os.Stat(path); err != nil {
				fmt.Fprintf(rc.Err, "truncate: cannot stat '%s': %v\n", name, reason(err))
				exit = 1
				continue
			}
		}
		final := value
		if rel != 0 {
			final = st.Size() + int64(rel)*value
			if *reference != "" {
				final = refSize + int64(rel)*value
			}
		}
		if final < 0 {
			final = 0
		}
		if err := os.Truncate(path, final); err != nil {
			fmt.Fprintf(rc.Err, "truncate: failed to truncate '%s' at %d bytes: %v\n", name, final, reason(err))
			exit = 1
		}
	}
	return exit
}

var errSizePrefix = errors.New("unsupported size prefix")

// multipliers per the GNU size-suffix table.
var multipliers = map[string]int64{
	"":    1,
	"K":   1 << 10,
	"M":   1 << 20,
	"G":   1 << 30,
	"T":   1 << 40,
	"P":   1 << 50,
	"E":   1 << 60,
	"KiB": 1 << 10,
	"MiB": 1 << 20,
	"GiB": 1 << 30,
	"TiB": 1 << 40,
	"PiB": 1 << 50,
	"EiB": 1 << 60,
	"KB":  1e3,
	"MB":  1e6,
	"GB":  1e9,
	"TB":  1e12,
	"PB":  1e15,
	"EB":  1e18,
}

// parseSize parses the -s operand. rel is -1/0/+1 for the relative
// prefixes; GNU's <, >, / and % prefixes are reported as unsupported.
func parseSize(s string) (value int64, rel int, err error) {
	if s == "" {
		return 0, 0, errors.New("empty size")
	}
	switch s[0] {
	case '<', '>', '/', '%':
		return 0, 0, errSizePrefix
	case '+':
		rel = 1
		s = s[1:]
	case '-':
		rel = -1
		s = s[1:]
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, 0, errors.New("no digits")
	}
	value, err = strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	mult, ok := multipliers[s[i:]]
	if !ok {
		return 0, 0, fmt.Errorf("invalid suffix %q", s[i:])
	}
	if value != 0 && mult > 1 {
		if value > (1<<63-1)/mult {
			return 0, 0, errors.New("value too large")
		}
	}
	return value * mult, rel, nil
}

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
