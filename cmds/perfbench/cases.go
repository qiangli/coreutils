package perfbenchcmd

// Authored fidelity case matrix for the EMPIRICAL frequency head — the commands
// real terminal-agent benchmarks (Terminal-Bench + TB-2.0, first-hand scan)
// actually exercise: the coreutils text/file core (cat wc head tail sort uniq
// cut tr) plus the agentic-extension filters (grep sed awk) and a few
// fs-mutator error cases. `find` is DEMOTED (a synthetic-NL2Bash artifact, ~0.5%
// in real agent tasks) so it gets a light touch here.
//
// Each case is run against both arms and diffed byte-for-byte (stdout + stderr +
// exit). Fixtures are the small deterministic files gen.go writes under
// corpus/conf/. Paths are relative — both arms share cwd (rc.Dir), so the path
// string in any output is identical across arms.
//
// GNU reference package per command (all installed into one $GNU_PREFIX):
//   coreutils: cat wc head tail sort uniq cut tr  ·  grep→grep  sed→sed  awk→gawk
//
// This is a FIRST matrix (byte-exact filters + error wording). Flag coverage is
// representative, not exhaustive; grow it as DIFFs are triaged.

const (
	fText   = "corpus/conf/text.txt"
	fNums   = "corpus/conf/nums.txt"
	fFields = "corpus/conf/fields.tsv"
	fDups   = "corpus/conf/dups.txt"
	fCommA  = "corpus/conf/comm_a.txt"
	fCommB  = "corpus/conf/comm_b.txt"
	fJoinA  = "corpus/conf/join_a.txt"
	fJoinB  = "corpus/conf/join_b.txt"
	fDag    = "corpus/conf/dag.txt"
	fLink   = "corpus/conf/link_to_text"
)

// authoredCases returns the head-command fidelity matrix.
func authoredCases() []Case {
	c := func(cmd string, argv ...string) Case { return Case{Cmd: cmd, Argv: argv} }
	stdin := func(cmd string, in string, argv ...string) Case {
		return Case{Cmd: cmd, Argv: argv, Stdin: []byte(in)}
	}

	var cs []Case
	cs = append(cs,
		// ---- cat ----
		c("cat", fText),
		c("cat", "-n", fText),
		c("cat", "-b", fText),
		c("cat", "-A", fText),
		c("cat", "-E", fText),
		c("cat", "-T", fText),
		c("cat", "-s", fText),
		c("cat", "-v", fText),
		c("cat", "-ns", fText),
		c("cat", fText, fNums),           // concatenate two files
		c("cat", "corpus/conf/nope.txt"), // error: no such file (stderr+exit wording)

		// ---- wc ----
		c("wc", fText),
		c("wc", "-l", fText),
		c("wc", "-w", fText),
		c("wc", "-c", fText),
		c("wc", "-m", fText),
		c("wc", "-L", fText),
		c("wc", "-lw", fText),
		c("wc", fText, fNums), // multi-file + total row

		// ---- head ----
		c("head", fText),
		c("head", "-n", "3", fText),
		c("head", "-n", "-2", fText), // all but last 2
		c("head", "-c", "12", fText),
		c("head", "-3", fText), // -NUM shorthand
		c("head", "-q", fText, fNums),

		// ---- tail ----
		c("tail", fText),
		c("tail", "-n", "3", fText),
		c("tail", "-n", "+3", fText), // from line 3
		c("tail", "-c", "10", fText),

		// ---- sort ----
		c("sort", fText),
		c("sort", "-r", fText),
		c("sort", "-f", fText), // fold case
		c("sort", fNums),       // lexical
		c("sort", "-n", fNums), // numeric (incl the -7)
		c("sort", "-rn", fNums),
		c("sort", "-u", fDups),
		c("sort", "-k2", fFields),
		c("sort", "-t", "\t", "-k1", "-n", fFields),
		c("sort", "-c", fNums), // check unsorted → exit 1 + "disorder" msg

		// ---- uniq ----
		c("uniq", fDups),
		c("uniq", "-c", fDups),
		c("uniq", "-d", fDups),
		c("uniq", "-u", fDups),

		// ---- cut ----
		c("cut", "-f1", fFields), // default tab delimiter
		c("cut", "-f2", fFields),
		c("cut", "-f1,3", fFields),
		c("cut", "-f2-", fFields),
		c("cut", "-d", " ", "-f1", fText),
		c("cut", "-c1-5", fText),
		c("cut", "--complement", "-f2", fFields),

		// ---- tr (stdin) ----
		stdin("tr", "hello world\n", "a-z", "A-Z"),
		stdin("tr", "hello world\n", "-d", "aeiou"),
		stdin("tr", "a   b   c\n", "-s", " "),
		stdin("tr", "abc123\n", "-c", "0-9", "."),
		stdin("tr", "hello\n", "-d", "l"),

		// ---- grep (grep 3.11) ----
		c("grep", "apple", fText),
		c("grep", "-i", "BANANA", fText),
		c("grep", "-v", "apple", fText),
		c("grep", "-n", "apple", fText),
		c("grep", "-c", "apple", fText),
		c("grep", "-w", "here", fText),
		c("grep", "-E", "[0-9]+", fText),
		c("grep", "-o", "[0-9][0-9]*", fText),
		c("grep", "-l", "apple", fText),
		c("grep", "zzz-nomatch", fText), // exit 1, no output

		// ---- sed (sed 4.9) ----
		c("sed", "s/apple/APPLE/", fText),
		c("sed", "s/a/X/g", fText),
		c("sed", "-n", "2p", fText),
		c("sed", "-n", "1,3p", fText),
		c("sed", "2d", fText),
		c("sed", "-E", "s/[0-9]+/N/g", fText),
		c("sed", "y/abc/ABC/", fText),

		// ---- awk (gawk 5.3.1) ----
		c("awk", "{print $1}", fFields),
		c("awk", "-F", "\t", "{print $2}", fFields),
		c("awk", "NR==2", fFields),
		c("awk", "{s+=$1} END{print s}", fFields),
		c("awk", "{print NF}", fText),

		// ================= the rest of the PRESENT coreutils set =================
		// (grep/sed/awk above are agentic extensions; find/tar are separate GNU pkgs)

		// ---- textutils: combine / sorted / checksums / encoding ----
		c("comm", fCommA, fCommB),
		c("comm", "-12", fCommA, fCommB),
		c("join", fJoinA, fJoinB),
		c("join", "-a", "1", fJoinA, fJoinB),
		c("paste", fCommA, fCommB),
		c("paste", "-d", ",", fCommA, fCommB),
		c("paste", "-s", fNums),
		c("tac", fText),
		c("tac", fNums),
		c("tsort", fDag),
		c("md5sum", fText),
		c("sha1sum", fText),
		c("sha256sum", fText),
		c("sha512sum", fText),
		c("sha224sum", fNums),
		c("sha384sum", fNums),
		c("base64", fNums),
		c("base32", fNums),
		stdin("base64", "hello world\n", "-d"), // decode invalid → error path (deterministic)
		// shuf: only the deterministic forms (a permutation would need a shared PRNG)
		c("shuf", "-i", "1-1"),
		c("shuf", "-e", "solo"),
		c("shuf", "-n", "0", fNums),

		// ---- sh-utils: pure string / numeric / env / system-info (same machine ⇒ same output) ----
		c("basename", "/a/b/c.txt"),
		c("basename", "/a/b/c.txt", ".txt"),
		c("basename", "-s", ".txt", "/a/b/c.txt"),
		c("dirname", "/a/b/c"),
		c("dirname", "-z", "/a/b/c"),
		c("echo", "hello", "world"),
		c("echo", "-n", "x"),
		c("echo", "-e", "a\\tb"),
		c("seq", "5"),
		c("seq", "2", "2", "10"),
		c("seq", "-w", "8", "10"),
		c("seq", "-s", ",", "1", "3"),
		c("true"),
		c("false"),
		c("sleep", "0"),
		c("sync"),
		c("pwd"),
		c("pwd", "-P"),
		c("printenv", "PATH"),
		c("env"), // dumps the (shared) invocation env in slice order ⇒ identical both arms
		c("whoami"),
		c("id"),
		c("uname"),
		c("uname", "-s"),
		c("uname", "-m"),
		// NOTE: `uname -a` and `hostname` are intentionally omitted — their output
		// embeds the real machine identity (never commit it). A separate finding:
		// bashy `uname -a` omits the -p (processor) field GNU prints on darwin.
		// `hostname` is not shipped by GNU coreutils by default (reference gap).
		c("tty"), // stdin not a tty ⇒ "not a tty", exit 1
		c("readlink", fLink),
		c("readlink", "-f", fLink),
		c("realpath", fLink),
		c("stat", "-c", "%s", fText),
		c("stat", "-c", "%n %s", fText),
		c("du", "-b", fText),
		c("du", "-sb", "corpus/conf"),
		c("timeout", "5", "true"), // completes ⇒ exit 0, no output
		// date: fixed epoch only (never the current clock). If -d @epoch is unsupported
		// bashy loudly declines ⇒ loud-skip (informative), not a diff.
		c("date", "-u", "-d", "@1000000000", "+%Y-%m-%dT%H:%M:%S"),

		// ---- fs-mutator ERROR cases (empty stdout on success; test stderr wording + exit) ----
		c("rm", "corpus/conf/nope.txt"),                         // exit 1, "cannot remove"
		c("ls", "corpus/conf/nope.txt"),                         // exit 2, "cannot access"
		c("cp", "corpus/conf/nope.txt", "corpus/conf/dest.txt"), // exit 1
		c("mv", "corpus/conf/nope.txt", "corpus/conf/dest2.txt"),
		c("mkdir", fText), // exists as a file ⇒ "File exists"
		c("rmdir", "corpus/conf/nope"),
		c("touch", "corpus/conf/nope_dir/x"), // missing parent
		c("chmod", "644", "corpus/conf/nope.txt"),
		c("ln", fText, fText), // hard link over an existing file ⇒ "File exists"
		c("link", "corpus/conf/nope.txt", "corpus/conf/l2"),
		c("unlink", "corpus/conf/nope.txt"),

		// NOTE deliberately NOT compared (non-deterministic / would hang):
		//   yes (infinite), df/uptime (fluctuating system state), mktemp (random name),
		//   shuf permutations (PRNG), split (writes side-effect files, empty stdout).
	)
	return cs
}
