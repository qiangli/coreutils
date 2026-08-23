package tputcmd

// The three capability-name tables below ARE the binary terminfo format: a
// compiled entry stores three unnamed arrays (booleans, numbers, strings) and
// nothing else, so a capability's name is purely its INDEX in these lists.
// Getting an index wrong does not produce an error — it produces a plausible
// value for the wrong capability — so the order is transcribed from the
// terminfo(5)/term(5) capability listing and cross-checked, index by index,
// against real compiled entries (see the TERMINFO-backed comparison in
// terminfo_db_test.go, which is skipped when no database is installed).
//
// The tail of each list ("OT…") holds the obsolete termcap-compatibility
// capabilities. Entries compiled by older tools simply declare fewer array
// members; the header counts, not these lengths, decide how much is read, so
// listing them costs nothing and lets a full-length entry resolve.

// boolNames is the boolean capability array order (44 members).
var boolNames = []string{
	"bw", "am", "xsb", "xhp", "xenl", "eo", "gn", "hc",
	"km", "hs", "in", "da", "db", "mir", "msgr", "os",
	"eslok", "xt", "hz", "ul", "xon", "nxon", "mc5i", "chts",
	"nrrmc", "npc", "ndscr", "ccc", "bce", "hls", "xhpa", "crxm",
	"daisy", "xvpa", "sam", "cpix", "lpix",
	"OTbs", "OTns", "OTnc", "OTMT", "OTNL", "OTpt", "OTxr",
}

// numNames is the numeric capability array order (39 members).
var numNames = []string{
	"cols", "it", "lines", "lm", "xmc", "pb", "vt", "wsl",
	"nlab", "lh", "lw", "ma", "wnum", "colors", "pairs", "ncv",
	"bufsz", "spinv", "spinh", "maddr", "mjump", "mcs", "mls", "npins",
	"orc", "orl", "orhi", "orvi", "cps", "widcs", "btns", "bitwin",
	"bitype",
	"OTug", "OTdC", "OTdN", "OTdB", "OTdT", "OTkn",
}

// strNames is the string capability array order (414 members).
var strNames = []string{
	// 0..
	"cbt", "bel", "cr", "csr", "tbc", "clear", "el", "ed",
	"hpa", "cmdch", "cup", "cud1", "home", "civis", "cub1", "mrcup",
	"cnorm", "cuf1", "ll", "cuu1", "cvvis", "dch1", "dl1", "dsl",
	"hd", "smacs", "blink", "bold", "smcup", "smdc", "dim", "smir",
	"invis", "prot", "rev", "smso", "smul", "ech", "rmacs", "sgr0",
	"rmcup", "rmdc", "rmir", "rmso", "rmul", "flash", "ff", "fsl",
	"is1", "is2", "is3", "if", "ich1", "il1", "ip", "kbs",
	"ktbc", "kclr", "kctab", "kdch1", "kdl1", "kcud1", "krmir", "kel",
	"ked", "kf0", "kf1", "kf10", "kf2", "kf3", "kf4", "kf5",
	"kf6", "kf7", "kf8", "kf9", "khome", "kich1", "kil1", "kcub1",
	"kll", "knp", "kpp", "kcuf1", "kind", "kri", "khts", "kcuu1",
	"rmkx", "smkx", "lf0", "lf1", "lf10", "lf2", "lf3", "lf4",
	"lf5", "lf6", "lf7", "lf8", "lf9", "rmm", "smm", "nel",
	"pad", "dch", "dl", "cud", "ich", "indn", "il", "cub",
	"cuf", "rin", "cuu", "pfkey", "pfloc", "pfx", "mc0", "mc4",
	"mc5", "rep", "rs1", "rs2", "rs3", "rf", "rc", "vpa",
	"sc", "ind", "ri", "sgr", "hts", "wind", "ht", "tsl",
	"uc", "hu", "iprog", "ka1", "ka3", "kb2", "kc1", "kc3",
	"mc5p", "rmp", "acsc", "pln", "kcbt", "smxon", "rmxon", "smam",
	"rmam", "xonc", "xoffc", "enacs", "smln", "rmln", "kbeg", "kcan",
	"kclo", "kcmd", "kcpy", "kcrt", "kend", "kent", "kext", "kfnd",
	"khlp", "kmrk", "kmsg", "kmov", "knxt", "kopn", "kopt", "kprv",
	"kprt", "krdo", "kref", "krfr", "krpl", "krst", "kres", "ksav",
	"kspd", "kund", "kBEG", "kCAN", "kCMD", "kCPY", "kCRT", "kDC",
	"kDL", "kslt", "kEND", "kEOL", "kEXT", "kFND", "kHLP", "kHOM",
	"kIC", "kLFT", "kMSG", "kMOV", "kNXT", "kOPT", "kPRV", "kPRT",
	"kRDO", "kRPL", "kRIT", "kRES", "kSAV", "kSPD", "kUND", "rfi",
	// 216: kf11..kf63
	"kf11", "kf12", "kf13", "kf14", "kf15", "kf16", "kf17", "kf18",
	"kf19", "kf20", "kf21", "kf22", "kf23", "kf24", "kf25", "kf26",
	"kf27", "kf28", "kf29", "kf30", "kf31", "kf32", "kf33", "kf34",
	"kf35", "kf36", "kf37", "kf38", "kf39", "kf40", "kf41", "kf42",
	"kf43", "kf44", "kf45", "kf46", "kf47", "kf48", "kf49", "kf50",
	"kf51", "kf52", "kf53", "kf54", "kf55", "kf56", "kf57", "kf58",
	"kf59", "kf60", "kf61", "kf62", "kf63",
	// 269:
	"el1", "mgc", "smgl", "smgr", "fln", "sclk", "dclk", "rmclk",
	"cwin", "wingo", "hup", "dial", "qdial", "tone", "pulse", "hook",
	"pause", "wait", "u0", "u1", "u2", "u3", "u4", "u5",
	"u6", "u7", "u8", "u9", "op", "oc", "initc", "initp",
	"scp", "setf", "setb", "cpi", "lpi", "chr", "cvr", "defc",
	"swidm", "sdrfq", "sitm", "slm", "smicm", "snlq", "snrmq", "sshm",
	"ssubm", "ssupm", "sum", "rwidm", "ritm", "rlm", "rmicm", "rshm",
	"rsubm", "rsupm", "rum", "mhpa", "mcud1", "mcub1", "mcuf1", "mvpa",
	"mcuu1", "porder", "mcud", "mcub", "mcuf", "mcuu", "scs", "smgb",
	"smgbp", "smglp", "smgrp", "smgt", "smgtp", "sbim", "scsd", "rbim",
	"rcsd", "subcs", "supcs", "docr", "zerom", "csnm", "kmous", "minfo",
	"reqmp", "getm", "setaf", "setab", "pfxl", "devt", "csin", "s0ds",
	"s1ds", "s2ds", "s3ds", "smglr", "smgtb", "birep", "binel", "bicr",
	"colornm", "defbi", "endbi", "setcolor", "slines", "dispc", "smpch", "rmpch",
	"smsc", "rmsc", "pctrm", "scesc", "scesa", "ehhlm", "elhlm", "elohlm",
	"erhlm", "ethlm", "evhlm", "sgr1", "slength",
	// 394: obsolete termcap-compatibility strings
	"OTi2", "OTrs", "OTnl", "OTbc", "OTko", "OTma", "OTG2", "OTG3",
	"OTG1", "OTG4", "OTGR", "OTGL", "OTGU", "OTGD", "OTGH", "OTGV",
	"OTGC", "meml", "memu", "box1",
}

// Expected array lengths, asserted by the tests. A capability appended in the
// wrong section would otherwise shift every index after it.
const (
	boolArrayLen = 44
	numArrayLen  = 39
	strArrayLen  = 414
)

// capKind says which of the three arrays a name belongs to. "Unknown" is a
// distinct answer from "absent": POSIX gives them different exit statuses
// (4 versus 1), so the caller must be able to tell them apart.
type capKind int

const (
	capUnknown capKind = iota
	capBool
	capNum
	capStr
)

var capKinds = map[string]capKind{}

func init() {
	for _, n := range boolNames {
		capKinds[n] = capBool
	}
	for _, n := range numNames {
		capKinds[n] = capNum
	}
	for _, n := range strNames {
		capKinds[n] = capStr
	}
}

// kindOf reports which array name lives in, or capUnknown.
func kindOf(name string) capKind { return capKinds[name] }
