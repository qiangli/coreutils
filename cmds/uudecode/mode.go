package uudecodecmd

import (
	"os"
	"strconv"
	"strings"
)

// parseHeaderMode accepts the octal mode required by uuencode as well as the
// POSIX chmod symbolic grammar. Symbolic modes start from no permissions: a
// begin header describes the resulting access bits, not a mutation of the
// decoder's process mode or umask.
func parseHeaderMode(s string) (os.FileMode, error) {
	if n, err := strconv.ParseUint(s, 8, 32); err == nil && n <= 0o7777 {
		// Only rwxrwxrwx are file access permission bits. Ignore encoded
		// set-ID/sticky attributes rather than applying them from untrusted data.
		return os.FileMode(n).Perm(), nil
	}
	if s == "" {
		return 0, errBadMode
	}
	var bits uint32
	for _, clause := range strings.Split(s, ",") {
		i, who := 0, uint32(0)
		for i < len(clause) {
			switch clause[i] {
			case 'u':
				who |= 1
			case 'g':
				who |= 2
			case 'o':
				who |= 4
			case 'a':
				who |= 7
			default:
				goto operators
			}
			i++
		}
	operators:
		if i == len(clause) {
			return 0, errBadMode
		}
		if who == 0 {
			who = 7
		}
		for i < len(clause) {
			op := clause[i]
			if op != '+' && op != '-' && op != '=' {
				return 0, errBadMode
			}
			i++
			perm := uint32(0)
			if i < len(clause) && strings.ContainsRune("ugo", rune(clause[i])) &&
				(i+1 == len(clause) || strings.ContainsRune("+-=", rune(clause[i+1]))) {
				switch clause[i] {
				case 'u':
					perm = bits >> 6 & 7
				case 'g':
					perm = bits >> 3 & 7
				case 'o':
					perm = bits & 7
				}
				i++
			} else {
				for i < len(clause) && !strings.ContainsRune("+-=", rune(clause[i])) {
					switch clause[i] {
					case 'r':
						perm |= 4
					case 'w':
						perm |= 2
					case 'x':
						perm |= 1
					case 'X':
						if bits&0o111 != 0 {
							perm |= 1
						}
					case 's', 't':
						// These are chmod attributes, not file access permissions,
						// and are not valid in a POSIX uuencode mode description.
						return 0, errBadMode
					default:
						return 0, errBadMode
					}
					i++
				}
			}
			set, clear := modeBits(who, perm)
			switch op {
			case '+':
				bits |= set
			case '-':
				bits &^= set
			case '=':
				bits = bits&^clear | set
			}
		}
	}
	return os.FileMode(bits & 0o777), nil
}

var errBadMode = strconv.ErrSyntax

func modeBits(who, perm uint32) (set, clear uint32) {
	if who&1 != 0 {
		set |= perm << 6
		clear |= 0o700
	}
	if who&2 != 0 {
		set |= perm << 3
		clear |= 0o070
	}
	if who&4 != 0 {
		set |= perm
		clear |= 0o007
	}
	return
}
