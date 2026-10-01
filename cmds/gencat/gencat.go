// Package gencatcmd implements POSIX gencat for the Linux product profile.
// The catalog encoder writes the documented GNU libc catalog layout so files
// can be consumed by the host's catopen/catgets interfaces.
package gencatcmd

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

const catalogMagic = uint32(0x960408de)

var cmd = &tool.Tool{
	Name: "gencat", Synopsis: "Generate a formatted message catalog.",
	Usage: "gencat catfile msgfile...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type messageKey struct{ set, id uint32 }
type catalog map[messageKey]string

func run(rc *tool.RunContext, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(rc.Err, "gencat: usage: gencat catfile msgfile...")
		return 2
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			fmt.Fprintf(rc.Err, "gencat: unsupported option %s\n", arg)
			return 2
		}
	}
	cat := make(catalog)
	output := args[0]
	if output != "-" {
		data, err := os.ReadFile(rc.Path(output))
		if err == nil {
			cat, err = decodeCatalog(data)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(rc.Err, "gencat: %s: %v\n", output, err)
			return 1
		}
	}
	for _, name := range args[1:] {
		var reader io.Reader = rc.In
		if name != "-" {
			file, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "gencat: %s: %v\n", name, err)
				return 1
			}
			reader = file
			if err := mergeSource(cat, reader); err != nil {
				file.Close()
				fmt.Fprintf(rc.Err, "gencat: %s: %v\n", name, err)
				return 1
			}
			file.Close()
		} else if err := mergeSource(cat, reader); err != nil {
			fmt.Fprintf(rc.Err, "gencat: standard input: %v\n", err)
			return 1
		}
	}
	data, err := encodeCatalog(cat)
	if err != nil {
		fmt.Fprintf(rc.Err, "gencat: %v\n", err)
		return 1
	}
	if output == "-" {
		if _, err = rc.Out.Write(data); err != nil {
			fmt.Fprintf(rc.Err, "gencat: standard output: %v\n", err)
			return 1
		}
		return 0
	}
	if err = os.WriteFile(rc.Path(output), data, 0o666); err != nil {
		fmt.Fprintf(rc.Err, "gencat: %s: %v\n", output, err)
		return 1
	}
	return 0
}

func mergeSource(cat catalog, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	set, quote := uint32(1), byte(0)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		for strings.HasSuffix(line, "\\") {
			line = strings.TrimSuffix(line, "\\")
			if !scanner.Scan() {
				break
			}
			line += scanner.Text()
			lineNo++
		}
		if line == "" {
			continue
		}
		if line[0] == '$' {
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			switch parts[0] {
			case "$set", "$delset":
				if len(parts) < 2 {
					return fmt.Errorf("line %d: missing set number", lineNo)
				}
				n, err := positive(parts[1])
				if err != nil {
					return fmt.Errorf("line %d: invalid set number: %w", lineNo, err)
				}
				if parts[0] == "$set" {
					set = n
				} else {
					for k := range cat {
						if k.set == n {
							delete(cat, k)
						}
					}
				}
			case "$quote":
				quote = 0
				if len(parts) > 1 {
					quote = parts[1][0]
				}
			case "$":
			default:
				return fmt.Errorf("line %d: unknown directive %s", lineNo, parts[0])
			}
			continue
		}
		sep := strings.IndexAny(line, " \t")
		idText := line
		if sep >= 0 {
			idText = line[:sep]
		}
		id, err := positive(idText)
		if err != nil {
			return fmt.Errorf("line %d: invalid message number: %w", lineNo, err)
		}
		key := messageKey{set, id}
		if sep < 0 {
			delete(cat, key)
			continue
		}
		body := line[sep+1:]
		if quote != 0 && len(body) > 0 && body[0] == quote {
			if len(body) < 2 || body[len(body)-1] != quote {
				return fmt.Errorf("line %d: unterminated quoted message", lineNo)
			}
			body = body[1 : len(body)-1]
		}
		cat[key] = unescape(body)
	}
	return scanner.Err()
}

func positive(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || n == 0 || n == 0xffffffff {
		return 0, fmt.Errorf("%q must be a positive 32-bit number", s)
	}
	return uint32(n), nil
}

func unescape(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			out.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case 'b':
			out.WriteByte('\b')
		case 'r':
			out.WriteByte('\r')
		case 'f':
			out.WriteByte('\f')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			value := int(s[i] - '0')
			for j := 0; j < 2 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7'; j++ {
				i++
				value = value*8 + int(s[i]-'0')
			}
			out.WriteByte(byte(value))
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

func encodeCatalog(cat catalog) ([]byte, error) {
	keys := make([]messageKey, 0, len(cat))
	for k := range cat {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].set != keys[j].set {
			return keys[i].set < keys[j].set
		}
		return keys[i].id < keys[j].id
	})
	size := uint32(len(keys) + 1)
	buckets := make([][]messageKey, size)
	depth := 1
	for _, k := range keys {
		bucket := ((k.set + 1) * k.id) % size
		buckets[bucket] = append(buckets[bucket], k)
		if len(buckets[bucket]) > depth {
			depth = len(buckets[bucket])
		}
	}
	if uint64(size)*uint64(depth) > 1<<24 {
		return nil, errors.New("catalog is too large")
	}
	table := make([]uint32, int(size)*depth*3)
	var stringsData bytes.Buffer
	for bucket, entries := range buckets {
		for layer, k := range entries {
			idx := (layer*int(size) + bucket) * 3
			table[idx], table[idx+1], table[idx+2] = k.set+1, k.id, uint32(stringsData.Len())
			stringsData.WriteString(cat[k])
			stringsData.WriteByte(0)
		}
	}
	var out bytes.Buffer
	for _, n := range []uint32{catalogMagic, size, uint32(depth)} {
		binary.Write(&out, binary.LittleEndian, n)
	}
	for _, n := range table {
		binary.Write(&out, binary.LittleEndian, n)
	}
	for _, n := range table {
		binary.Write(&out, binary.BigEndian, n)
	}
	out.Write(stringsData.Bytes())
	return out.Bytes(), nil
}

func decodeCatalog(data []byte) (catalog, error) {
	if len(data) < 12 || binary.LittleEndian.Uint32(data[:4]) != catalogMagic {
		return nil, errors.New("unsupported existing catalog format")
	}
	size, depth := binary.LittleEndian.Uint32(data[4:8]), binary.LittleEndian.Uint32(data[8:12])
	if size == 0 || depth == 0 || uint64(size)*uint64(depth) > 1<<24 {
		return nil, errors.New("invalid catalog table dimensions")
	}
	entries := uint64(size) * uint64(depth) * 3
	start := uint64(12) + entries*8
	if start > uint64(len(data)) {
		return nil, errors.New("truncated catalog table")
	}
	cat := make(catalog)
	for i := uint64(0); i < entries; i += 3 {
		off := uint64(12) + i*4
		set := binary.LittleEndian.Uint32(data[off:])
		if set == 0 {
			continue
		}
		id := binary.LittleEndian.Uint32(data[off+4:])
		stringOff := uint64(binary.LittleEndian.Uint32(data[off+8:])) + start
		if stringOff >= uint64(len(data)) {
			return nil, errors.New("catalog string offset is out of range")
		}
		end := bytes.IndexByte(data[stringOff:], 0)
		if end < 0 {
			return nil, errors.New("catalog string is unterminated")
		}
		cat[messageKey{set - 1, id}] = string(data[stringOff : stringOff+uint64(end)])
	}
	return cat, nil
}
