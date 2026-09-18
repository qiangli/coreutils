module github.com/qiangli/coreutils

go 1.26.5

require (
	github.com/benhoyt/goawk v1.31.0
	github.com/creack/pty/v2 v2.0.1
	github.com/ebitengine/purego v0.10.1
	github.com/mattn/go-runewidth v0.0.23
	github.com/opencontainers/selinux v1.13.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
	github.com/shirou/gopsutil/v4 v4.26.5
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/tjfoc/gmsm v1.4.1
	github.com/tklauser/ps v0.0.5-0.20260804061010-39c4acb07b31
	golang.org/x/crypto v0.53.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.44.0
	golang.org/x/text v0.38.0
	lukechampine.com/blake3 v1.4.1
	mvdan.cc/sh/v3 v3.13.1
)

// Sibling-path replace: ../sh resolves to the sh submodule inside the dhnt
// umbrella, and to a flat sibling clone in a standalone checkout. Same
// convention as ycode/outpost/bashy.
replace mvdan.cc/sh/v3 => ../sh

// Local MIT fork adds POSIX awk float formats, locale-aware data and string
// semantics, an error-bearing regex backend across all surfaces, and the
// required side effect that a non-"in" reference creates an absent array
// element. Keeping the narrow fork here makes the conformance fixes build from
// this repository rather than an unpublished dependency commit.
replace github.com/benhoyt/goawk => ./third_party/goawk

require (
	cyphar.com/go-pathrs v0.2.4 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/mod v0.36.0 // indirect
)
