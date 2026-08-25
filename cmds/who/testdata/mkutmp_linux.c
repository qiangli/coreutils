/*
 * Independent Linux who fixture generator. Compile and run this file on the
 * target libc/architecture: the compiler supplies struct utmp's true ABI.
 * It is test evidence only and is never linked into coreutils.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <utmp.h>

static void put(FILE *f, short type, int pid, const char *user,
                const char *line, const char *id, long long sec,
                short term, short status) {
    struct utmp u;
    memset(&u, 0, sizeof(u));
    u.ut_type = type;
    u.ut_pid = pid;
    if (user) strncpy(u.ut_user, user, sizeof(u.ut_user));
    if (line) strncpy(u.ut_line, line, sizeof(u.ut_line));
    if (id) memcpy(u.ut_id, id, strlen(id) < sizeof(u.ut_id) ? strlen(id) : sizeof(u.ut_id));
    u.ut_tv.tv_sec = sec;
    u.ut_exit.e_termination = term;
    u.ut_exit.e_exit = status;
    if (fwrite(&u, sizeof(u), 1, f) != 1) exit(2);
}

int main(int argc, char **argv) {
    if (argc != 2) return 2;
    FILE *f = fopen(argv[1], "wb");
    if (!f) return 1;
    put(f, BOOT_TIME, 0, "reboot", "~", "~~", 1700000000, 0, 0);
    put(f, RUN_LVL, 'S' + ('5' << 8), "runlevel", "~", "~~", 1700000001, 0, 0);
    put(f, OLD_TIME, 0, "", "|", "~~", 1700000002, 0, 0);
    put(f, NEW_TIME, 0, "", "{", "~~", 1700000003, 0, 0);
    put(f, INIT_PROCESS, 111, "", "tty1", "si", 1700000004, 0, 0);
    put(f, LOGIN_PROCESS, 222, "LOGIN", "tty2", "ty2", 1700000005, 0, 0);
    put(f, USER_PROCESS, 333, "alice", "pts/0", "p/0", 1700000006, 0, 0);
    put(f, DEAD_PROCESS, 444, "", "pts/9", "p/9", 1700000007, 9, 3);
    return fclose(f) == 0 ? 0 : 1;
}
