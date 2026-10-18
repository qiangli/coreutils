package pscmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ps lives below Yoke in the dependency graph, so this encoder implements the
// public bashy-graph-v1 wire contract without importing Yoke's stream package.
const processGraphSource = "graph:ps"

type processGraphNode struct {
	T             string         `json:"t"`
	SchemaVersion string         `json:"schema_version"`
	Ref           string         `json:"ref"`
	Path          string         `json:"path"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Type          string         `json:"type"`
	Attrs         map[string]any `json:"attrs"`
}

type processGraphEdge struct {
	T             string         `json:"t"`
	SchemaVersion string         `json:"schema_version"`
	Src           string         `json:"src"`
	Rel           string         `json:"rel"`
	Dst           string         `json:"dst"`
	Attrs         map[string]any `json:"attrs"`
}

func processRef(pid int) string { return "proc:" + strconv.Itoa(pid) }

func writeProcessGraph(w io.Writer, procs []process) error {
	// The caller's selection can arrive in any order. Sorting before writing
	// also makes the revision independent of the process source's enumeration.
	procs = append([]process(nil), procs...)
	sort.Slice(procs, func(i, j int) bool { return processRef(procs[i].pid) < processRef(procs[j].pid) })
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(processGraphNode{
		T: "node", SchemaVersion: "bashy-node-v1", Ref: processGraphSource,
		Name: "processes", Kind: "roster", Type: "ps/processes", Attrs: map[string]any{},
	}); err != nil {
		return err
	}
	for _, p := range procs {
		attrs := map[string]any{
			"pid": p.pid, "ppid": p.ppid, "ruid": p.ruid, "euid": p.euid,
			"rgid": p.rgid, "egid": p.egid, "command": p.command,
			"args": p.args, "tty": p.tty, "state": p.state,
		}
		if !p.start.IsZero() {
			attrs["start"] = p.start.UTC().Format(time.RFC3339Nano)
		}
		if p.cpuKnown {
			attrs["cpu_ns"] = p.cpu.Nanoseconds()
		}
		if p.vszKnown {
			attrs["virtual_bytes"] = p.vsz
		}
		if p.szKnown {
			attrs["resident_bytes"] = p.sz
		}
		if err := enc.Encode(processGraphNode{
			T: "node", SchemaVersion: "bashy-node-v1", Ref: processRef(p.pid),
			Name: p.command, Kind: "process", Type: "ps/process", Attrs: attrs,
		}); err != nil {
			return err
		}
	}
	for _, p := range procs {
		if err := enc.Encode(processGraphEdge{
			T: "edge", SchemaVersion: "bashy-edge-v1", Src: processGraphSource,
			Rel: "contains", Dst: processRef(p.pid), Attrs: map[string]any{},
		}); err != nil {
			return err
		}
	}
	sum := sha256.Sum256(body.Bytes())
	rev := "sha256:" + hex.EncodeToString(sum[:])
	tokenSum := sha256.Sum256([]byte(strings.Join([]string{
		"bashy-cursor-v1", "ps", processGraphSource, "op=list", rev, "end",
	}, "\x00")))
	var frame bytes.Buffer
	out := json.NewEncoder(&frame)
	out.SetEscapeHTML(false)
	if err := out.Encode(struct {
		T             string `json:"t"`
		SchemaVersion string `json:"schema_version"`
		Source        string `json:"source"`
		Rev           string `json:"rev"`
	}{"header", "bashy-graph-v1", processGraphSource, rev}); err != nil {
		return err
	}
	frame.Write(body.Bytes())
	if err := out.Encode(map[string]any{
		"t": "cursor",
		"cursor": map[string]any{
			"schema_version": "bashy-cursor-v1", "kind": "pagination",
			"token": hex.EncodeToString(tokenSum[:16]),
			"binding": map[string]any{
				"provider": "ps", "target": processGraphSource,
				"query": "op=list", "revision": rev,
			},
		},
	}); err != nil {
		return err
	}
	if _, err := w.Write(frame.Bytes()); err != nil {
		return fmt.Errorf("graph stream: %w", err)
	}
	return nil
}
