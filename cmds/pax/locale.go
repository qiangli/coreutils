package paxcmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
)

// paxLocaleTables resolves both POSIX pattern categories once per invocation.
// The immutable snapshot is shared by operand matching and every -s BRE, so
// LC_CTYPE and LC_COLLATE cannot drift midway through archive processing.
func paxLocaleTables(env []string) (*bre.LocaleByteTables, error) {
	ctypeName := locale.Resolve(env, locale.CType)
	var tables *bre.LocaleByteTables
	if paxCLocale(ctypeName) || paxUTF8Locale(ctypeName) {
		tables, _ = bre.SnapshotLocaleByteCtypeTables(nil)
	} else {
		provider, err := ctype.Open(ctypeName)
		if err != nil {
			return nil, fmt.Errorf("LC_CTYPE %q: %v", ctypeName, err)
		}
		tables, err = bre.SnapshotLocaleByteCtypeTables(provider)
		closeErr := provider.Close()
		if err != nil {
			return nil, fmt.Errorf("LC_CTYPE %q: %v", ctypeName, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("LC_CTYPE %q: %v", ctypeName, closeErr)
		}
	}

	collateName := locale.Resolve(env, locale.Collate)
	if paxCLocale(collateName) || paxUTF8Locale(collateName) {
		return tables, nil
	}
	provider, err := collate.Open(collateName)
	if err != nil {
		return nil, fmt.Errorf("LC_COLLATE %q: %v", collateName, err)
	}
	withCollation, snapshotErr := tables.WithCollation(provider)
	closeErr := provider.Close()
	if snapshotErr != nil {
		return nil, fmt.Errorf("LC_COLLATE %q: %v", collateName, snapshotErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("LC_COLLATE %q: %v", collateName, closeErr)
	}
	return withCollation, nil
}

func paxCLocale(name string) bool { return name == "C" || name == "POSIX" }

func paxUTF8Locale(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "utf-8") || strings.Contains(n, "utf8")
}
