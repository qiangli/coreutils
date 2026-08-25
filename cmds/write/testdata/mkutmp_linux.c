#include <stddef.h>
#include <stdio.h>
#include <string.h>
#include <utmp.h>

int main(int argc, char **argv) {
    struct utmp record;
    FILE *out;

    if (argc != 2) {
        return 2;
    }
    memset(&record, 0, sizeof(record));
    record.ut_type = USER_PROCESS;
    record.ut_pid = 4242;
    memcpy(record.ut_user, "native-user", sizeof("native-user") - 1);
    memcpy(record.ut_line, "pts/42", sizeof("pts/42") - 1);
    memcpy(record.ut_host, "native-host", sizeof("native-host") - 1);
    record.ut_tv.tv_sec = 1700000000;

    printf("%zu %zu %zu %zu %zu %zu %zu %zu\n",
           sizeof(record), offsetof(struct utmp, ut_user),
           offsetof(struct utmp, ut_line), offsetof(struct utmp, ut_host),
           offsetof(struct utmp, ut_type), offsetof(struct utmp, ut_pid),
           offsetof(struct utmp, ut_tv), sizeof(record.ut_tv.tv_sec));

    out = fopen(argv[1], "wb");
    if (out == NULL) {
        return 3;
    }
    if (fwrite(&record, sizeof(record), 1, out) != 1) {
        fclose(out);
        return 4;
    }
    if (fclose(out) != 0) {
        return 5;
    }
    return 0;
}
