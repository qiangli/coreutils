// Package lean registers the lean userland: the standard Unix utilities every
// shell-driven helper invocation needs, WITHOUT the heavy agent extensions that
// bloat the binary and its per-invocation page footprint.
//
// It is a strict subset of [github.com/qiangli/coreutils/cmds/all]. Consumers
// that embed the whole userland import cmds/all; consumers that need a small,
// fast-starting helper binary (the /vsc/cushim helper path, a busybox-style
// multicall fronting cheap commands like true/expr/cmp/grep) import this
// package instead and get the same dispatch and fail-closed behavior at a
// fraction of the binary size.
//
// # Why this exists
//
// The full multicall binary (cmd/coreutils) links every command, including
// agent extensions that pull large dependency trees: browser (chromedp/cdproto),
// fetch (HTTP + markdown), jq (gojq), and the code-intelligence / orchestration
// verbs (ast, graph, tokens, resources, foreman). A single such applet can add
// 7-10 MB to the binary. When a shell script resolves every helper through one
// multicall binary, that binary is mmap'd and demand-paged on EVERY invocation
// — including the no-op `true` — so a 64 MB binary costs ~9 ms per cheap call
// where a 13 MB binary costs ~3.8 ms (measured; see the lean benchmark). Across
// hundreds of helper invocations that is minutes of wall time.
//
// The lean set keeps the page footprint small by excluding only the applets
// whose own documentation marks them as agent extensions or non-utility
// commands. Every standard coreutils / findutils / diffutils / greputils /
// POSIX utility stays, so a lean-built binary remains a drop-in for the helper
// path: anything it does not ship fails closed with exit 2 (the same diagnostic
// the full binary emits for an unknown command), never silently approximates.
//
// # Deliberately EXCLUDED (and why)
//
//	ast       tree-sitter code intelligence (agent extension, not a utility)
//	browser   headless browser automation (chromedp/cdproto — ~10 MB)
//	clip      clipboard (agent extension)
//	duration  agent-workflow duration conversion (agent extension)
//	fetch     HTTP fetch + markdown extraction (~7 MB; agent extension)
//	graph     code-graph / space-graph (agent extension)
//	jq        JSON query via gojq (agent extension; ~2 MB)
//	tokens    LLM token counting (agent extension)
//
// These eight are excluded from cmds/all's inventory. The full cmd/coreutils
// binary is unchanged and still ships them.
//
// Keep the import list alphabetical; like cmds/all, this list IS the shipped
// inventory. The companion test (lean_test.go) asserts it is exactly the
// documented set, so a future addition that forgets to decide lean-vs-full is
// caught.
package lean

import (
	_ "github.com/qiangli/coreutils/cmds/arch"
	_ "github.com/qiangli/coreutils/cmds/at"
	_ "github.com/qiangli/coreutils/cmds/atq"
	_ "github.com/qiangli/coreutils/cmds/atrm"
	_ "github.com/qiangli/coreutils/cmds/awk"
	_ "github.com/qiangli/coreutils/cmds/b2sum"
	_ "github.com/qiangli/coreutils/cmds/base32"
	_ "github.com/qiangli/coreutils/cmds/base64"
	_ "github.com/qiangli/coreutils/cmds/basename"
	_ "github.com/qiangli/coreutils/cmds/basenc"
	_ "github.com/qiangli/coreutils/cmds/batch"
	_ "github.com/qiangli/coreutils/cmds/cal"
	_ "github.com/qiangli/coreutils/cmds/cat"
	_ "github.com/qiangli/coreutils/cmds/chcon"
	_ "github.com/qiangli/coreutils/cmds/chgrp"
	_ "github.com/qiangli/coreutils/cmds/chmod"
	_ "github.com/qiangli/coreutils/cmds/chown"
	_ "github.com/qiangli/coreutils/cmds/chroot"
	_ "github.com/qiangli/coreutils/cmds/cksum"
	_ "github.com/qiangli/coreutils/cmds/cmp"
	_ "github.com/qiangli/coreutils/cmds/comm"
	_ "github.com/qiangli/coreutils/cmds/cp"
	_ "github.com/qiangli/coreutils/cmds/crontab"
	_ "github.com/qiangli/coreutils/cmds/csplit"
	_ "github.com/qiangli/coreutils/cmds/cut"
	_ "github.com/qiangli/coreutils/cmds/date"
	_ "github.com/qiangli/coreutils/cmds/dd"
	_ "github.com/qiangli/coreutils/cmds/df"
	_ "github.com/qiangli/coreutils/cmds/diff"
	_ "github.com/qiangli/coreutils/cmds/dir"
	_ "github.com/qiangli/coreutils/cmds/dircolors"
	_ "github.com/qiangli/coreutils/cmds/dirname"
	_ "github.com/qiangli/coreutils/cmds/du"
	_ "github.com/qiangli/coreutils/cmds/echo"
	_ "github.com/qiangli/coreutils/cmds/env"
	_ "github.com/qiangli/coreutils/cmds/expand"
	_ "github.com/qiangli/coreutils/cmds/expr"
	_ "github.com/qiangli/coreutils/cmds/factor"
	_ "github.com/qiangli/coreutils/cmds/false"
	_ "github.com/qiangli/coreutils/cmds/file"
	_ "github.com/qiangli/coreutils/cmds/find"
	_ "github.com/qiangli/coreutils/cmds/fmt"
	_ "github.com/qiangli/coreutils/cmds/fold"
	_ "github.com/qiangli/coreutils/cmds/grep"
	_ "github.com/qiangli/coreutils/cmds/groups"
	_ "github.com/qiangli/coreutils/cmds/gzip"
	_ "github.com/qiangli/coreutils/cmds/head"
	_ "github.com/qiangli/coreutils/cmds/hexdump"
	_ "github.com/qiangli/coreutils/cmds/hostid"
	_ "github.com/qiangli/coreutils/cmds/hostname"
	_ "github.com/qiangli/coreutils/cmds/iconv"
	_ "github.com/qiangli/coreutils/cmds/id"
	_ "github.com/qiangli/coreutils/cmds/install"
	_ "github.com/qiangli/coreutils/cmds/join"
	_ "github.com/qiangli/coreutils/cmds/kill"
	_ "github.com/qiangli/coreutils/cmds/link"
	_ "github.com/qiangli/coreutils/cmds/ln"
	_ "github.com/qiangli/coreutils/cmds/logname"
	_ "github.com/qiangli/coreutils/cmds/ls"
	_ "github.com/qiangli/coreutils/cmds/md5sum"
	_ "github.com/qiangli/coreutils/cmds/mkdir"
	_ "github.com/qiangli/coreutils/cmds/mkfifo"
	_ "github.com/qiangli/coreutils/cmds/mknod"
	_ "github.com/qiangli/coreutils/cmds/mktemp"
	_ "github.com/qiangli/coreutils/cmds/more"
	_ "github.com/qiangli/coreutils/cmds/mv"
	_ "github.com/qiangli/coreutils/cmds/nice"
	_ "github.com/qiangli/coreutils/cmds/nl"
	_ "github.com/qiangli/coreutils/cmds/nohup"
	_ "github.com/qiangli/coreutils/cmds/nproc"
	_ "github.com/qiangli/coreutils/cmds/ntp"
	_ "github.com/qiangli/coreutils/cmds/numfmt"
	_ "github.com/qiangli/coreutils/cmds/od"
	_ "github.com/qiangli/coreutils/cmds/paste"
	_ "github.com/qiangli/coreutils/cmds/pathchk"
	_ "github.com/qiangli/coreutils/cmds/pinky"
	_ "github.com/qiangli/coreutils/cmds/pr"
	_ "github.com/qiangli/coreutils/cmds/printenv"
	_ "github.com/qiangli/coreutils/cmds/printf"
	_ "github.com/qiangli/coreutils/cmds/ps"
	_ "github.com/qiangli/coreutils/cmds/ptx"
	_ "github.com/qiangli/coreutils/cmds/pwd"
	_ "github.com/qiangli/coreutils/cmds/readlink"
	_ "github.com/qiangli/coreutils/cmds/realpath"
	_ "github.com/qiangli/coreutils/cmds/rm"
	_ "github.com/qiangli/coreutils/cmds/rmdir"
	_ "github.com/qiangli/coreutils/cmds/runcon"
	_ "github.com/qiangli/coreutils/cmds/sed"
	_ "github.com/qiangli/coreutils/cmds/seq"
	_ "github.com/qiangli/coreutils/cmds/sha1sum"
	_ "github.com/qiangli/coreutils/cmds/sha224sum"
	_ "github.com/qiangli/coreutils/cmds/sha256sum"
	_ "github.com/qiangli/coreutils/cmds/sha384sum"
	_ "github.com/qiangli/coreutils/cmds/sha512sum"
	_ "github.com/qiangli/coreutils/cmds/shred"
	_ "github.com/qiangli/coreutils/cmds/shuf"
	_ "github.com/qiangli/coreutils/cmds/sleep"
	_ "github.com/qiangli/coreutils/cmds/sort"
	_ "github.com/qiangli/coreutils/cmds/split"
	_ "github.com/qiangli/coreutils/cmds/stat"
	_ "github.com/qiangli/coreutils/cmds/stdbuf"
	_ "github.com/qiangli/coreutils/cmds/strings"
	_ "github.com/qiangli/coreutils/cmds/stty"
	_ "github.com/qiangli/coreutils/cmds/sum"
	_ "github.com/qiangli/coreutils/cmds/sync"
	_ "github.com/qiangli/coreutils/cmds/tac"
	_ "github.com/qiangli/coreutils/cmds/tail"
	_ "github.com/qiangli/coreutils/cmds/tar"
	_ "github.com/qiangli/coreutils/cmds/tee"
	// registers both `test` and its `[` spelling, as upstream does
	_ "github.com/qiangli/coreutils/cmds/test"
	_ "github.com/qiangli/coreutils/cmds/time"
	_ "github.com/qiangli/coreutils/cmds/timeout"
	_ "github.com/qiangli/coreutils/cmds/touch"
	_ "github.com/qiangli/coreutils/cmds/tr"
	_ "github.com/qiangli/coreutils/cmds/tree"
	_ "github.com/qiangli/coreutils/cmds/true"
	_ "github.com/qiangli/coreutils/cmds/truncate"
	_ "github.com/qiangli/coreutils/cmds/tsort"
	_ "github.com/qiangli/coreutils/cmds/tty"
	_ "github.com/qiangli/coreutils/cmds/tz"
	_ "github.com/qiangli/coreutils/cmds/uname"
	_ "github.com/qiangli/coreutils/cmds/unexpand"
	_ "github.com/qiangli/coreutils/cmds/uniq"
	_ "github.com/qiangli/coreutils/cmds/unlink"
	_ "github.com/qiangli/coreutils/cmds/uptime"
	_ "github.com/qiangli/coreutils/cmds/users"
	_ "github.com/qiangli/coreutils/cmds/uudecode"
	_ "github.com/qiangli/coreutils/cmds/uuencode"
	_ "github.com/qiangli/coreutils/cmds/vdir"
	_ "github.com/qiangli/coreutils/cmds/watch"
	_ "github.com/qiangli/coreutils/cmds/wc"
	_ "github.com/qiangli/coreutils/cmds/which"
	_ "github.com/qiangli/coreutils/cmds/who"
	_ "github.com/qiangli/coreutils/cmds/whoami"
	_ "github.com/qiangli/coreutils/cmds/xargs"
	_ "github.com/qiangli/coreutils/cmds/yes"
)
