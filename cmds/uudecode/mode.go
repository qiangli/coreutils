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
		return fileMode(uint32(n)), nil
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
			perm, setid, sticky := uint32(0), false, false
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
					case 's':
						setid = true
					case 't':
						sticky = true
					default:
						return 0, errBadMode
					}
					i++
				}
			}
			set, clear := modeBits(who, perm, setid, sticky)
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
	return fileMode(bits), nil
}

var errBadMode = strconv.ErrSyntax

func modeBits(who, perm uint32, setid, sticky bool) (set, clear uint32) {
	if who&1 != 0 {
		set |= perm << 6
		clear |= 0o4700
		if setid {
			set |= 0o4000
		}
	}
	if who&2 != 0 {
		set |= perm << 3
		clear |= 0o2070
		if setid {
			set |= 0o2000
		}
	}
	if who&4 != 0 {
		set |= perm
		clear |= 0o1007
		if sticky {
			set |= 0o1000
		}
	}
	return
}

func fileMode(bits uint32) os.FileMode {
	m := os.FileMode(bits & 0o777)
	if bits&0o4000 != 0 {
		m |= os.ModeSetuid
	}
	if bits&0o2000 != 0 {
		m |= os.ModeSetgid
	}
	if bits&0o1000 != 0 {
		m |= os.ModeSticky
	}
	return m
}
