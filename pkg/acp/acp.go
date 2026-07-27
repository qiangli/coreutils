package acp

import (
	"context"
	"fmt"
	"os/exec"

	sdk "github.com/coder/acp-go-sdk"
)

// This is the ONE file that imports the ACP SDK. Every SDK type stays behind
// this boundary: the public API (types.go, recorder.go) is defined in
// bashy-owned types, and the conversions below translate between the two.
// Replacing the SDK with a hand-written client means rewriting only this file.

// Client launches and manages an ACP agent subprocess.
type Client struct {
	conn            *sdk.ClientSideConnection
	cmd             *exec.Cmd
	protocolVersion int
}

// NewClient starts the given command as an ACP agent subprocess, connects to
// it, and initializes with minimal capabilities (filesystem and terminal
// declined via an empty ClientCapabilities).
//
// The handler implements the callbacks the agent will invoke. Use BaseHandler
// (or RecordingHandler) if you only need the default behavior.
func NewClient(ctx context.Context, handler Handler, cmd *exec.Cmd) (*Client, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start: %w", err)
	}

	conn := sdk.NewClientSideConnection(sdkAdapter{h: handler}, stdin, stdout)

	initResp, err := conn.Initialize(ctx, sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{},
	})
	if err != nil {
		// Reap the subprocess we just started so it does not linger.
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("acp: initialize: %w", err)
	}

	return &Client{
		conn:            conn,
		cmd:             cmd,
		protocolVersion: int(initResp.ProtocolVersion),
	}, nil
}

// ProtocolVersion reports the protocol version the agent negotiated during
// initialize. A value the caller does not speak (e.g. a v2-only peer) is how
// an incompatible agent is detected.
func (c *Client) ProtocolVersion() int { return c.protocolVersion }

// NewSession creates a session with the given absolute working directory.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	resp, err := c.conn.NewSession(ctx, sdk.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []sdk.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("acp: new session: %w", err)
	}
	return string(resp.SessionId), nil
}

// Prompt sends a text prompt to the given session and blocks until the turn
// completes, returning the structured stop reason. A cancelled turn returns
// StopReasonCancelled with a nil error.
func (c *Client) Prompt(ctx context.Context, sessionID, prompt string) (StopReason, error) {
	resp, err := c.conn.Prompt(ctx, sdk.PromptRequest{
		SessionId: sdk.SessionId(sessionID),
		Prompt:    []sdk.ContentBlock{toSDKContent(TextBlock(prompt))},
	})
	if err != nil {
		return "", fmt.Errorf("acp: prompt: %w", err)
	}
	return StopReason(resp.StopReason), nil
}

// Cancel sends a cancellation notification for the given session.
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	return c.conn.Cancel(ctx, sdk.CancelNotification{SessionId: sdk.SessionId(sessionID)})
}

// Kill terminates the agent subprocess WITHOUT reaping it. Prefer Close, which
// also waits. Kill is a no-op if no subprocess was launched (e.g. tests using
// pipe transport).
func (c *Client) Kill() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	return c.cmd.Process.Kill()
}

// Close terminates the agent subprocess and waits for it to exit, releasing
// its resources so it does not become a zombie. It is a no-op if no
// subprocess was launched. The error from Wait is returned (typically a
// non-nil "signal: killed" for a killed process), except that a nil-process
// client closes cleanly.
func (c *Client) Close() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	killErr := c.cmd.Process.Kill()
	waitErr := c.cmd.Wait()
	if killErr != nil {
		return killErr
	}
	return waitErr
}

// sdkAdapter bridges a bashy-owned Handler to the SDK's acp.Client interface.
// It handles the two callbacks a bashy client cares about (session updates and
// permission requests) and declines every filesystem and terminal callback —
// this client advertises no such capabilities, so a well-behaved agent never
// invokes them. These declining stubs, and the empty ClientCapabilities that
// justifies them, are the nine required acp.Client methods.
type sdkAdapter struct{ h Handler }

// Compile-time check that the adapter satisfies the SDK's Client interface.
var _ sdk.Client = sdkAdapter{}

func (a sdkAdapter) SessionUpdate(ctx context.Context, n sdk.SessionNotification) error {
	return a.h.SessionUpdate(ctx, fromSDKNotification(n))
}

func (a sdkAdapter) RequestPermission(ctx context.Context, p sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	resp, err := a.h.RequestPermission(ctx, fromSDKPermissionRequest(p))
	if err != nil {
		return sdk.RequestPermissionResponse{}, err
	}
	return toSDKPermissionResponse(resp), nil
}

func (sdkAdapter) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, fmt.Errorf("fs.readTextFile not offered")
}

func (sdkAdapter) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, fmt.Errorf("fs.writeTextFile not offered")
}

func (sdkAdapter) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, fmt.Errorf("terminal not offered")
}

func (sdkAdapter) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, fmt.Errorf("terminal not offered")
}

func (sdkAdapter) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, fmt.Errorf("terminal not offered")
}

func (sdkAdapter) ReleaseTerminal(context.Context, sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, fmt.Errorf("terminal not offered")
}

func (sdkAdapter) WaitForTerminalExit(context.Context, sdk.WaitForTerminalExitRequest) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, fmt.Errorf("terminal not offered")
}

// --- boundary conversions: SDK types <-> bashy-owned types ---

func toSDKContent(cb ContentBlock) sdk.ContentBlock {
	return sdk.ContentBlock{Text: &sdk.ContentBlockText{Text: cb.Text, Type: "text"}}
}

func fromSDKContent(c sdk.ContentBlock) ContentBlock {
	if c.Text != nil {
		return ContentBlock{Text: c.Text.Text}
	}
	return ContentBlock{}
}

func fromSDKNotification(n sdk.SessionNotification) SessionNotification {
	u := n.Update
	var out SessionUpdate
	switch {
	case u.AgentMessageChunk != nil:
		cb := fromSDKContent(u.AgentMessageChunk.Content)
		out.AgentMessageChunk = &cb
	case u.AgentThoughtChunk != nil:
		cb := fromSDKContent(u.AgentThoughtChunk.Content)
		out.AgentThoughtChunk = &cb
	case u.UserMessageChunk != nil:
		cb := fromSDKContent(u.UserMessageChunk.Content)
		out.UserMessageChunk = &cb
	case u.ToolCall != nil:
		tc := fromSDKToolCall(u.ToolCall)
		out.ToolCall = &tc
	case u.ToolCallUpdate != nil:
		tc := fromSDKSessionToolCallUpdate(u.ToolCallUpdate)
		out.ToolCallUpdate = &tc
	}
	return SessionNotification{SessionID: string(n.SessionId), Update: out}
}

func fromSDKToolCall(t *sdk.SessionUpdateToolCall) ToolCall {
	return ToolCall{
		ID:        string(t.ToolCallId),
		Title:     t.Title,
		Status:    ToolCallStatus(t.Status),
		Kind:      string(t.Kind),
		Locations: fromSDKLocations(t.Locations),
	}
}

func fromSDKSessionToolCallUpdate(t *sdk.SessionToolCallUpdate) ToolCall {
	tc := ToolCall{
		ID:        string(t.ToolCallId),
		Locations: fromSDKLocations(t.Locations),
	}
	if t.Status != nil {
		tc.Status = ToolCallStatus(*t.Status)
	}
	if t.Title != nil {
		tc.Title = *t.Title
	}
	if t.Kind != nil {
		tc.Kind = string(*t.Kind)
	}
	return tc
}

func fromSDKToolCallUpdate(t sdk.ToolCallUpdate) ToolCall {
	tc := ToolCall{
		ID:        string(t.ToolCallId),
		Locations: fromSDKLocations(t.Locations),
	}
	if t.Status != nil {
		tc.Status = ToolCallStatus(*t.Status)
	}
	if t.Title != nil {
		tc.Title = *t.Title
	}
	if t.Kind != nil {
		tc.Kind = string(*t.Kind)
	}
	return tc
}

func fromSDKLocations(in []sdk.ToolCallLocation) []ToolCallLocation {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCallLocation, len(in))
	for i, l := range in {
		out[i] = ToolCallLocation{Path: l.Path}
		if l.Line != nil {
			out[i].Line = *l.Line
		}
	}
	return out
}

func fromSDKPermissionRequest(p sdk.RequestPermissionRequest) PermissionRequest {
	req := PermissionRequest{
		SessionID: string(p.SessionId),
		ToolCall:  fromSDKToolCallUpdate(p.ToolCall),
	}
	for _, o := range p.Options {
		req.Options = append(req.Options, PermissionOption{
			ID:   string(o.OptionId),
			Name: o.Name,
			Kind: PermissionOptionKind(o.Kind),
		})
	}
	return req
}

func toSDKPermissionResponse(r PermissionResponse) sdk.RequestPermissionResponse {
	if r.Cancelled || r.OptionID == "" {
		return sdk.RequestPermissionResponse{
			Outcome: sdk.RequestPermissionOutcome{Cancelled: &sdk.RequestPermissionOutcomeCancelled{}},
		}
	}
	return sdk.RequestPermissionResponse{
		Outcome: sdk.RequestPermissionOutcome{
			Selected: &sdk.RequestPermissionOutcomeSelected{OptionId: sdk.PermissionOptionId(r.OptionID)},
		},
	}
}
