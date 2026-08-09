package meet

import (
	"encoding/json"
	"strings"
)

// An agent CLI does not always speak prose on its stdout. When bashy asks a tool
// to stream its turn live (see chat.Options.Stream), the fleet registry hands the
// tool its structured-event flags — claude gets `--output-format stream-json
// --verbose`, ycode gets an NDJSON events channel — so what lands in the captured
// turn is TRANSPORT, one JSON envelope per line:
//
//	{"type":"system","subtype":"init",...,"session_id":"…"}
//	{"type":"assistant","message":{"content":[{"type":"text","text":"…"},{"type":"tool_use",…}]},"session_id":"…"}
//	{"type":"result","subtype":"success","is_error":false,"usage":{…},"session_id":"…"}
//
// A meeting is a CONVERSATION: the record and the live view want the assistant's
// human-readable words and nothing else — not the init banner, not the model's
// thinking, not a tool call, not the token-usage summary. Recorded and replayed
// verbatim, the transport also poisons the next agent's prompt context and bloats
// the minutes with machine noise.
//
// So this is the normalization seam every meet turn passes through. It recognizes
// the two configured transports — Claude stream-json and ycode/first-party NDJSON
// — extracts only the assistant text, and DROPS the rest. It is fail-closed for
// exactly those shapes: an unrecognized event type inside a recognized stream is
// dropped rather than shown raw, and a line that opens like a JSON object but does
// not parse is treated as a torn transport line and dropped, never leaked as half
// an event. Everything it does NOT recognize as one of those transports — plain
// prose, another harness's output, an agent that legitimately answered with JSON —
// is preserved byte-for-byte, because guessing that arbitrary output is transport
// would silently eat a real answer.

// lineClass is what classifyEventLine decided one line is.
type lineClass int

const (
	// linePlain is anything not recognized as a configured transport envelope:
	// plain prose, another harness's stdout, an agent's own JSON answer. Preserved
	// verbatim.
	linePlain lineClass = iota
	// lineText is a recognized envelope carrying assistant human-readable text.
	lineText
	// lineDrop is recognized transport with no human text — a system/thinking/
	// tool-call/tool-result/usage/control/result event, or a torn transport line.
	lineDrop
)

// claudeMessage is the Anthropic Messages payload on a Claude `assistant` event.
type claudeMessage struct {
	// Content is either an array of typed blocks or, defensively, a bare string.
	Content json.RawMessage `json:"content"`
}

// claudeBlock is one content block. Only `text` blocks are human-readable; a
// `tool_use`, `thinking`, or `redacted_thinking` block is machine/interior state
// and must never reach the record.
type claudeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// eventProbe is the minimal envelope shared enough to route a line. `type` names
// the event for both transports; `session_id` is present on EVERY Claude
// stream-json line and is the fingerprint that marks a line as Claude transport
// even when its `type` is one we do not enumerate. `message`/`data` carry the
// payload we extract text from.
type eventProbe struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Message   json.RawMessage `json:"message"`
	Data      json.RawMessage `json:"data"`
}

// classifyEventLine decides what one physical line of agent output is, and, when
// it is assistant text, returns that text.
//
// Recognition of BOTH transports is bound to a fingerprint that a foreign JSON
// answer cannot forge merely by naming a matching `type`, because eating a real
// answer that happens to look like transport is the worst failure this seam has:
//
//   - Claude is keyed on `session_id`, which every stream-json line carries and a
//     foreign answer does not. A bare {"type":"assistant","message":{…}} with no
//     session_id is NOT Claude transport — it is preserved.
//   - ycode/first-party is keyed on the configured envelope from chat/events.go:
//     an exact event name (turn.start/tool.call/turn.end) AND the `data` object,
//     with turn.end additionally carrying the configured `status`. A foreign
//     {"type":"turn.end","data":{"text":…}} that lacks that shape is preserved.
func classifyEventLine(line string) (string, lineClass) {
	s := strings.TrimSpace(line)
	if s == "" || s[0] != '{' {
		// Not even shaped like a JSON object — ordinary prose. Preserve it.
		return line, linePlain
	}
	var p eventProbe
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		// It OPENS like a JSON object but does not parse. In practice that is a
		// torn or malformed transport line, and showing half an event to a watcher
		// is worse than showing nothing — fail-closed and drop it.
		return "", lineDrop
	}

	// Claude stream-json. The session_id fingerprint — present on EVERY Claude line
	// and absent from a foreign answer — is what binds a line to this transport, so
	// text is extracted ONLY from a real Claude `assistant` event, never from a
	// look-alike. An `assistant` event is the only one carrying human-readable
	// words; extract its text blocks (dropping tool_use/thinking). Every other
	// Claude event — system, user (tool_result), result, stream_event,
	// control_request/response, and any future unenumerated type — is transport and
	// is dropped rather than leaked (fail-closed on the fingerprint).
	if p.SessionID != "" {
		if p.Type == "assistant" && len(p.Message) > 0 {
			if t := claudeAssistantText(p.Message); t != "" {
				return t, lineText
			}
		}
		return "", lineDrop
	}

	// ycode / first-party NDJSON: the configured event vocabulary is exactly
	// turn.start / tool.call / turn.end, each carrying a `data` object (see
	// chat/events.go). Recognition is bound to that configured shape so a foreign
	// JSON that merely names one of those types is not mistaken for transport.
	if t, class, ok := classifyNDJSON(p); ok {
		return t, class
	}

	// A JSON object that is neither configured transport. It may be an agent's own
	// JSON answer or another harness's output; preserving it is the safe default,
	// since eating a real answer is worse than showing one machine-shaped line.
	return line, linePlain
}

// classifyNDJSON recognizes a first-party/ycode NDJSON envelope, bound to the
// configured shape from chat/events.go, and reports ok=false for anything that is
// not unambiguously that transport (so the caller preserves it verbatim).
//
// The binding is the CONFIGURED contract, not a loose `type` prefix: the event
// name must be one of the three constants AND the line must carry the `data`
// object every ycode event has. turn.end additionally must carry the configured
// `status` field — that is what separates the real answer envelope
// {"type":"turn.end","data":{"status":"ok","text":"…"}} from a foreign answer
// that happens to be shaped like {"type":"turn.end","data":{"text":"…"}}.
func classifyNDJSON(p eventProbe) (text string, class lineClass, ok bool) {
	if !hasJSONObject(p.Data) {
		return "", linePlain, false
	}
	switch p.Type {
	case "turn.start", "tool.call":
		// Structured transport with no human-readable words — drop it.
		return "", lineDrop, true
	case "turn.end":
		// Only the configured turn-end envelope (a present `status`) is our
		// transport; a look-alike without it is a foreign answer, preserved.
		var d struct {
			Status *string `json:"status"`
			Text   string  `json:"text"`
		}
		if err := json.Unmarshal(p.Data, &d); err != nil || d.Status == nil {
			return "", linePlain, false
		}
		if t := strings.TrimSpace(d.Text); t != "" {
			return t, lineText, true
		}
		return "", lineDrop, true
	}
	return "", linePlain, false
}

// hasJSONObject reports whether raw is a non-empty JSON object payload.
func hasJSONObject(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return len(s) > 0 && s[0] == '{'
}

// claudeAssistantText joins the human-readable text of a Claude assistant message,
// skipping tool_use / thinking / redacted_thinking blocks.
func claudeAssistantText(raw json.RawMessage) string {
	var m claudeMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if len(m.Content) == 0 {
		return ""
	}
	// Content is normally an array of blocks; tolerate a bare string too.
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			return strings.TrimSpace(s)
		}
		return ""
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// normalizeLiveLine is the seam for ONE line of the live channel: the text to show
// and whether to show it at all. A dropped transport line returns keep=false so
// the observer's idle notice can still tell a quiet meeting from a busy one.
func normalizeLiveLine(line string) (text string, keep bool) {
	t, class := classifyEventLine(line)
	if class == lineDrop {
		return "", false
	}
	return t, true
}

// normalizeTurnText is the seam for a WHOLE captured turn — the concatenated
// stdout of a one-shot, or a completed live turn. It walks the lines, keeps
// assistant text and genuine plain output, and drops transport.
//
// It is fail-safe in the common case: output with no recognized transport line at
// all (ordinary prose, or a tool this seam does not model) is returned byte-for-
// byte, so normalizing already-clean text — a marker, a human note, a turn a fixed
// build already stored clean — is an exact no-op and safe to run at every seam.
func normalizeTurnText(raw string) string {
	if raw == "" || !strings.Contains(raw, "{") {
		return raw // nothing shaped like a JSON envelope — plain text, untouched.
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	recognized := false
	for _, line := range lines {
		text, class := classifyEventLine(line)
		switch class {
		case lineText:
			recognized = true
			out = append(out, text)
		case lineDrop:
			recognized = true
		default: // linePlain
			out = append(out, line)
		}
	}
	if !recognized {
		return raw // no transport seen — preserve the original exactly.
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}
