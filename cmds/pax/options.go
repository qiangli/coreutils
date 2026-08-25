package paxcmd

import (
	"archive/tar"
	"fmt"
	"os/user"
	"path"
	"path/filepath"
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
	listFormat             string
	global, local          map[string]string
	needsPAX               bool
}

func applyWritePAXOptions(h *tar.Header, options paxOptions) error {
	if value, ok := options.global["size"]; ok && value != strconv.FormatInt(h.Size, 10) {
		return fmt.Errorf("-o size=%s conflicts with member %q size %d", value, h.Name, h.Size)
	}
	if value, ok := options.local["size"]; ok && value != "" && value != strconv.FormatInt(h.Size, 10) {
		return fmt.Errorf("-o size:=%s conflicts with member %q size %d", value, h.Name, h.Size)
	}
	if h.PAXRecords == nil {
		h.PAXRecords = map[string]string{}
	}
	for key, value := range options.local {
		h.PAXRecords[key] = value
	}
	if err := applyPAXValues(h, options.local); err != nil {
		return err
	}
	if options.invalid == "binary" && (!utf8.ValidString(h.Name) || !utf8.ValidString(h.Linkname)) {
		h.PAXRecords["hdrcharset"] = "BINARY"
	}
	for key := range h.PAXRecords {
		if deletedPAXKeyword(options, key) {
			delete(h.PAXRecords, key)
		}
	}
	return nil
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

func invalidPAXText(rc *tool.RunContext, value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	ctype := locale.Resolve(rc.Env, locale.CType)
	base, codeset := ctype, ""
	if at := strings.IndexByte(base, '@'); at >= 0 {
		base = base[:at]
	}
	if at := strings.IndexByte(base, '.'); at >= 0 {
		base, codeset = base[:at], base[at+1:]
	}
	codeset = strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	if codeset == "UTF8" {
		return false
	}
	for _, r := range value {
		if r < utf8.RuneSelf {
			continue
		}
		switch codeset {
		case "ISO88591":
			if r <= 0xff {
				continue
			}
		case "ISO885915":
			if iso885915Rune(r) {
				continue
			}
		case "":
			// The only carried locales without a codeset are C and POSIX.
			// Unknown names deliberately take the same fail-closed ASCII path.
			_ = base
		}
		return true
	}
	return false
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
	for key, value := range values {
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
