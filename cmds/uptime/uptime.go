// Package uptimecmd implements uptime(1) (GNU/procps output shape):
// current time, how long the system has been running, and the load
// averages where the platform provides them.
//
// Platform probes: /proc/uptime on Linux, sysctl kern.boottime on
// darwin, GetTickCount64 on Windows. Load averages print on Linux
// only (/proc/loadavg); other platforms omit the field. The logged-in
// user count is omitted on every platform — counting sessions purely
// from Go would be a guess, and a clear omission beats a wrong
// number.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/uptime/uptime.go (BSD-3-Clause).
// Changes: rewired to the tool framework; procps duration formatting
// (the prior art mis-renders >24h spans via time.Time); darwin and
// Windows probes added.
package uptimecmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uptime",
	Synopsis: "Tell how long the system has been running.",
	Usage:    "uptime [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	pretty := fs.BoolP("pretty", "p", false, "show uptime in pretty format")
	since := fs.BoolP("since", "s", false, "system up since")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	up, err := uptimeDuration()
	if err != nil {
		fmt.Fprintf(rc.Err, "uptime: cannot determine uptime: %v\n", err)
		return 1
	}
	if *pretty {
		fmt.Fprintf(rc.Out, "up %s\n", formatPrettyUptime(up))
		return 0
	}
	if *since {
		fmt.Fprintf(rc.Out, "%s\n", time.Now().Add(-up).Format("2006-01-02 15:04:05"))
		return 0
	}

	line := fmt.Sprintf(" %s up %s", time.Now().Format("15:04:05"), formatUptime(up))
	if load, ok := loadAverages(); ok {
		line += ",  load average: " + load
	}
	fmt.Fprintf(rc.Out, "%s\n", line)
	return 0
}

func formatPrettyUptime(d time.Duration) string {
	mins := int64(d.Minutes())
	days := mins / (24 * 60)
	hours := (mins % (24 * 60)) / 60
	minutes := mins % 60
	var parts []string
	if days == 1 {
		parts = append(parts, "1 day")
	} else if days > 1 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}
	if hours == 1 {
		parts = append(parts, "1 hour")
	} else if hours > 1 {
		parts = append(parts, fmt.Sprintf("%d hours", hours))
	}
	if minutes == 1 {
		parts = append(parts, "1 minute")
	} else if minutes > 1 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d minutes", minutes))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", " + parts[len(parts)-1]
}

// formatUptime renders the procps duration shape: "N days," when at
// least a day, then "H:MM" past the first hour or "N min" under it.
func formatUptime(d time.Duration) string {
	mins := int64(d.Minutes())
	days := mins / (24 * 60)
	hours := (mins % (24 * 60)) / 60
	minutes := mins % 60

	out := ""
	if days == 1 {
		out = "1 day, "
	} else if days > 1 {
		out = fmt.Sprintf("%d days, ", days)
	}
	if hours > 0 {
		return out + fmt.Sprintf("%d:%02d", hours, minutes)
	}
	return out + fmt.Sprintf("%d min", minutes)
}
