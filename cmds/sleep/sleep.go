// Package sleepcmd implements sleep(1) per the GNU coreutils manual:
// pause for NUMBER seconds, where NUMBER may be fractional and may
// carry an s/m/h/d suffix; multiple arguments are summed.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/sleep/sleep.go (BSD-3-Clause).
// Changes: rewired to the tool framework; GNU s/m/h/d suffixes and
// multi-operand summing; RunContext.Ctx cancellation.
package sleepcmd

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sleep",
	Synopsis: "Pause for NUMBER seconds (suffixes: s, m, h, d; NUMBER may be fractional).",
	Usage:    "sleep NUMBER[smhd]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	total := 0.0
	for _, op := range operands {
		secs, err := parseInterval(op)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid time interval %q", op)
		}
		total += secs
	}

	d := time.Duration(math.MaxInt64)
	if sec := total * float64(time.Second); sec < float64(math.MaxInt64) {
		d = time.Duration(sec)
	}
	if d <= 0 {
		return 0
	}

	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return 0
	case <-ctx.Done():
		// Cancelled by the embedder: exit quietly.
		return 1
	}
}

// parseInterval converts one NUMBER[smhd] operand to seconds. GNU
// accepts any strtod float (including exponents) with one optional
// lowercase suffix.
func parseInterval(s string) (float64, error) {
	mult := 1.0
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 's':
			s = s[:len(s)-1]
		case 'm':
			mult = 60
			s = s[:len(s)-1]
		case 'h':
			mult = 60 * 60
			s = s[:len(s)-1]
		case 'd':
			mult = 24 * 60 * 60
			s = s[:len(s)-1]
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsNaN(v) {
		return 0, strconv.ErrSyntax
	}
	return v * mult, nil
}
