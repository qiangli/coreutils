package paxcmd

import (
	"archive/tar"
	"fmt"
	"maps"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

type paxOptionMode int

const (
	paxList paxOptionMode = iota
	paxRead
	paxWrite
	paxCopy
)

type paxOptions struct {
	deletes                []string
	exthdrName, globalName string
	invalid                string
	linkdata, times        bool
	listSet                bool
	listFormat             string
	global, local          map[string]string
	needsPAX               bool
}

func applyWritePAXOptions(rc *tool.RunContext, h *tar.Header, options paxOptions) (bool, error) {
	if value, ok := options.global["size"]; ok && value != strconv.FormatInt(h.Size, 10) {
		return false, fmt.Errorf("-o size=%s conflicts with member %q size %d", value, h.Name, h.Size)
	}
	if value, ok := options.local["size"]; ok && value != "" && value != strconv.FormatInt(h.Size, 10) {
		return false, fmt.Errorf("-o size:=%s conflicts with member %q size %d", value, h.Name, h.Size)
	}
	if h.PAXRecords == nil {
		h.PAXRecords = map[string]string{}
	}
	localValues, binaryValue, err := localPAXValuesToArchive(rc, options.local, options.invalid)
	if err != nil {
		return false, err
	}
	for key, value := range localValues {
		h.PAXRecords[key] = value
	}
	if err := applyPAXValues(h, localValues); err != nil {
		return false, err
	}
	headerBinary := paxHeaderNeedsBinary(rc, h)
	if options.invalid == "binary" && (binaryValue || headerBinary) {
		h.PAXRecords["hdrcharset"] = "BINARY"
	}
	for key := range h.PAXRecords {
		if deletedPAXKeyword(options, key) {
			delete(h.PAXRecords, key)
		}
	}
	return binaryValue || headerBinary, nil
}

func localPAXValuesToArchive(rc *tool.RunContext, values map[string]string, action string) (map[string]string, bool, error) {
	out := make(map[string]string, len(values))
	binary := false
	for _, key := range slices.Sorted(maps.Keys(values)) {
		value := values[key]
		translated, err := localTextToArchive(rc, value)
		if err != nil {
			if action != "binary" {
				return nil, false, fmt.Errorf("-o %s value cannot be translated to UTF-8", key)
			}
			translated = value
			binary = true
		}
		out[key] = translated
	}
	return out, binary, nil
}

// translatePAXIdentityToArchive converts the locally sourced textual identity
// fields populated by tar.FileInfoHeader. Name and Linkname are converted by
// addPath after substitution and interactive rename, so converting them here
// would corrupt single-byte locale data a second time.
func translatePAXIdentityToArchive(rc *tool.RunContext, h *tar.Header, action string) error {
	for _, field := range []struct {
		label string
		value *string
	}{
		{label: "uname", value: &h.Uname},
		{label: "gname", value: &h.Gname},
	} {
		translated, err := localTextToArchive(rc, *field.value)
		if err != nil {
			if action == "binary" {
				continue
			}
			return fmt.Errorf("%s cannot be translated to UTF-8", field.label)
		}
		*field.value = translated
	}
	return nil
}

func paxHeaderNeedsBinary(rc *tool.RunContext, h *tar.Header) bool {
	for _, value := range []string{h.Name, h.Linkname, h.Uname, h.Gname} {
		if invalidPAXText(rc, value) {
			return true
		}
	}
	for _, value := range h.PAXRecords {
		if invalidPAXText(rc, value) {
			return true
		}
	}
	return false
}

func invalidPAXDestinationName(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 || !utf8.ValidString(name) || len(name) > 4096 {
		return true
	}
	for _, component := range strings.FieldsFunc(filepath.ToSlash(name), func(r rune) bool { return r == '/' }) {
		if len(component) > 255 {
			return true
		}
	}
	return false
}

func invalidPAXLocalDestinationName(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 || len(name) > 4096 {
		return true
	}
	for _, component := range strings.Split(filepath.ToSlash(name), "/") {
		if len(component) > 255 {
			return true
		}
	}
	return false
}

func invalidPAXText(rc *tool.RunContext, value string) bool {
	_, err := archiveTextToLocal(rc, value, false)
	return err != nil
}

type paxTextEncoding int

const (
	paxASCII paxTextEncoding = iota
	paxUTF8
	paxLatin1
	paxLatin15
	paxUnknown
)

func paxInvocationEncoding(rc *tool.RunContext) paxTextEncoding {
	name := locale.Resolve(rc.Env, locale.CType)
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ := strings.Cut(name, ".")
	codeset = strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	switch codeset {
	case "UTF8":
		return paxUTF8
	case "ISO88591":
		return paxLatin1
	case "ISO885915":
		return paxLatin15
	case "":
		if base == "C" || base == "POSIX" {
			return paxASCII
		}
	}
	return paxUnknown
}

// archiveTextToLocal translates the pax UTF-8 interchange encoding into the
// invocation's carried LC_CTYPE codeset. lossy is used only by invalid=write.
func archiveTextToLocal(rc *tool.RunContext, value string, lossy bool) (string, error) {
	if !utf8.ValidString(value) {
		if !lossy {
			return "", fmt.Errorf("archive value is not UTF-8")
		}
		value = strings.ToValidUTF8(value, "?")
	}
	encoding := paxInvocationEncoding(rc)
	if encoding == paxUTF8 {
		return value, nil
	}
	var out strings.Builder
	for _, r := range value {
		var b byte
		ok := true
		switch encoding {
		case paxASCII, paxUnknown:
			ok = r < utf8.RuneSelf
			b = byte(r)
		case paxLatin1:
			ok = r <= 0xff
			b = byte(r)
		case paxLatin15:
			b, ok = encodeISO885915(r)
		}
		if !ok {
			if !lossy {
				return "", fmt.Errorf("value cannot be represented in selected LC_CTYPE")
			}
			b = '?'
		}
		out.WriteByte(b)
	}
	return out.String(), nil
}

// localTextToArchive translates invocation-local bytes into pax UTF-8.
func localTextToArchive(rc *tool.RunContext, value string) (string, error) {
	encoding := paxInvocationEncoding(rc)
	switch encoding {
	case paxUTF8:
		if !utf8.ValidString(value) {
			return "", fmt.Errorf("local value is not UTF-8")
		}
		return value, nil
	case paxASCII, paxUnknown:
		for i := 0; i < len(value); i++ {
			if value[i] >= utf8.RuneSelf {
				return "", fmt.Errorf("local value is not representable in selected LC_CTYPE")
			}
		}
		return value, nil
	case paxLatin1, paxLatin15:
		var out strings.Builder
		for i := 0; i < len(value); i++ {
			r := rune(value[i])
			if encoding == paxLatin15 {
				r = decodeISO885915(value[i])
			}
			out.WriteRune(r)
		}
		return out.String(), nil
	}
	return "", fmt.Errorf("unsupported LC_CTYPE")
}

func iso885915Rune(r rune) bool {
	if r <= 0xff {
		switch r {
		case 0x00a4, 0x00a6, 0x00a8, 0x00b4, 0x00b8, 0x00bc, 0x00bd, 0x00be:
			return false
		}
		return true
	}
	switch r {
	case 0x20ac, 0x0160, 0x0161, 0x017d, 0x017e, 0x0152, 0x0153, 0x0178:
		return true
	}
	return false
}

func encodeISO885915(r rune) (byte, bool) {
	switch r {
	case 0x20ac:
		return 0xa4, true
	case 0x0160:
		return 0xa6, true
	case 0x0161:
		return 0xa8, true
	case 0x017d:
		return 0xb4, true
	case 0x017e:
		return 0xb8, true
	case 0x0152:
		return 0xbc, true
	case 0x0153:
		return 0xbd, true
	case 0x0178:
		return 0xbe, true
	}
	if iso885915Rune(r) && r <= 0xff {
		return byte(r), true
	}
	return 0, false
}

func decodeISO885915(b byte) rune {
	switch b {
	case 0xa4:
		return 0x20ac
	case 0xa6:
		return 0x0160
	case 0xa8:
		return 0x0161
	case 0xb4:
		return 0x017d
	case 0xb8:
		return 0x017e
	case 0xbc:
		return 0x0152
	case 0xbd:
		return 0x0153
	case 0xbe:
		return 0x0178
	}
	return rune(b)
}

type paxInvalidHeaderFields struct {
	name, link, other bool
}

func translatePAXHeaderToLocal(rc *tool.RunContext, h *tar.Header, action string, listMode bool) paxInvalidHeaderFields {
	var invalid paxInvalidHeaderFields
	hdrcharset := h.PAXRecords["hdrcharset"]
	if hdrcharset == "BINARY" {
		return invalid
	}
	unsupportedCharset := hdrcharset != "" && hdrcharset != "UTF-8" && hdrcharset != "ISO-IR 10646 2000 UTF-8"
	// In read/copy mode binary explicitly suppresses translation, but a failed
	// attempted translation is still diagnosed. In list mode it follows bypass
	// and therefore uses translated values when translation succeeds.
	suppressTranslation := action == "binary" && !listMode
	lossy := action == "write" && !listMode
	translate := func(value *string, field *bool) {
		if unsupportedCharset {
			*field = true
			return
		}
		translated, err := archiveTextToLocal(rc, *value, false)
		if err != nil {
			*field = true
			if lossy {
				if replacement, replacementErr := archiveTextToLocal(rc, *value, true); replacementErr == nil {
					*value = replacement
				}
			}
			return
		}
		if !suppressTranslation {
			*value = translated
		}
	}
	translate(&h.Name, &invalid.name)
	if h.Linkname != "" {
		translate(&h.Linkname, &invalid.link)
	}
	translate(&h.Uname, &invalid.other)
	translate(&h.Gname, &invalid.other)
	// Extraction rewrites the member headers into a private tar stream. Keep
	// its extended records in archive encoding: Header's concrete fields take
	// precedence when tar.Writer emits path/linkpath/uname/gname, while custom
	// records must not be transcoded twice. Listing consumes the values and
	// therefore translates every string operand.
	if listMode && h.PAXRecords != nil {
		for key, value := range h.PAXRecords {
			if key == "hdrcharset" || strings.HasPrefix(key, "COREUTILS.internal.cpio.filedata") {
				continue
			}
			bad := false
			translate(&value, &bad)
			invalid.other = invalid.other || bad
			h.PAXRecords[key] = value
		}
	}
	invalid.name = invalid.name || invalidPAXLocalDestinationName(h.Name)
	if h.Linkname != "" {
		invalid.link = invalid.link || invalidPAXLocalDestinationName(h.Linkname)
	}
	return invalid
}

func invalidPAXDestination(rc *tool.RunContext, name string) bool {
	return invalidPAXDestinationName(name) || invalidPAXText(rc, name)
}

func invalidPAXListHeader(rc *tool.RunContext, h *tar.Header) bool {
	for _, value := range []string{h.Name, h.Linkname, h.Uname, h.Gname} {
		if value != "" && invalidPAXText(rc, value) {
			return true
		}
	}
	for _, value := range h.PAXRecords {
		if invalidPAXText(rc, value) {
			return true
		}
	}
	return false
}

func parsePAXOptions(args []string, mode paxOptionMode, format string) (paxOptions, error) {
	o := paxOptions{invalid: "bypass", global: map[string]string{}, local: map[string]string{}}
	for _, arg := range args {
		items, err := splitPAXOptionArgument(arg)
		if err != nil {
			return paxOptions{}, err
		}
		for _, item := range items {
			key, value, local, hasValue := splitPAXAssignment(item)
			if !portableKeyword(key) {
				return paxOptions{}, fmt.Errorf("invalid -o keyword %q", key)
			}
			switch key {
			case "delete":
				if !hasValue || local || value == "" {
					return paxOptions{}, fmt.Errorf("-o delete requires a non-empty pattern")
				}
				if _, err := path.Match(value, "probe"); err != nil {
					return paxOptions{}, fmt.Errorf("invalid -o delete pattern %q: %v", value, err)
				}
				o.deletes = append(o.deletes, value)
				o.needsPAX = true
			case "exthdr.name":
				if !hasValue || local || (mode != paxWrite && mode != paxCopy) {
					return paxOptions{}, fmt.Errorf("-o exthdr.name is valid only in write or copy mode")
				}
				if _, err := expandExtendedHeaderName(value, "dir/file"); err != nil {
					return paxOptions{}, err
				}
				o.exthdrName, o.needsPAX = value, true
			case "globexthdr.name":
				if !hasValue || local || (mode != paxWrite && mode != paxCopy) {
					return paxOptions{}, fmt.Errorf("-o globexthdr.name is valid only in write or copy mode")
				}
				if _, err := expandGlobalHeaderName(value, 1); err != nil {
					return paxOptions{}, err
				}
				o.globalName, o.needsPAX = value, true
			case "invalid":
				if !hasValue || local {
					return paxOptions{}, fmt.Errorf("-o invalid requires an action")
				}
				switch value {
				case "binary", "bypass", "rename", "UTF-8", "write":
				default:
					return paxOptions{}, fmt.Errorf("invalid -o invalid action %q", value)
				}
				if mode == paxWrite && value != "binary" {
					return paxOptions{}, fmt.Errorf("-o invalid=%s is not applicable in write mode", value)
				}
				o.invalid, o.needsPAX = value, true
			case "linkdata":
				if hasValue || mode != paxWrite {
					return paxOptions{}, fmt.Errorf("-o linkdata is valid only as a keyword in write mode")
				}
				o.linkdata, o.needsPAX = true, true
			case "listopt":
				if !hasValue || local || mode != paxList {
					return paxOptions{}, fmt.Errorf("-o listopt is valid only in list mode")
				}
				o.listFormat += value
				o.listSet = true
			case "times":
				if hasValue || (mode != paxWrite && mode != paxCopy) {
					return paxOptions{}, fmt.Errorf("-o times is valid only as a keyword in write or copy mode")
				}
				o.times, o.needsPAX = true, true
			default:
				if !hasValue {
					return paxOptions{}, fmt.Errorf("-o %s requires '=' or ':=' and a value", key)
				}
				if !standardPAXKeyword(key) && !strings.Contains(key, ".") {
					return paxOptions{}, fmt.Errorf("unsupported -o extended-header keyword %q", key)
				}
				if local {
					o.local[key] = value
				} else {
					o.global[key] = value
				}
				o.needsPAX = true
			}
		}
	}
	if o.needsPAX && (mode == paxWrite || mode == paxCopy) && format != "pax" {
		return paxOptions{}, fmt.Errorf("-o option is applicable only to the pax archive format")
	}
	return o, nil
}

// listopt consumes the rest of its option-argument verbatim. Other values may
// quote a comma as \,, with the backslash removed.
func splitPAXOptionArgument(arg string) ([]string, error) {
	var out []string
	for pos := 0; pos < len(arg); {
		for pos < len(arg) && unicode.IsSpace(rune(arg[pos])) {
			pos++
		}
		if pos == len(arg) {
			return out, nil
		}
		if strings.HasPrefix(arg[pos:], "listopt=") {
			out = append(out, "listopt="+strings.ReplaceAll(arg[pos+len("listopt="):], `\,`, ","))
			return out, nil
		}
		start := pos
		var b strings.Builder
		for pos < len(arg) {
			if arg[pos] == '\\' && pos+1 < len(arg) && arg[pos+1] == ',' {
				b.WriteString(arg[start:pos])
				b.WriteByte(',')
				pos += 2
				start = pos
				continue
			}
			if arg[pos] == ',' {
				break
			}
			pos++
		}
		b.WriteString(arg[start:pos])
		item := b.String()
		if strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("empty keyword in -o option")
		}
		out = append(out, item)
		if pos < len(arg) {
			pos++
			if strings.TrimSpace(arg[pos:]) == "" {
				return out, nil
			}
		}
	}
	return out, nil
}

func splitPAXAssignment(item string) (key, value string, local, hasValue bool) {
	item = strings.TrimLeftFunc(item, unicode.IsSpace)
	if i := strings.Index(item, ":="); i >= 0 {
		return item[:i], item[i+2:], true, true
	}
	if i := strings.IndexByte(item, '='); i >= 0 {
		return item[:i], item[i+1:], false, true
	}
	return item, "", false, false
}

func portableKeyword(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}

func standardPAXKeyword(s string) bool {
	switch s {
	case "atime", "charset", "comment", "gid", "gname", "hdrcharset", "linkpath", "mtime", "path", "size", "uid", "uname":
		return true
	}
	return strings.HasPrefix(s, "realtime.") || strings.HasPrefix(s, "security.")
}

func deletedPAXKeyword(o paxOptions, key string) bool {
	if value, ok := o.local[key]; ok && value == "" {
		return true
	}
	for _, pattern := range o.deletes {
		if ok, _ := path.Match(pattern, key); ok {
			return true
		}
	}
	return false
}

func applyPAXValues(h *tar.Header, values map[string]string) error {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		value := values[key]
		if value == "" {
			continue
		}
		var err error
		switch key {
		case "path":
			h.Name = value
		case "linkpath":
			h.Linkname = value
		case "uname":
			h.Uname = value
		case "gname":
			h.Gname = value
		case "uid":
			var n int64
			n, err = strconv.ParseInt(value, 10, 64)
			h.Uid = int(n)
		case "gid":
			var n int64
			n, err = strconv.ParseInt(value, 10, 64)
			h.Gid = int(n)
		case "size":
			h.Size, err = strconv.ParseInt(value, 10, 64)
		case "atime":
			h.AccessTime, err = parsePAXTime(value)
		case "mtime":
			h.ModTime, err = parsePAXTime(value)
		}
		if err != nil {
			return fmt.Errorf("invalid %s extended-header value %q", key, value)
		}
	}
	// Name records have normative precedence over their numeric counterparts.
	// Resolve them after the map pass so Go's randomized map iteration cannot
	// invert uname/uid or gname/gid precedence.
	if name := values["uname"]; name != "" {
		if account, err := user.Lookup(name); err == nil {
			if uid, err := strconv.ParseInt(account.Uid, 10, 64); err == nil {
				h.Uid = int(uid)
			}
		}
	}
	if name := values["gname"]; name != "" {
		if group, err := user.LookupGroup(name); err == nil {
			if gid, err := strconv.ParseInt(group.Gid, 10, 64); err == nil {
				h.Gid = int(gid)
			}
		}
	}
	return nil
}

func parsePAXTime(value string) (time.Time, error) {
	parts := strings.SplitN(value, ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	var nanos int64
	if len(parts) == 2 {
		fraction := parts[1]
		if fraction == "" {
			return time.Time{}, fmt.Errorf("missing fractional digits")
		}
		for _, r := range fraction {
			if r < '0' || r > '9' {
				return time.Time{}, fmt.Errorf("invalid fractional time")
			}
		}
		if len(fraction) > 9 {
			fraction = fraction[:9]
		}
		fraction += strings.Repeat("0", 9-len(fraction))
		nanos, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		if strings.HasPrefix(value, "-") {
			nanos = -nanos
		}
	}
	return time.Unix(seconds, nanos), nil
}
