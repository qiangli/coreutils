package acp

import (
	"context"
	"io"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
)

// fakeAgent is a minimal SDK-side agent driven by per-test callbacks. It lives
// in the internal test package because it necessarily speaks SDK types; the
// public API guard is in api_external_test.go (package acp_test).
type fakeAgent struct {
	asc        *sdk.AgentSideConnection
	initialize func(sdk.InitializeRequest) (sdk.InitializeResponse, error)
	newSession func(sdk.NewSessionRequest) (sdk.NewSessionResponse, error)
	prompt     func(*fakeAgent, sdk.PromptRequest) (sdk.PromptResponse, error)
}

func (a *fakeAgent) Initialize(_ context.Context, r sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return a.initialize(r)
}
func (a *fakeAgent) NewSession(_ context.Context, r sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return a.newSession(r)
}
func (a *fakeAgent) Prompt(_ context.Context, r sdk.PromptRequest) (sdk.PromptResponse, error) {
	return a.prompt(a, r)
}
func (a *fakeAgent) Cancel(context.Context, sdk.CancelNotification) error { return nil }
func (a *fakeAgent) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, nil
}
func (a *fakeAgent) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, nil
}
func (a *fakeAgent) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, nil
}
func (a *fakeAgent) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, nil
}
func (a *fakeAgent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, nil
}
func (a *fakeAgent) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, nil
}
func (a *fakeAgent) SetSessionMode(context.Context, sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, nil
}

func newPipeClient(t *testing.T, handler Handler, agent *fakeAgent) *Client {
	t.Helper()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()

	agent.asc = sdk.NewAgentSideConnection(agent, a2cW, c2aR)
	conn := sdk.NewClientSideConnection(sdkAdapter{h: handler}, c2aW, a2cR)

	initResp, err := conn.Initialize(context.Background(), sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return &Client{conn: conn, protocolVersion: int(initResp.ProtocolVersion)}
}

func defaultInit(r sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{ProtocolVersion: r.ProtocolVersion}, nil
}

func TestNewClient_InitializeAndNewSession(t *testing.T) {
	agent := &fakeAgent{
		initialize: func(r sdk.InitializeRequest) (sdk.InitializeResponse, error) {
			if r.ProtocolVersion != sdk.ProtocolVersionNumber {
				t.Errorf("protocol version = %d, want %d", r.ProtocolVersion, sdk.ProtocolVersionNumber)
			}
			return sdk.InitializeResponse{ProtocolVersion: r.ProtocolVersion}, nil
		},
		newSession: func(r sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
			if r.Cwd != "/tmp/test" {
				t.Errorf("cwd = %q, want /tmp/test", r.Cwd)
			}
			if r.McpServers == nil {
				t.Error("McpServers is nil")
			}
			return sdk.NewSessionResponse{SessionId: "sess-001"}, nil
		},
		prompt: func(_ *fakeAgent, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
			return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
		},
	}

	client := newPipeClient(t, &RecordingHandler{}, agent)
	defer client.Close()

	if client.ProtocolVersion() != ProtocolVersionNumber {
		t.Errorf("ProtocolVersion() = %d, want %d", client.ProtocolVersion(), ProtocolVersionNumber)
	}

	sessionID, err := client.NewSession(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sessionID != "sess-001" {
		t.Errorf("sessionID = %q, want sess-001", sessionID)
	}
}

func TestPrompt_StopReasons(t *testing.T) {
	// A cancelled turn must be a SUCCESSFUL response carrying the stop reason,
	// never an error.
	for _, tc := range []struct {
		name string
		sr   sdk.StopReason
		want StopReason
	}{
		{"end_turn", sdk.StopReasonEndTurn, StopReasonEndTurn},
		{"cancelled", sdk.StopReasonCancelled, StopReasonCancelled},
		{"refusal", sdk.StopReasonRefusal, StopReasonRefusal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := &fakeAgent{
				initialize: defaultInit,
				newSession: func(sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
					return sdk.NewSessionResponse{SessionId: "s"}, nil
				},
				prompt: func(_ *fakeAgent, r sdk.PromptRequest) (sdk.PromptResponse, error) {
					if len(r.Prompt) != 1 {
						t.Errorf("prompt blocks = %d, want 1", len(r.Prompt))
					}
					return sdk.PromptResponse{StopReason: tc.sr}, nil
				},
			}
			client := newPipeClient(t, &RecordingHandler{}, agent)
			defer client.Close()

			sid, err := client.NewSession(context.Background(), "/tmp/test")
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			got, err := client.Prompt(context.Background(), sid, "hello")
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}
			if got != tc.want {
				t.Errorf("stopReason = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrompt_StreamsThroughRecorder exercises the SDK->bashy boundary: the
// agent streams a text chunk and a tool call with locations, and the
// RecordingHandler captures them as bashy-owned values.
func TestPrompt_StreamsThroughRecorder(t *testing.T) {
	agent := &fakeAgent{
		initialize: defaultInit,
		newSession: func(sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
			return sdk.NewSessionResponse{SessionId: "s-stream"}, nil
		},
		prompt: func(a *fakeAgent, r sdk.PromptRequest) (sdk.PromptResponse, error) {
			ctx := context.Background()
			_ = a.asc.SessionUpdate(ctx, sdk.SessionNotification{
				SessionId: r.SessionId,
				Update: sdk.SessionUpdate{AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{
					Content: sdk.ContentBlock{Text: &sdk.ContentBlockText{Text: "working ", Type: "text"}},
				}},
			})
			status := sdk.ToolCallStatusInProgress
			_ = a.asc.SessionUpdate(ctx, sdk.SessionNotification{
				SessionId: r.SessionId,
				Update: sdk.SessionUpdate{ToolCall: &sdk.SessionUpdateToolCall{
					ToolCallId: "tc-1",
					Title:      "edit",
					Status:     status,
					Locations: []sdk.ToolCallLocation{
						{Path: "src/main.go"},
						{Path: "src/utils.go"},
					},
				}},
			})
			return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
		},
	}

	rec := &RecordingHandler{}
	client := newPipeClient(t, rec, agent)
	defer client.Close()

	sid, _ := client.NewSession(context.Background(), "/tmp/test")
	if _, err := client.Prompt(context.Background(), sid, "go"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if got := rec.AgentText(); got != "working " {
		t.Errorf("AgentText() = %q, want %q", got, "working ")
	}
	paths := rec.TouchedPaths()
	if len(paths) != 2 || paths[0] != "src/main.go" || paths[1] != "src/utils.go" {
		t.Errorf("TouchedPaths() = %v, want [src/main.go src/utils.go]", paths)
	}
	events := rec.ToolCallEvents()
	if len(events) != 1 || events[0].Status != ToolCallInProgress || events[0].ID != "tc-1" {
		t.Errorf("ToolCallEvents() = %+v, want one in_progress tc-1", events)
	}
}
