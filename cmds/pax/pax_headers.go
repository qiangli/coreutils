package paxcmd

import (
	"archive/tar"
	"bytes"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

func writeGlobalPAXHeader(rc *tool.RunContext, o *options, tw *tar.Writer) (bool, error) {
	if len(o.paxOptions.global) == 0 && o.paxOptions.globalName == "" {
		return false, nil
	}
	pattern := o.paxOptions.globalName
	if pattern == "" {
		tmp := rc.Getenv("TMPDIR")
		if tmp == "" {
			tmp = "/tmp"
		}
		pattern = filepath.ToSlash(filepath.Join(tmp, "GlobalHead.%p.%n"))
	}
	name, err := expandGlobalHeaderName(pattern, 1)
	if err != nil {
		return false, err
	}
	archiveName, nameErr := localTextToArchive(rc, name)
	if nameErr == nil {
		name = archiveName
	} else if o.paxOptions.invalid != "binary" {
		return false, fmt.Errorf("global extended-header name cannot be translated to UTF-8")
	}
	records, binaryValue, err := localPAXValuesToArchive(rc, o.paxOptions.global, o.paxOptions.invalid)
	if err != nil {
		return false, err
	}
	for key := range records {
		if deletedPAXKeyword(o.paxOptions, key) {
			delete(records, key)
		}
	}
	if len(records) == 0 {
		// globexthdr.name alone still requests a global extended header. A
		// private format marker gives archive/tar a record to materialize while
		// remaining harmless to other readers.
		records["COREUTILS.format"] = "pax"
	}
	if o.paxOptions.invalid == "binary" {
		probe := &tar.Header{Name: name, PAXRecords: records}
		if binaryValue || paxHeaderNeedsBinary(rc, probe) {
			records["hdrcharset"] = "BINARY"
		}
	}
	err = tw.WriteHeader(&tar.Header{
		Name: name, Typeflag: tar.TypeXGlobalHeader,
		Format: tar.FormatPAX, PAXRecords: records,
	})
	return nameErr != nil || binaryValue, err
}

func expandGlobalHeaderName(pattern string, sequence int) (string, error) {
	return expandHeaderPattern(pattern, map[byte]string{
		'n': strconv.Itoa(sequence), 'p': strconv.Itoa(os.Getpid()), '%': "%",
	})
}

func expandExtendedHeaderName(pattern, member string) (string, error) {
	if pattern == "" {
		pattern = "%d/PaxHeaders.%p/%f"
	}
	dir, file := path.Dir(member), path.Base(member)
	return expandHeaderPattern(pattern, map[byte]string{
		'd': dir, 'f': file, 'p': strconv.Itoa(os.Getpid()), '%': "%",
	})
}

func expandHeaderPattern(pattern string, replacements map[byte]string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' {
			out.WriteByte(pattern[i])
			continue
		}
		i++
		if i >= len(pattern) {
			return "", fmt.Errorf("trailing %% in extended-header name")
		}
		value, ok := replacements[pattern[i]]
		if !ok {
			return "", fmt.Errorf("unsupported %%%c in extended-header name", pattern[i])
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

type optionTarReader struct {
	tr      *tar.Reader
	options paxOptions
	global  map[string]string
	raw     []rawUSTARFields
	index   int
}

type rawUSTARFields struct {
	name, prefix, magic, version, checksum string
}

func newOptionTarReader(data []byte, options paxOptions, preserveRaw bool) *optionTarReader {
	r := &optionTarReader{tr: tar.NewReader(bytes.NewReader(data)), options: options, global: map[string]string{}}
	if preserveRaw {
		r.raw = scanRawUSTARFields(data)
	}
	return r
}

func scanRawUSTARFields(data []byte) []rawUSTARFields {
	var fields []rawUSTARFields
	for off := 0; off+512 <= len(data); {
		header := data[off : off+512]
		if allZero(header) {
			break
		}
		size, err := rawTarSize(header)
		if err != nil || size < 0 {
			break
		}
		if header[156] != tar.TypeXHeader && header[156] != tar.TypeXGlobalHeader {
			fields = append(fields, rawUSTARFields{
				name:     string(bytes.TrimRight(header[:100], "\x00")),
				prefix:   string(bytes.TrimRight(header[345:500], "\x00")),
				magic:    string(bytes.TrimRight(header[257:263], "\x00")),
				version:  string(bytes.TrimRight(header[263:265], "\x00")),
				checksum: strings.Trim(string(bytes.Trim(header[148:156], " \x00")), " "),
			})
		}
		off += 512 + int((size+511)&^511)
	}
	return fields
}

func (r *optionTarReader) Next() (*tar.Header, error) {
	for {
		h, err := r.tr.Next()
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			for key, value := range h.PAXRecords {
				if value == "" {
					delete(r.global, key)
				} else {
					r.global[key] = value
				}
			}
			continue
		}
		if r.index < len(r.raw) {
			if h.PAXRecords == nil {
				h.PAXRecords = map[string]string{}
			}
			fields := r.raw[r.index]
			h.PAXRecords["COREUTILS.internal.ustar.name"] = fields.name
			h.PAXRecords["COREUTILS.internal.ustar.prefix"] = fields.prefix
			h.PAXRecords["COREUTILS.internal.ustar.magic"] = fields.magic
			h.PAXRecords["COREUTILS.internal.ustar.version"] = fields.version
			h.PAXRecords["COREUTILS.internal.ustar.chksum"] = fields.checksum
			r.index++
		}
		values := maps.Clone(r.global)
		for key, value := range r.options.global {
			if !deletedPAXKeyword(r.options, key) {
				values[key] = value
			}
		}
		for key, value := range h.PAXRecords {
			values[key] = value
		}
		for key, value := range r.options.local {
			if deletedPAXKeyword(r.options, key) {
				delete(values, key)
				continue
			}
			if value == "" {
				delete(values, key)
			} else {
				values[key] = value
			}
		}
		if err := applyPAXValues(h, values); err != nil {
			return nil, err
		}
		h.PAXRecords = values
		return h, nil
	}
}

func (r *optionTarReader) Read(p []byte) (int, error) { return r.tr.Read(p) }

// filterDeletedPAXRecords operates on physical tar records before archive/tar
// folds local records into Header fields. This is required for delete=path to
// expose the underlying ustar name rather than the already-overridden name.
func filterDeletedPAXRecords(data []byte, options paxOptions) ([]byte, error) {
	hasDeletion := len(options.deletes) != 0
	for _, value := range options.local {
		hasDeletion = hasDeletion || value == ""
	}
	if !hasDeletion {
		return data, nil
	}
	var out bytes.Buffer
	for off := 0; off+512 <= len(data); {
		header := append([]byte(nil), data[off:off+512]...)
		if allZero(header) {
			out.Write(data[off:])
			return out.Bytes(), nil
		}
		size, err := rawTarSize(header)
		if err != nil {
			return nil, err
		}
		padded := (size + 511) &^ 511
		end := off + 512 + int(padded)
		if size < 0 || end > len(data) {
			return nil, fmt.Errorf("truncated tar member")
		}
		payload := data[off+512 : off+512+int(size)]
		if header[156] == tar.TypeXHeader || header[156] == tar.TypeXGlobalHeader {
			records, err := parseRawPAXRecords(payload)
			if err != nil {
				return nil, err
			}
			for key := range records {
				if deletedPAXKeyword(options, key) {
					delete(records, key)
				}
			}
			if len(records) == 0 {
				off = end
				continue
			}
			payload = formatRawPAXRecords(records)
			setRawTarSize(header, int64(len(payload)))
			setRawTarChecksum(header)
			out.Write(header)
			out.Write(payload)
			out.Write(make([]byte, (512-len(payload)%512)%512))
		} else {
			out.Write(data[off:end])
		}
		off = end
	}
	return nil, fmt.Errorf("invalid tar archive: missing end markers")
}

func rawTarSize(header []byte) (int64, error) {
	field := strings.Trim(string(bytes.Trim(header[124:136], " \x00")), " ")
	if field == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(field, 8, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid tar size field")
	}
	return n, nil
}

func setRawTarSize(header []byte, size int64) {
	for i := 124; i < 136; i++ {
		header[i] = 0
	}
	copy(header[124:136], fmt.Sprintf("%011o", size))
}

func setRawTarChecksum(header []byte) {
	for i := 148; i < 156; i++ {
		header[i] = ' '
	}
	var sum int64
	for _, b := range header {
		sum += int64(b)
	}
	copy(header[148:156], fmt.Sprintf("%06o\x00 ", sum))
}

// patchRawMemberNames restores the ustar name/prefix pair for ordinary pax
// members. archive/tar deliberately truncates the physical name field when it
// writes a PAX header; POSIX instead requires a representable pathname to use
// the ustar prefix split, and a command-line path:= override belongs only in
// the preceding extended header.
func patchRawMemberNames(data []byte, names []string) ([]byte, error) {
	out := append([]byte(nil), data...)
	index := 0
	for off := 0; off+512 <= len(out); {
		header := out[off : off+512]
		if allZero(header) {
			if index != len(names) {
				return nil, fmt.Errorf("tar member/name count mismatch")
			}
			return out, nil
		}
		size, err := rawTarSize(header)
		if err != nil {
			return nil, err
		}
		next := off + 512 + int((size+511)&^511)
		if next > len(out) {
			return nil, fmt.Errorf("truncated tar member")
		}
		if header[156] != tar.TypeXHeader && header[156] != tar.TypeXGlobalHeader {
			if index >= len(names) {
				return nil, fmt.Errorf("tar member/name count mismatch")
			}
			name := filepath.ToSlash(names[index])
			index++
			for i := 0; i < 100; i++ {
				header[i] = 0
			}
			for i := 345; i < 500; i++ {
				header[i] = 0
			}
			prefix, suffix := "", name
			if len(name) > 100 {
				cut := -1
				for i := len(name) - 1; i >= 0; i-- {
					if name[i] == '/' && i <= 155 && len(name)-i-1 <= 100 {
						cut = i
						break
					}
				}
				if cut >= 0 {
					prefix, suffix = name[:cut], name[cut+1:]
				} else {
					suffix = name[:min(len(name), 100)]
				}
			}
			copy(header[:100], suffix)
			copy(header[345:500], prefix)
			setRawTarChecksum(header)
		}
		off = next
	}
	return nil, fmt.Errorf("invalid tar archive: missing end markers")
}

func parseRawPAXRecords(data []byte) (map[string]string, error) {
	records := map[string]string{}
	for len(data) != 0 {
		space := bytes.IndexByte(data, ' ')
		if space <= 0 {
			return nil, fmt.Errorf("invalid pax extended-header record")
		}
		length, err := strconv.Atoi(string(data[:space]))
		if err != nil || length <= space+2 || length > len(data) || data[length-1] != '\n' {
			return nil, fmt.Errorf("invalid pax extended-header record length")
		}
		record := data[space+1 : length-1]
		eq := bytes.IndexByte(record, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid pax extended-header assignment")
		}
		records[string(record[:eq])] = string(record[eq+1:])
		data = data[length:]
	}
	return records, nil
}

func formatRawPAXRecords(records map[string]string) []byte {
	var out strings.Builder
	for _, key := range slices.Sorted(maps.Keys(records)) {
		body := key + "=" + records[key] + "\n"
		length := len(body) + 2
		for {
			digits := len(strconv.Itoa(length))
			next := digits + 1 + len(body)
			if next == length {
				break
			}
			length = next
		}
		fmt.Fprintf(&out, "%d %s", length, body)
	}
	return []byte(out.String())
}

func patchExtendedHeaderNames(data []byte, pattern string) ([]byte, error) {
	out := append([]byte(nil), data...)
	for off := 0; off+512 <= len(out); {
		header := out[off : off+512]
		if allZero(header) {
			return out, nil
		}
		size, err := rawTarSize(header)
		if err != nil {
			return nil, err
		}
		padded := (size + 511) &^ 511
		next := off + 512 + int(padded)
		if next > len(out) {
			return nil, fmt.Errorf("truncated tar member")
		}
		if header[156] == tar.TypeXHeader {
			if next+512 > len(out) {
				return nil, fmt.Errorf("extended header has no member")
			}
			member := rawTarName(out[next : next+512])
			name, err := expandExtendedHeaderName(pattern, member)
			if err != nil {
				return nil, err
			}
			if err := setRawTarName(header, name); err != nil {
				return nil, err
			}
			setRawTarChecksum(header)
		}
		off = next
	}
	return nil, fmt.Errorf("invalid tar archive: missing end markers")
}

func patchLinkdataHeaders(data []byte) ([]byte, error) {
	var out bytes.Buffer
	pendingLink := ""
	for off := 0; off+512 <= len(data); {
		header := append([]byte(nil), data[off:off+512]...)
		if allZero(header) {
			out.Write(data[off:])
			return out.Bytes(), nil
		}
		size, err := rawTarSize(header)
		if err != nil {
			return nil, err
		}
		padded := (size + 511) &^ 511
		end := off + 512 + int(padded)
		if end > len(data) {
			return nil, fmt.Errorf("truncated tar member")
		}
		payload := data[off+512 : off+512+int(size)]
		if header[156] == tar.TypeXHeader {
			records, err := parseRawPAXRecords(payload)
			if err != nil {
				return nil, err
			}
			if target, ok := records["COREUTILS.linkdata"]; ok {
				pendingLink = target
				delete(records, "COREUTILS.linkdata")
				payload = formatRawPAXRecords(records)
				if len(records) != 0 {
					setRawTarSize(header, int64(len(payload)))
					setRawTarChecksum(header)
					out.Write(header)
					out.Write(payload)
					out.Write(make([]byte, (512-len(payload)%512)%512))
				}
				off = end
				continue
			}
		}
		if pendingLink != "" && header[156] != tar.TypeXGlobalHeader {
			header[156] = tar.TypeLink
			if err := setRawTarLinkname(header, pendingLink); err != nil {
				return nil, err
			}
			setRawTarChecksum(header)
			pendingLink = ""
		}
		out.Write(header)
		out.Write(data[off+512 : end])
		off = end
	}
	return nil, fmt.Errorf("invalid tar archive: missing end markers")
}

func rawTarName(header []byte) string {
	name := string(bytes.TrimRight(header[:100], "\x00"))
	prefix := string(bytes.TrimRight(header[345:500], "\x00"))
	if prefix != "" {
		return prefix + "/" + name
	}
	return name
}

func setRawTarName(header []byte, name string) error {
	if strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("extended-header name contains NUL")
	}
	prefix, suffix := "", name
	if len(suffix) > 100 {
		found := false
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '/' && i <= 155 && len(name)-i-1 <= 100 {
				prefix, suffix, found = name[:i], name[i+1:], true
				break
			}
		}
		if !found {
			return fmt.Errorf("extended-header name %q exceeds ustar limits", name)
		}
	}
	for i := 0; i < 100; i++ {
		header[i] = 0
	}
	for i := 345; i < 500; i++ {
		header[i] = 0
	}
	copy(header[:100], suffix)
	copy(header[345:500], prefix)
	return nil
}

func setRawTarLinkname(header []byte, name string) error {
	if len(name) > 100 || strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("linkdata target %q exceeds ustar limits", name)
	}
	for i := 157; i < 257; i++ {
		header[i] = 0
	}
	copy(header[157:257], name)
	return nil
}
