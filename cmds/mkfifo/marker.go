package mkfifocmd

// The Windows FIFO marker format, v1. docs/windows-fifo.md is the contract;
// this file is its one in-repo implementation. The shell's open path
// re-implements the reader half from that document (it cannot import this
// module), so a change here without a version bump there is a wire break.
//
// A FIFO on Windows is a small regular file (the marker) naming a
// \\.\pipe\ leaf; pipe instances are created at open time by readers.
// The format code is portable on purpose: the grammar is enforced and
// tested on every host, not only where it runs.

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const (
	// MarkerMagic is the first line of a FIFO marker file, newline included.
	MarkerMagic = "!<bashyfifo>\n"

	// pipeLeafPrefix starts every FIFO pipe leaf; the token after it is
	// 128 random bits in lowercase hex.
	pipeLeafPrefix = "bashy-fifo-"
	pipeTokenLen   = 32

	// MaxMarkerSize is a cheap pre-filter for detection: no v1 marker is
	// larger (v1 markers are exactly len(MarkerMagic)+len(leaf)+1 bytes).
	MaxMarkerSize = 128

	// pipeDir is the local named-pipe namespace the leaf resolves under.
	pipeDir = `\\.\pipe\`
)

// NewPipeLeaf returns a fresh pipe leaf name: bashy-fifo-<32 lowercase hex>.
func NewPipeLeaf() (string, error) {
	var b [pipeTokenLen / 2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return pipeLeafPrefix + hex.EncodeToString(b[:]), nil
}

// PipePath returns the native CreateNamedPipe/CreateFile path for a leaf.
func PipePath(leaf string) string { return pipeDir + leaf }

// EncodeMarker renders the exact marker bytes for a leaf.
func EncodeMarker(leaf string) []byte {
	return []byte(MarkerMagic + leaf + "\n")
}

// ParseMarker validates content against the v1 grammar and returns the pipe
// leaf. The strictness is a security boundary, not pedantry: an opener that
// accepted a looser leaf could be pointed by a crafted marker at an
// arbitrary \\.\pipe\ name outside the bashy-fifo- namespace.
func ParseMarker(content []byte) (leaf string, ok bool) {
	if len(content) > MaxMarkerSize {
		return "", false
	}
	s := string(content)
	if !strings.HasPrefix(s, MarkerMagic) {
		return "", false
	}
	s = s[len(MarkerMagic):]
	if !strings.HasSuffix(s, "\n") {
		return "", false
	}
	leaf = s[:len(s)-1]
	if !validPipeLeaf(leaf) {
		return "", false
	}
	return leaf, true
}

func validPipeLeaf(leaf string) bool {
	if !strings.HasPrefix(leaf, pipeLeafPrefix) {
		return false
	}
	token := leaf[len(pipeLeafPrefix):]
	if len(token) != pipeTokenLen {
		return false
	}
	for i := 0; i < len(token); i++ {
		c := token[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
