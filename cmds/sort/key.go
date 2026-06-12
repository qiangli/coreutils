package sortcmd

// Key specification parsing and extraction, following the GNU manual:
// -k POS1[,POS2], each POS of the form F[.C][OPTS]. Fields and
// character positions are origin 1; a character position of zero in
// POS2 means the field's last character. If .C is omitted from POS1 it
// defaults to 1, if omitted from POS2 it defaults to 0. With no -t,
// fields are separated by the empty string between a non-blank and a
// blank (each field keeps its leading blanks); with -t SEP the
// separator is not part of either adjacent field, but a key spanning
// fields retains the separators inside the range.

// keyOpts are the per-key ordering options (type letters n/b/f/r/h).
type keyOpts struct {
	numeric bool // n
	human   bool // h
	fold    bool // f
	reverse bool // r
	skipSB  bool // b attached to POS1 (or global -b)
	skipEB  bool // b attached to POS2 (or global -b)
}

// hasMods reports whether the key carries any ordering option other
// than r — GNU's default_key_compare(): such a key does NOT inherit
// the global ordering options.
func (o keyOpts) hasMods() bool {
	return o.numeric || o.human || o.fold || o.skipSB || o.skipEB
}

// keySpec is one parsed -k KEYDEF. All counts mirror GNU sort's
// internal representation: sword/schar are 0-based start field and
// char offsets; eword is the 0-based end field (-1 = through end of
// line); echar is the 1-based inclusive end char count (0 = through
// the end of field eword).
type keySpec struct {
	sword, schar int
	eword, echar int
	opts         keyOpts
}

// parseKeySpec parses one KEYDEF. On a malformed spec it returns the
// GNU diagnostic fragment (e.g. "field number is zero"); on a valid
// GNU type letter this implementation doesn't cover (d i g M R V z)
// it returns that letter in badType.
func parseKeySpec(spec string) (k keySpec, errMsg string, badType byte) {
	k.eword, k.echar = -1, 0
	s := spec

	n, rest, ok := parseNum(s)
	if !ok {
		return k, "invalid number at field start", 0
	}
	if n == 0 {
		return k, "field number is zero", 0
	}
	k.sword = n - 1
	s = rest
	if len(s) > 0 && s[0] == '.' {
		n, rest, ok = parseNum(s[1:])
		if !ok {
			return k, "invalid number after '.'", 0
		}
		if n == 0 {
			return k, "character offset is zero", 0
		}
		k.schar = n - 1
		s = rest
	}
	s, errMsg, badType = parseKeyOpts(s, &k.opts, true)
	if errMsg != "" || badType != 0 {
		return k, errMsg, badType
	}

	if len(s) == 0 {
		return k, "", 0
	}
	if s[0] != ',' {
		return k, "stray character in field spec", 0
	}
	n, rest, ok = parseNum(s[1:])
	if !ok {
		return k, "invalid number after ','", 0
	}
	if n == 0 {
		return k, "field number is zero", 0
	}
	k.eword = n - 1
	s = rest
	if len(s) > 0 && s[0] == '.' {
		n, rest, ok = parseNum(s[1:])
		if !ok {
			return k, "invalid number after '.'", 0
		}
		k.echar = n // 0 is allowed: the field's last character
		s = rest
	}
	s, errMsg, badType = parseKeyOpts(s, &k.opts, false)
	if errMsg != "" || badType != 0 {
		return k, errMsg, badType
	}
	if len(s) > 0 {
		return k, "stray character in field spec", 0
	}
	return k, "", 0
}

// parseKeyOpts consumes ordering letters until ',' or end of spec.
func parseKeyOpts(s string, o *keyOpts, start bool) (rest, errMsg string, badType byte) {
	for len(s) > 0 && s[0] != ',' {
		switch c := s[0]; c {
		case 'b':
			if start {
				o.skipSB = true
			} else {
				o.skipEB = true
			}
		case 'f':
			o.fold = true
		case 'h':
			o.human = true
		case 'n':
			o.numeric = true
		case 'r':
			o.reverse = true
		case 'd', 'g', 'i', 'M', 'R', 'V', 'z':
			return s, "", c
		default:
			return s, "stray character in field spec", 0
		}
		s = s[1:]
	}
	return s, "", 0
}

// parseNum reads a leading run of decimal digits.
func parseNum(s string) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:], i > 0
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// extractKey returns the key text for one line, GNU begfield/limfield
// semantics: offsets may run past field boundaries but are clamped to
// the end of the line; an inverted range yields the empty key.
func extractKey(line string, k *keySpec, tab int) string {
	beg := begfield(line, k, tab)
	lim := len(line)
	if k.eword >= 0 {
		lim = limfield(line, k, tab)
	}
	if beg >= lim {
		return ""
	}
	return line[beg:lim]
}

// begfield mirrors GNU sort's begfield(): skip sword fields, optionally
// skip the field's leading blanks (b), then advance schar characters,
// clamped to the end of the line.
func begfield(line string, k *keySpec, tab int) int {
	ptr, lim := 0, len(line)
	sword := k.sword
	if tab >= 0 {
		for ptr < lim && sword > 0 {
			for ptr < lim && line[ptr] != byte(tab) {
				ptr++
			}
			if ptr < lim {
				ptr++
			}
			sword--
		}
	} else {
		for ptr < lim && sword > 0 {
			for ptr < lim && isBlank(line[ptr]) {
				ptr++
			}
			for ptr < lim && !isBlank(line[ptr]) {
				ptr++
			}
			sword--
		}
	}
	if k.opts.skipSB {
		for ptr < lim && isBlank(line[ptr]) {
			ptr++
		}
	}
	ptr += k.schar
	if ptr > lim {
		ptr = lim
	}
	return ptr
}

// limfield mirrors GNU sort's limfield(): when echar is 0 the key runs
// through the end of field eword (in -t mode the trailing separator is
// excluded); otherwise it runs through the echar-th character of field
// eword, inclusive.
func limfield(line string, k *keySpec, tab int) int {
	ptr, lim := 0, len(line)
	eword, echar := k.eword, k.echar
	if echar == 0 {
		eword++ // skip all of the end field
	}
	if tab >= 0 {
		for ptr < lim && eword > 0 {
			for ptr < lim && line[ptr] != byte(tab) {
				ptr++
			}
			if ptr < lim && (eword > 1 || echar != 0) {
				ptr++
			}
			eword--
		}
	} else {
		for ptr < lim && eword > 0 {
			for ptr < lim && isBlank(line[ptr]) {
				ptr++
			}
			for ptr < lim && !isBlank(line[ptr]) {
				ptr++
			}
			eword--
		}
	}
	if echar != 0 {
		if k.opts.skipEB {
			for ptr < lim && isBlank(line[ptr]) {
				ptr++
			}
		}
		ptr += echar
		if ptr > lim {
			ptr = lim
		}
	}
	return ptr
}
