package perfbenchcmd

import "github.com/qiangli/coreutils/tool"

// Group is one of the three original GNU packages that merged into coreutils
// (fileutils + sh-utils + textutils, 2002–03). Every program is folded into
// exactly one, so the fidelity/perf scoreboards report per-group.
type Group string

const (
	FileUtils Group = "fileutils"
	ShUtils   Group = "sh-utils"
	TextUtils Group = "textutils"
)

// Program is one GNU coreutils command. The inventory below is the COMPLETE
// coreutils 9.11 program set — cross-checked against the source tree
// (github.com/coreutils/coreutils/tree/master/src, one .c per command) and the
// three-package grouping documented in the coreutils 4.5.4 manual
// (ftp.gnu.org/old-gnu/Manuals/coreutils-4.5.4/html_mono/coreutils.html), with
// post-merge additions folded into the nearest original group.
//
// Historical=true means the program shipped in the named original package;
// Historical=false is a post-merge addition (arch, base32/64, basenc, b2sum,
// sha224..512sum, shuf, numfmt, mktemp, readlink, realpath, truncate, timeout,
// stdbuf, nproc, chcon, runcon, kill, hostname, `[`).
type Program struct {
	Name       string
	Group      Group
	Historical bool
}

// Inventory is the authoritative complete list. Present-ness is NOT stored here
// — it is computed at runtime from the tool registry (Present), so the table
// stays a pure GNU-side reference that never drifts from what the reimpl ships.
//
// Count: 106 programs (fileutils 27 · sh-utils 43 · textutils 36). `[` is the
// alias of test; the `coreutils` multicall dispatcher is not a user command.
var Inventory = []Program{
	// ---- fileutils (directory listing, basic ops, special files, attributes, disk usage) ----
	{"chgrp", FileUtils, true}, {"chmod", FileUtils, true}, {"chown", FileUtils, true},
	{"cp", FileUtils, true}, {"dd", FileUtils, true}, {"df", FileUtils, true},
	{"dir", FileUtils, true}, {"dircolors", FileUtils, true}, {"du", FileUtils, true},
	{"install", FileUtils, true}, {"ln", FileUtils, true}, {"ls", FileUtils, true},
	{"mkdir", FileUtils, true}, {"mkfifo", FileUtils, true}, {"mknod", FileUtils, true},
	{"mv", FileUtils, true}, {"rm", FileUtils, true}, {"rmdir", FileUtils, true},
	{"shred", FileUtils, true}, {"stat", FileUtils, true}, {"sync", FileUtils, true},
	{"touch", FileUtils, true}, {"vdir", FileUtils, true},
	{"mktemp", FileUtils, false}, {"readlink", FileUtils, false},
	{"realpath", FileUtils, false}, {"truncate", FileUtils, false},

	// ---- sh-utils (printing, conditions, redirection, context, user/system info,
	//      modified invocation, process control, delay, numeric) ----
	{"basename", ShUtils, true}, {"chroot", ShUtils, true}, {"date", ShUtils, true},
	{"dirname", ShUtils, true}, {"echo", ShUtils, true}, {"env", ShUtils, true},
	{"expr", ShUtils, true}, {"factor", ShUtils, true}, {"false", ShUtils, true},
	{"groups", ShUtils, true}, {"hostid", ShUtils, true}, {"id", ShUtils, true},
	{"logname", ShUtils, true}, {"nice", ShUtils, true}, {"nohup", ShUtils, true},
	{"pathchk", ShUtils, true}, {"pinky", ShUtils, true}, {"printenv", ShUtils, true},
	{"printf", ShUtils, true}, {"pwd", ShUtils, true}, {"seq", ShUtils, true},
	{"sleep", ShUtils, true}, {"stty", ShUtils, true}, {"tee", ShUtils, true},
	{"test", ShUtils, true}, {"true", ShUtils, true}, {"tty", ShUtils, true},
	{"uname", ShUtils, true}, {"uptime", ShUtils, true}, {"users", ShUtils, true},
	{"who", ShUtils, true}, {"whoami", ShUtils, true}, {"yes", ShUtils, true},
	{"[", ShUtils, false}, {"arch", ShUtils, false}, {"chcon", ShUtils, false},
	{"hostname", ShUtils, false}, {"kill", ShUtils, false}, {"nproc", ShUtils, false},
	{"numfmt", ShUtils, false}, {"runcon", ShUtils, false}, {"stdbuf", ShUtils, false},
	{"timeout", ShUtils, false},

	// ---- textutils (output whole files, formatting, parts of files, summarizing,
	//      sorted files, fields, characters) ----
	{"cat", TextUtils, true}, {"cksum", TextUtils, true}, {"comm", TextUtils, true},
	{"csplit", TextUtils, true}, {"cut", TextUtils, true}, {"expand", TextUtils, true},
	{"fmt", TextUtils, true}, {"fold", TextUtils, true}, {"head", TextUtils, true},
	{"join", TextUtils, true}, {"md5sum", TextUtils, true}, {"nl", TextUtils, true},
	{"od", TextUtils, true}, {"paste", TextUtils, true}, {"pr", TextUtils, true},
	{"ptx", TextUtils, true}, {"sha1sum", TextUtils, true}, {"sort", TextUtils, true},
	{"split", TextUtils, true}, {"sum", TextUtils, true}, {"tac", TextUtils, true},
	{"tail", TextUtils, true}, {"tr", TextUtils, true}, {"tsort", TextUtils, true},
	{"unexpand", TextUtils, true}, {"uniq", TextUtils, true}, {"wc", TextUtils, true},
	{"base32", TextUtils, false}, {"base64", TextUtils, false}, {"basenc", TextUtils, false},
	{"b2sum", TextUtils, false}, {"sha224sum", TextUtils, false}, {"sha256sum", TextUtils, false},
	{"sha384sum", TextUtils, false}, {"sha512sum", TextUtils, false}, {"shuf", TextUtils, false},
}

// deliberate is the set of GNU programs the reimpl will NEVER implement (the
// "NO — not supported" list in coreutils/docs/commands.md), by reason: needs
// exec, unix-machinery-with-no-cross-platform-meaning, identity dup'd by
// whoami, or low-value/legacy/dangerous. Absent-and-not-deliberate ⇒ a real
// (planned) coverage gap.
var deliberate = map[string]bool{
	// requires executing another program (no-shell-out)
	"nohup": true, "nice": true, "stdbuf": true, "chroot": true, "kill": true,
	// unix machinery, no cross-platform meaning
	"mkfifo": true, "mknod": true, "stty": true, "chcon": true, "runcon": true, "hostid": true,
	// identity duplicated by whoami
	"who": true, "users": true, "pinky": true, "groups": true, "logname": true,
	// low value / legacy / dangerous
	"ptx": true, "factor": true, "pr": true, "fmt": true, "dircolors": true,
	"dir": true, "vdir": true, "sum": true, "pathchk": true, "shred": true,
}

// Status classifies a program against the reimpl.
type Status string

const (
	StatusPresent    Status = "present"            // registered in the tool registry
	StatusPlanned    Status = "missing-planned"    // absent, not deliberate → a real gap
	StatusDeliberate Status = "missing-deliberate" // absent, on the NO list
)

// Present reports whether name is a registered tool. Requires the host binary
// to have blank-imported the cmds (bashy and the perfbench dev multicall do).
func Present(name string) bool { return tool.Lookup(name) != nil }

// StatusOf classifies one program.
func StatusOf(p Program) Status {
	switch {
	case Present(p.Name):
		return StatusPresent
	case deliberate[p.Name]:
		return StatusDeliberate
	default:
		return StatusPlanned
	}
}

// ByGroup returns the inventory partitioned into the three groups, order stable.
func ByGroup() map[Group][]Program {
	out := map[Group][]Program{}
	for _, p := range Inventory {
		out[p.Group] = append(out[p.Group], p)
	}
	return out
}
