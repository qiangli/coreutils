package terminfo

import "strings"

// A small compiled-in database.
//
// It exists for two situations, and for nothing else. First, a host with no
// terminfo database at all — a scratch container, or Windows, where the tool
// must still compile and still answer for the handful of terminals anyone
// would name. Second, the tests: a unit test that consults /usr/share/terminfo
// passes or fails according to which ncurses the developer happens to have
// installed, which is not a test.
//
// The on-disk database always wins (see Load). These entries are a
// fallback, never an override, so an administrator's customised entry is never
// silently shadowed by a value compiled into this binary.
//
// Values are the standard published control sequences for these terminals
// (ECMA-48 / the DEC VT100 manual / the xterm control-sequence reference).
// Padding delays are deliberately absent: this table is a floor, not a
// reproduction of a vendor database.

type builtinDef struct {
	names []string
	bools []string
	nums  map[string]int
	strs  map[string]string
}

// ansiCommon is the ECMA-48 core shared by every entry below except dumb.
var ansiCommon = map[string]string{
	"bel":   "\a",
	"cr":    "\r",
	"tbc":   "\x1b[3g",
	"clear": "\x1b[H\x1b[2J",
	"el":    "\x1b[K",
	"el1":   "\x1b[1K",
	"ed":    "\x1b[J",
	"cup":   "\x1b[%i%p1%d;%p2%dH",
	"home":  "\x1b[H",
	"hpa":   "\x1b[%i%p1%dG",
	"vpa":   "\x1b[%i%p1%dd",
	"cub1":  "\b",
	"cuf1":  "\x1b[C",
	"cuu1":  "\x1b[A",
	"cud1":  "\n",
	"cub":   "\x1b[%p1%dD",
	"cuf":   "\x1b[%p1%dC",
	"cuu":   "\x1b[%p1%dA",
	"cud":   "\x1b[%p1%dB",
	"csr":   "\x1b[%i%p1%d;%p2%dr",
	"sc":    "\x1b7",
	"rc":    "\x1b8",
	"ind":   "\n",
	"ri":    "\x1bM",
	"nel":   "\r\n",
	"ht":    "\t",
	"cbt":   "\x1b[Z",
	"dch1":  "\x1b[P",
	"dch":   "\x1b[%p1%dP",
	"ich1":  "\x1b[@",
	"ich":   "\x1b[%p1%d@",
	"il1":   "\x1b[L",
	"il":    "\x1b[%p1%dL",
	"dl1":   "\x1b[M",
	"dl":    "\x1b[%p1%dM",
	"ech":   "\x1b[%p1%dX",
	"indn":  "\x1b[%p1%dS",
	"rin":   "\x1b[%p1%dT",
	"bold":  "\x1b[1m",
	"dim":   "\x1b[2m",
	"blink": "\x1b[5m",
	"rev":   "\x1b[7m",
	"invis": "\x1b[8m",
	"smso":  "\x1b[7m",
	"rmso":  "\x1b[27m",
	"smul":  "\x1b[4m",
	"rmul":  "\x1b[24m",
	"sgr0":  "\x1b[m",
	"op":    "\x1b[39;49m",
	"civis": "\x1b[?25l",
	"cnorm": "\x1b[?25h",
	"kcuu1": "\x1bOA",
	"kcud1": "\x1bOB",
	"kcuf1": "\x1bOC",
	"kcub1": "\x1bOD",
	"khome": "\x1bOH",
	"kend":  "\x1bOF",
	"kbs":   "\x7f",
	"kdch1": "\x1b[3~",
	"kich1": "\x1b[2~",
	"knp":   "\x1b[6~",
	"kpp":   "\x1b[5~",
	"flash": "\x1b[?5h$<100/>\x1b[?5l",
}

// setaf/setab as xterm publishes them: the first eight colours use the
// ECMA-48 3x/4x codes, the next eight the aixterm 9x/10x codes, and anything
// above uses the 256-colour extension. It is also the single best exercise of
// the %-directive engine in any real entry.
const (
	xtermSetaf = "\x1b[%?%p1%{8}%<%t3%p1%d%e%p1%{16}%<%t9%p1%{8}%-%d%e38;5;%p1%d%;m"
	xtermSetab = "\x1b[%?%p1%{8}%<%t4%p1%d%e%p1%{16}%<%t10%p1%{8}%-%d%e48;5;%p1%d%;m"
)

var builtins = []builtinDef{
	{
		names: []string{"dumb", "80-column dumb tty"},
		bools: []string{"am"},
		nums:  map[string]int{"cols": 80},
		strs: map[string]string{
			"bel":  "\a",
			"cr":   "\r",
			"cud1": "\n",
			"ind":  "\n",
		},
	},
	{
		names: []string{"vt100", "vt100-am", "dec vt100 (w/advanced video)"},
		bools: []string{"am", "msgr", "xenl", "xon"},
		nums:  map[string]int{"cols": 80, "lines": 24, "it": 8, "vt": 3},
		strs: mergeCaps(ansiCommon, map[string]string{
			"clear": "\x1b[H\x1b[J",
			"smkx":  "\x1b[?1h\x1b=",
			"rmkx":  "\x1b[?1l\x1b>",
			"is2":   "\x1b[?3l\x1b[?4l\x1b[?5l\x1b[?7h\x1b[?8h",
			"rs2":   "\x1b>\x1b[?3l\x1b[?4l\x1b[?5l\x1b[?7h\x1b[?8h",
			"sgr0":  "\x1b[m\x0f",
			"rmso":  "\x1b[m",
			"rmul":  "\x1b[m",
			"kf1":   "\x1bOP",
			"kf2":   "\x1bOQ",
			"kf3":   "\x1bOR",
			"kf4":   "\x1bOS",
		}),
	},
	{
		names: []string{"ansi", "ansi/pc-term compatible with color"},
		bools: []string{"am", "bce", "mir", "msgr", "xon"},
		nums:  map[string]int{"cols": 80, "lines": 24, "it": 8, "colors": 8, "pairs": 64},
		strs: mergeCaps(ansiCommon, map[string]string{
			"setaf": "\x1b[3%p1%dm",
			"setab": "\x1b[4%p1%dm",
			"setf":  "\x1b[3%?%p1%{1}%=%t4%e%p1%{3}%=%t6%e%p1%{4}%=%t1%e%p1%{6}%=%t3%e%p1%d%;m",
			"setb":  "\x1b[4%?%p1%{1}%=%t4%e%p1%{3}%=%t6%e%p1%{4}%=%t1%e%p1%{6}%=%t3%e%p1%d%;m",
		}),
	},
	{
		names: []string{"xterm", "xterm-color", "xterm terminal emulator (X Window System)"},
		bools: []string{"am", "bce", "km", "mir", "msgr", "npc", "xenl"},
		nums:  map[string]int{"cols": 80, "lines": 24, "it": 8, "colors": 8, "pairs": 64},
		strs: mergeCaps(ansiCommon, map[string]string{
			"setaf": xtermSetaf,
			"setab": xtermSetab,
			"sgr0":  "\x1b(B\x1b[m",
			"smcup": "\x1b[?1049h",
			"rmcup": "\x1b[?1049l",
			"smkx":  "\x1b[?1h\x1b=",
			"rmkx":  "\x1b[?1l\x1b>",
			"is2":   "\x1b[!p\x1b[?3;4l\x1b[4l\x1b>",
			"rs1":   "\x1bc",
			"rs2":   "\x1b[!p\x1b[?3;4l\x1b[4l\x1b>",
			"kf1":   "\x1bOP",
			"kf2":   "\x1bOQ",
			"kf3":   "\x1bOR",
			"kf4":   "\x1bOS",
			"kf5":   "\x1b[15~",
			"kf6":   "\x1b[17~",
			"kf7":   "\x1b[18~",
			"kf8":   "\x1b[19~",
			"kf9":   "\x1b[20~",
			"kf10":  "\x1b[21~",
			"kf11":  "\x1b[23~",
			"kf12":  "\x1b[24~",
			"kmous": "\x1b[M",
		}),
	},
	{
		names: []string{"xterm-256color", "xterm with 256 colors"},
		bools: []string{"am", "bce", "km", "mir", "msgr", "npc", "xenl"},
		nums:  map[string]int{"cols": 80, "lines": 24, "it": 8, "colors": 256, "pairs": 32767},
		strs: mergeCaps(ansiCommon, map[string]string{
			"setaf": xtermSetaf,
			"setab": xtermSetab,
			"sgr0":  "\x1b(B\x1b[m",
			"smcup": "\x1b[?1049h",
			"rmcup": "\x1b[?1049l",
			"smkx":  "\x1b[?1h\x1b=",
			"rmkx":  "\x1b[?1l\x1b>",
			"is2":   "\x1b[!p\x1b[?3;4l\x1b[4l\x1b>",
			"rs1":   "\x1bc",
			"rs2":   "\x1b[!p\x1b[?3;4l\x1b[4l\x1b>",
			"kf1":   "\x1bOP",
			"kf2":   "\x1bOQ",
			"kf3":   "\x1bOR",
			"kf4":   "\x1bOS",
			"kmous": "\x1b[M",
		}),
	},
}

func mergeCaps(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// builtinEntry returns the compiled-in description for term, or nil.
// Every alias in an entry's names list matches, exactly as it would in a
// compiled file.
func builtinEntry(term string) *Entry {
	for _, def := range builtins {
		// The last name is the long description, not an alias.
		aliases := def.names
		if len(aliases) > 1 {
			aliases = aliases[:len(aliases)-1]
		}
		for _, alias := range aliases {
			if alias != term {
				continue
			}
			e := newEntry()
			e.names = append([]string(nil), def.names...)
			e.source = "(built-in)"
			for _, b := range def.bools {
				e.bools[b] = true
			}
			for k, v := range def.nums {
				e.nums[k] = v
			}
			for k, v := range def.strs {
				e.strs[k] = v
			}
			return e
		}
	}
	return nil
}

// BuiltinNames lists the terminal types the compiled-in table answers for,
// for the --help text.
func BuiltinNames() string {
	var names []string
	for _, def := range builtins {
		aliases := def.names
		if len(aliases) > 1 {
			aliases = aliases[:len(aliases)-1]
		}
		names = append(names, aliases...)
	}
	return strings.Join(names, ", ")
}
