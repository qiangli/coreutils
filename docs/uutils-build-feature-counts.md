# uutils build feature command counts

Generated on 2026-07-07 from `reference/uutils-coreutils`.

uutils does not expose the same command inventory for every build. The default
build uses the `feat_common_core` feature set, while the Unix build enables
additional platform-specific utilities through the `unix` / `feat_os_unix`
feature path.

## Counts

| Build command | `coreutils --list` count | Notes |
|---|---:|---|
| `cargo build --release` | 79 | Default `feat_common_core` set. |
| `cargo build --release --features unix` | 107 | Adds Tier-1 and Unix-specific utilities. |
| `cargo build --release --features unix,feat_selinux` on macOS | 107 | Builds SELinux crates, but `build.rs` excludes `chcon` and `runcon` from the multicall map on non-Linux/non-Android targets. |

The source tree has 109 directories under `src/uu`; `checksum_common` is a
shared helper crate, not a command. That leaves 108 utility command crates.
The built multicall also exposes `[` as an alias for `test`, so a Linux/Android
SELinux-capable build can list 109 command names: 108 utility crates plus the
`[` alias.

## Commands Added by `--features unix`

Compared with the default build, `cargo build --release --features unix` adds:

`arch`, `chgrp`, `chmod`, `chown`, `chroot`, `groups`, `hostid`, `hostname`,
`id`, `install`, `kill`, `logname`, `mkfifo`, `mknod`, `nice`, `nohup`,
`nproc`, `pinky`, `stat`, `stdbuf`, `stty`, `sync`, `timeout`, `uname`,
`uptime`, `users`, `who`, `whoami`.

## SELinux Commands

`chcon` and `runcon` are behind the `feat_selinux` feature and are also
platform-gated by uutils' `build.rs`:

```rust
#[cfg(not(any(target_os = "linux", target_os = "android")))]
"chcon" | "runcon" => {
    continue;
}
```

On macOS, enabling `feat_selinux` compiles the relevant crates but does not add
`chcon` or `runcon` to the multicall command map. On Linux/Android with
SELinux-capable dependencies, those two commands should be included.

## GNU Command-Name Comparison

GNU coreutils' `reference/gnu-coreutils/README` lists 109 command names.
uutils' source tree also resolves to 109 command names if `checksum_common` is
treated as a helper crate and `[` is counted as the `test` alias.

The source-level command-name sets are not byte-for-byte identical:

| Side | Different command name |
|---|---|
| GNU only | `coreutils` |
| uutils only | `more` |

All other source-level command names match.

For the macOS `cargo build --release --features unix` uutils multicall used in
local comparisons:

| Comparison | Command names |
|---|---|
| GNU not in this uutils build | `chcon`, `coreutils`, `runcon` |
| uutils build not in GNU | `more` |

`chcon` and `runcon` are present in uutils source, but excluded from the macOS
multicall by platform gating. `coreutils` is GNU's multicall command name;
uutils' binary is named `coreutils`, but it does not list `coreutils` as a
subcommand.
