package lognamecmd

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "logname",
	Synopsis: "Print the user's login name.",
	Usage:    "logname [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return runWith(rc, args, loginName)
}

// runWith is the testable core of run: resolve the login name via resolve
// and print it to stdout, or fail per POSIX when no login name is available.
func runWith(rc *tool.RunContext, args []string, resolve func() string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}
	name := resolve()
	if name == "" {
		fmt.Fprintln(rc.Err, "logname: no login name")
		return 1
	}
	fmt.Fprintln(rc.Out, name)
	return 0
}

// loginName returns the name of the user logged into the current session,
// matching the POSIX getlogin() contract. It deliberately ignores the
// LOGNAME, USER, LNAME and USERNAME environment variables, which POSIX
// requires logname not to consult.
func loginName() string {
	if name := loginNameFromLoginUID(); name != "" {
		return name
	}
	// Pure-Go fallback for platforms without /proc/self/loginuid (macOS,
	// Windows) or for Linux sessions with no recorded audit login uid:
	// use the effective account. POSIX getlogin() may differ after
	// su/sudo, but libc getlogin() cannot be called from pure Go without
	// cgo, so this is the best available approximation off Linux.
	if u, err := user.Current(); err == nil {
		return bareUser(strings.TrimSpace(u.Username))
	}
	return ""
}

// loginNameFromLoginUID resolves the session login user on Linux from
// /proc/self/loginuid, the kernel's audit record of who logged in. This
// value persists across su/sudo, matching getlogin(). An unset login uid
// (4294967295) or an absent /proc yields "" so the caller can fall back.
func loginNameFromLoginUID() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/proc/self/loginuid")
	if err != nil {
		return ""
	}
	return resolveLoginUID(string(data))
}

// resolveLoginUID maps a login-uid string to a user name via the passwd
// database. Unset, empty, or unresolvable uids return "".
func resolveLoginUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" || isUnsetLoginUID(uid) {
		return ""
	}
	if u, err := user.LookupId(uid); err == nil && u.Username != "" {
		return bareUser(strings.TrimSpace(u.Username))
	}
	return ""
}

// isUnsetLoginUID reports whether uid is the sentinel meaning "no login
// user recorded". The kernel writes it as the unsigned value 4294967295;
// "-1" is accepted defensively.
func isUnsetLoginUID(uid string) bool {
	return uid == "4294967295" || uid == "-1"
}

// bareUser strips a Windows domain prefix ("DOMAIN\user" -> "user") so the
// printed name matches the Unix bare-username convention on every platform.
func bareUser(s string) string {
	if runtime.GOOS == "windows" {
		if i := strings.LastIndexByte(s, '\\'); i >= 0 {
			return s[i+1:]
		}
	}
	return s
}
