# POSIX certification scope: the claimed utilities

This is the fixed list of utilities the official POSIX Shell and Utilities
certification claim covers (Linux x86_64, base scope). Read it before any
certification work. It is hand-maintained; the generated 116-name inventory is
[posix-required-commands.md](posix-required-commands.md).

## Rules for every agent

1. **Work only inside the claimed set below.** Diagnose, fix and report on
   these names only.
2. **Do nothing for the 14 optional utilities**, even when the test suite
   reports a failure in one of them. No diagnosis, no fix, no question to the
   certification authority. Record the result as "outside the claim" and move
   on.
3. **Split every result list by scope first.** Before reporting non-pass
   results, mark each one as in the claim or outside it.
4. **Do not change the list.** No utility is added to or removed from the claim
   without an explicit operator decision.
5. **No external providers.** Every claimed name is supplied by the Go
   coreutils binary or by the shell. A claimed name that resolves to a host
   binary is a defect in the test host, not an acceptable result.

The suite always runs all 117 test sets (116 utility targets plus the shell),
so results for the optional utilities appear in every journal. That does not
bring them into scope.

## In the claim: 102 utility targets, plus `gencat`

### 92 utilities supplied by the Go coreutils binary

```
at        awk       basename  batch     bc        cat       chgrp     chmod
chown     cksum     cmp       comm      cp        crontab   csplit    cut
date      dd        df        diff      dirname   du        echo      ed
env       expand    expr      false     file      find      fold      getconf
grep      head      iconv     id        join      kill      ln        locale
logger    logname   lp        ls        m4        mailx     man       mesg
mkdir     mkfifo    mv        newgrp    nice      nohup     od        paste
patch     pathchk   pax       pr        printf    ps        pwd       renice
rm        rmdir     sed       sleep     sort      split     strings   stty
tabs      tail      tee       test      time      touch     tput      tr
true      tsort     tty       uname     unexpand  uniq      uudecode  uuencode
wc        who       write     xargs
```

### 10 commands supplied by the shell

```
alias  cd  command  getopts  hash  read  sh  umask  unalias  wait
```

### One more mandatory utility with no test-suite target

`gencat` is required by the standard but has no target in the suite. It is
supplied in Go and proven separately.

## Outside the claim: 14 optional utilities

```
ar  bg  ctags  ex  fc  fg  jobs  localedef  make  more  nm  strip  talk  vi
```

These belong to optional feature groups (user portability, software
development) that the claim does not declare. Some of them exist in this
repository as Go applets; that does not put them in the claim.

## Source

The list is the command inventory recorded by the Sprint 675 full run
(92 Go, 10 shell, 0 external, 14 excluded). The operating contract and the
run procedure are kept with the private certification harness.
