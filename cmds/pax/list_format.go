package paxcmd

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/locale"
)

func formatPAXList(h *tar.Header, format string, loc *time.Location, timeFormat *locale.TimeFormatter) (string, error) {
	var out strings.Builder
	for i := 0; i < len(format); {
		if format[i] == '\\' {
			text, next, err := listEscape(format, i)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
			i = next
			continue
		}
		if format[i] != '%' {
			out.WriteByte(format[i])
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			out.WriteByte('%')
			i += 2
			continue
		}
		i++
		keyword := ""
		if i < len(format) && format[i] == '(' {
			end := strings.IndexByte(format[i+1:], ')')
			if end < 0 {
				return "", fmt.Errorf("unterminated listopt keyword")
			}
			end += i + 1
			keyword = format[i+1 : end]
			i = end + 1
		}
		formatStart := i
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		formatEnd := i
		if keyword == "" && i < len(format) && format[i] == '(' {
			end := strings.IndexByte(format[i+1:], ')')
			if end < 0 {
				return "", fmt.Errorf("unterminated listopt keyword")
			}
			end += i + 1
			keyword = format[i+1 : end]
			i = end + 1
		}
		if i >= len(format) {
			return "", fmt.Errorf("incomplete listopt conversion")
		}
		conv := format[i]
		spec := "%" + format[formatStart:formatEnd] + string(conv)
		i++
		value, err := listConversion(h, keyword, conv, spec, loc, timeFormat)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

func listFormatUsesTime(format string) (bool, error) {
	for i := 0; i < len(format); {
		if format[i] == '\\' {
			_, next, err := listEscape(format, i)
			if err != nil {
				return false, err
			}
			i = next
			continue
		}
		if format[i] != '%' {
			i++
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			i++
			continue
		}
		if i < len(format) && format[i] == '(' {
			end := strings.IndexByte(format[i+1:], ')')
			if end < 0 {
				return false, fmt.Errorf("unterminated listopt keyword")
			}
			i += end + 2
		}
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i < len(format) && format[i] == '(' {
			end := strings.IndexByte(format[i+1:], ')')
			if end < 0 {
				return false, fmt.Errorf("unterminated listopt keyword")
			}
			i += end + 2
		}
		if i >= len(format) {
			return false, fmt.Errorf("incomplete listopt conversion")
		}
		if format[i] == 'T' {
			return true, nil
		}
		i++
	}
	return false, nil
}

func listConversion(h *tar.Header, keyword string, conv byte, spec string, loc *time.Location, timeFormat *locale.TimeFormatter) (string, error) {
	switch conv {
	case 'T':
		key, subformat := keyword, "%b %e %H:%M %Y"
		if key == "" {
			key = "mtime"
		}
		if at := strings.IndexByte(key, '='); at >= 0 {
			key, subformat = key[:at], key[at+1:]
		}
		v, ok := listKeyword(h, key)
		if !ok {
			return "", fmt.Errorf("unknown listopt time keyword %q", key)
		}
		t, ok := v.(time.Time)
		if !ok {
			return "", fmt.Errorf("listopt keyword %q is not a time", key)
		}
		if timeFormat == nil {
			return "", fmt.Errorf("listopt %%T requires LC_TIME data")
		}
		return timeFormat.Format(t.In(loc), subformat)
	case 'M':
		if keyword != "" && keyword != "mode" {
			return "", fmt.Errorf("listopt %%M requires the mode keyword")
		}
		return applyStringWidth(spec, headerModeString(h)), nil
	case 'D':
		if keyword != "" {
			v, ok := listKeyword(h, keyword)
			if !ok {
				return "", fmt.Errorf("unknown listopt keyword %q", keyword)
			}
			return numericListFormat(spec[:len(spec)-1]+"u", v)
		}
		if h.Typeflag == tar.TypeChar || h.Typeflag == tar.TypeBlock {
			return fmt.Sprintf("%d,%d", h.Devmajor, h.Devminor), nil
		}
		return " ", nil
	case 'F', 'L':
		keys := []string{"path"}
		if keyword != "" {
			keys = strings.Split(keyword, ",")
		}
		var parts []string
		for _, key := range keys {
			v, ok := listKeyword(h, key)
			if !ok {
				continue
			}
			s := fmt.Sprint(v)
			if s != "" {
				parts = append(parts, s)
			}
		}
		name := strings.Join(parts, "/")
		if conv == 'L' && h.Typeflag == tar.TypeSymlink {
			name += " -> " + h.Linkname
		}
		return applyStringWidth(spec, name), nil
	}
	if !strings.ContainsRune("diouxXfFeEgGcs", rune(conv)) {
		return "", fmt.Errorf("unsupported listopt conversion %%%c", conv)
	}
	if keyword == "" {
		return "", fmt.Errorf("listopt %%%c requires a keyword", conv)
	}
	v, ok := listKeyword(h, keyword)
	if !ok {
		return "", fmt.Errorf("unknown listopt keyword %q", keyword)
	}
	if conv == 's' {
		return fmt.Sprintf(spec, fmt.Sprint(v)), nil
	}
	if conv == 'c' {
		s := fmt.Sprint(v)
		if s == "" {
			return fmt.Sprintf(spec, rune(0)), nil
		}
		return fmt.Sprintf(spec, []rune(s)[0]), nil
	}
	return numericListFormat(spec, v)
}

func numericListFormat(spec string, value any) (string, error) {
	conv := spec[len(spec)-1]
	if conv == 'u' {
		spec = spec[:len(spec)-1] + "d"
	}
	wantsFloat := strings.ContainsRune("fFeEgG", rune(conv))
	switch v := value.(type) {
	case int:
		if wantsFloat {
			return fmt.Sprintf(spec, float64(v)), nil
		}
		return fmt.Sprintf(spec, v), nil
	case int64:
		if wantsFloat {
			return fmt.Sprintf(spec, float64(v)), nil
		}
		return fmt.Sprintf(spec, v), nil
	case byte:
		if wantsFloat {
			return fmt.Sprintf(spec, float64(v)), nil
		}
		return fmt.Sprintf(spec, v), nil
	case time.Time:
		if wantsFloat {
			return fmt.Sprintf(spec, float64(v.Unix())), nil
		}
		return fmt.Sprintf(spec, v.Unix()), nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(v, 64)
			if ferr != nil {
				return "", fmt.Errorf("listopt value %q is not numeric", v)
			}
			return fmt.Sprintf(spec, f), nil
		}
		return fmt.Sprintf(spec, n), nil
	default:
		return "", fmt.Errorf("listopt value is not numeric")
	}
}

func listKeyword(h *tar.Header, key string) (any, bool) {
	if strings.HasPrefix(key, "c_") {
		if key == "c_filedata" {
			encoded, ok := h.PAXRecords["COREUTILS.internal.cpio.filedata"]
			if !ok || !strings.HasPrefix(encoded, "b64:") {
				return nil, false
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "b64:"))
			if err != nil {
				return nil, false
			}
			return string(data), true
		}
		if v, ok := h.PAXRecords["COREUTILS.cpio."+key]; ok {
			return v, true
		}
	}
	if v, ok := h.PAXRecords[key]; ok {
		return v, true
	}
	switch key {
	case "path":
		return h.Name, true
	case "name":
		_, name := splitUSTARPath(h.Name)
		return name, true
	case "linkpath", "linkname":
		return h.Linkname, true
	case "mode":
		return h.Mode, true
	case "uid":
		return h.Uid, true
	case "gid":
		return h.Gid, true
	case "size", "filesize":
		return h.Size, true
	case "mtime":
		return h.ModTime, true
	case "atime":
		return h.AccessTime, !h.AccessTime.IsZero()
	case "uname":
		return h.Uname, true
	case "gname":
		return h.Gname, true
	case "devmajor":
		return h.Devmajor, true
	case "devminor":
		return h.Devminor, true
	case "typeflag":
		return string([]byte{h.Typeflag}), true
	case "prefix":
		prefix, _ := splitUSTARPath(h.Name)
		return prefix, true
	case "magic":
		return "ustar", true
	case "version":
		return "00", true
	case "chksum":
		return reconstructedTarChecksum(h), true
	}
	return nil, false
}

func splitUSTARPath(name string) (prefix, base string) {
	if len(name) <= 100 {
		return "", name
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' && i <= 155 && len(name)-i-1 <= 100 {
			return name[:i], name[i+1:]
		}
	}
	return "", name
}

func reconstructedTarChecksum(h *tar.Header) int64 {
	clone := *h
	clone.PAXRecords = nil
	clone.Xattrs = nil
	clone.Format = tar.FormatUSTAR
	var raw strings.Builder
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&clone); err != nil {
		return 0
	}
	data := []byte(raw.String())
	if len(data) < 156 {
		return 0
	}
	field := strings.Trim(string(bytes.Trim(data[148:156], " \x00")), " ")
	n, _ := strconv.ParseInt(field, 8, 64)
	return n
}

func applyStringWidth(spec, value string) string {
	if len(spec) == 0 {
		return value
	}
	return fmt.Sprintf(spec[:len(spec)-1]+"s", value)
}

func headerModeString(h *tar.Header) string {
	s := h.FileInfo().Mode().String()
	if strings.HasPrefix(s, "L") {
		s = "l" + s[1:]
	}
	return s
}

func listEscape(format string, i int) (string, int, error) {
	if i+1 >= len(format) {
		return "", 0, fmt.Errorf("trailing backslash in listopt")
	}
	switch format[i+1] {
	case 'a':
		return "\a", i + 2, nil
	case 'b':
		return "\b", i + 2, nil
	case 'f':
		return "\f", i + 2, nil
	case 'n':
		return "\n", i + 2, nil
	case 'r':
		return "\r", i + 2, nil
	case 't':
		return "\t", i + 2, nil
	case 'v':
		return "\v", i + 2, nil
	case '\\':
		return "\\", i + 2, nil
	}
	if format[i+1] >= '0' && format[i+1] <= '7' {
		end := i + 2
		for end < len(format) && end < i+4 && format[end] >= '0' && format[end] <= '7' {
			end++
		}
		n, _ := strconv.ParseUint(format[i+1:end], 8, 8)
		return string([]byte{byte(n)}), end, nil
	}
	return string(format[i+1]), i + 2, nil
}
