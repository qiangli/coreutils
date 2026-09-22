# Windows host locale service contract

Sprint: #253

The Windows `locale` applet can use a host-installed POSIX locale service
without shipping locale data. Bashy's Windows fixture launcher must provide:

- `BASHY_HOST_LOCALE`: an absolute native Windows path to the host-owned
  `locale` executable (for example Git Bash's installed executable). It must
  not resolve through `PATH`; the fixture's `locale` command can be coreutils
  itself.
- `BASHY_HOST_LOCALE_NAMES` (optional): semicolon- or newline-separated locale
  names provisioned into that host service but omitted by its `locale -a`.

Coreutils first obtains generic candidates from `BASHY_HOST_LOCALE -a`, then
adds the optional provisioned candidates. It advertises a candidate only when
the host confirms that `LC_ALL=<name>` remains selected, every POSIX category
answers `locale -k <category>`, and `locale -k charmap` returns a non-empty
charmap. This rejects Git Bash's `zh_HK.big5hkscs` C fallback. Data returned by
`locale -k` is delegated to that executable at query time.

The fixture provisioner owns installation of every corpus locale it places in
`BASHY_HOST_LOCALE_NAMES`; coreutils neither downloads nor embeds locale
archives or names. This is the handoff contract for the bashy#11 worker.
