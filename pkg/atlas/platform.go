// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package atlas

import "fmt"

// Platform vocabulary: the three operating systems bashy ships for. A
// command's OS list says where it is SUPPORTED; its Partial list names the
// supported OSes where it runs with a documented gap (a flag or mode that
// errors "not supported on windows"). Portable = full support on all three.
//
// Curated, never inferred — every cmds/ package compiles on every GOOS, so
// build tags prove nothing; the truth is in the *_other.go / *_windows.go
// stubs, and each entry below cites the stub it was read from. Windows is
// the OS where the lists must be believed rather than checked from a mac
// (dhnt docs/windows-crossplatform-uniformity.md), so a change here wants a
// run on a real Windows host, not a green cross-compile.
const (
	OSWindows = "windows"
	OSDarwin  = "darwin"
	OSLinux   = "linux"
)

// OSes returns the platform vocabulary, sorted.
func OSes() []string { return []string{OSDarwin, OSLinux, OSWindows} }

var allOS = []string{OSDarwin, OSLinux, OSWindows}

// SupportedOn reports whether the entry is supported on os.
func (e Entry) SupportedOn(os string) bool {
	for _, o := range e.OS {
		if o == os {
			return true
		}
	}
	return false
}

// PartialOn reports whether the entry runs on os with a documented gap.
func (e Entry) PartialOn(os string) bool {
	for _, o := range e.Partial {
		if o == os {
			return true
		}
	}
	return false
}

// Portable reports full support on every platform: the command a script can
// use as-is on windows, macOS and linux.
func (e Entry) Portable() bool {
	return len(e.OS) == len(allOS) && len(e.Partial) == 0
}

// classifyPlatforms stamps OS and Partial on every entry: everything is
// supported everywhere unless a pass below says otherwise. Runs before the
// alias pass so aliases inherit.
func classifyPlatforms() {
	for n, e := range tools {
		e.OS = append([]string(nil), allOS...)
		tools[n] = e
	}
	for n, e := range verbs {
		e.OS = append([]string(nil), allOS...)
		verbs[n] = e
	}

	// --- unsupported: the whole command errors on the platform --------------
	// cmds/ps/process_other.go: "live process inspection is supported only on
	// Linux"; cmds/chcon/chcon_other.go: SELinux contexts.
	osOnly([]string{OSLinux}, "ps", "chcon")
	// !unix stubs that refuse the command outright: no POSIX uid/gid (chgrp
	// chown), no mode bits (chmod), no FIFO/special files (mkfifo mknod), no
	// scheduling priorities (nice renice), no crontab install (crontab), no
	// terminal messaging (talk), no system log sink (logger), no login name
	// probe (logname).
	osOnly([]string{OSDarwin, OSLinux},
		"chgrp", "chmod", "chown", "mkfifo", "mknod", "nice", "renice",
		"crontab", "talk", "logger", "logname")
	// The pinned POSIX providers are built from C upstream source on the host
	// (cmds/posixproviders: "… is not supported on windows"), as is their
	// provisioner.
	osOnly([]string{OSDarwin, OSLinux},
		"ar", "ctags", "ex", "localedef", "lp", "m4", "man", "nm", "strip", "vi",
		"posix-providers")
	// bashy's engines_windows.go: "bashy ollama: not supported in the Windows
	// engine build".
	osOnly([]string{OSDarwin, OSLinux}, "ollama")

	// --- partial: runs, with a documented gap on that platform --------------
	// Each name cites the stub whose error message names the gap.
	partialOn(OSWindows,
		"more",    // tty_windows.go: interactive terminal mode
		"stty",    // stty_windows.go: most settings
		"sync",    // sync_windows.go: whole-system sync (file operands work)
		"kill",    // signal_other.go: process groups
		"pax",     // fifo/preserve/link stubs: FIFOs, ownership, link times
		"find",    // times_windows.go: -ctime
		"xargs",   // xargs_tty_windows.go: -p (interactive)
		"pr",      // tty_windows.go: /dev/tty
		"stat",    // statfs_other.go: file-system fields
		"pathchk", // limits_other.go: PATH_MAX / NAME_MAX queries
		"touch",   // no_deref_other.go: --no-dereference on a symlink
		"cp",      // special_other.go / link_times_other.go: special files, link times
		"mv",      // special_other.go / owner_other.go: special files, ownership
		"at",      // access_other.go / umask_other.go: access checks, umask
		"batch",   // same as at
		"uptime",  // uptime_windows.go: reduced probe
		"tty",     // tty_windows.go: reduced pathname lookup
	)
	partialOn(OSDarwin,
		"renice", // prio_unix_libc.go: process-group and user adjustments
	)
}

func osOnly(oses []string, names ...string) {
	for _, n := range names {
		if e, ok := tools[n]; ok {
			e.OS = append([]string(nil), oses...)
			tools[n] = e
			continue
		}
		if e, ok := verbs[n]; ok {
			e.OS = append([]string(nil), oses...)
			verbs[n] = e
			continue
		}
		panic(fmt.Sprintf("atlas: platform pass names unknown command %q", n))
	}
}

func partialOn(os string, names ...string) {
	for _, n := range names {
		if e, ok := tools[n]; ok {
			if !e.SupportedOn(os) {
				panic(fmt.Sprintf("atlas: %q marked partial on %s but not supported there", n, os))
			}
			e.Partial = append(e.Partial, os)
			tools[n] = e
			continue
		}
		if e, ok := verbs[n]; ok {
			e.Partial = append(e.Partial, os)
			verbs[n] = e
			continue
		}
		panic(fmt.Sprintf("atlas: partial pass names unknown command %q", n))
	}
}
