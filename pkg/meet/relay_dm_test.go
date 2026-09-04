package meet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/chat"
)

func TestRelayDMHTTPUsesChatAndPersistsTranscript(t *testing.T) {
	t.Setenv("BASHY_MEET_DIR", t.TempDir())
	t.Setenv("USER", "tester")
	pinFleet(t)
	old := invokeRelayDM
	invokeRelayDM = func(context.Context, chat.Options, chat.Runner) (chat.Result, error) {
		return chat.Result{Output: "private reply"}, nil
	}
	t.Cleanup(func() { invokeRelayDM = old })

	h := newServeHandler(context.Background(), MountOptions{})
	post := httptest.NewRequest(http.MethodPost, "/api/dms", strings.NewReader(`{"Agent":"codex"}`))
	post.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/dms/codex", nil)
	get.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", w.Code, w.Body.String())
	}
	var detail relayDMDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.State.Agent != "codex" || detail.State.Human != "tester" {
		t.Fatalf("state=%+v", detail.State)
	}
}

// A CHAT MESSAGE IS WITHDRAWN, NOT UNSENT — and the record it withdraws stays.
//
// The 1:1 is the surface where a real cancel is least available: it appends the
// human's line INSIDE the send request (that is what makes the reply
// streamable), so by the time any recall can arrive the message exists. The
// honest answer is therefore a retraction, and the send has to hand back the
// handle that makes one possible.
func TestRelayDMRecallRetractsTheMessageItNamed(t *testing.T) {
	t.Setenv("BASHY_MEET_DIR", t.TempDir())
	t.Setenv("USER", "tester")
	pinFleet(t)
	old := invokeRelayDM
	invokeRelayDM = func(ctx context.Context, _ chat.Options, _ chat.Runner) (chat.Result, error) {
		// Hold the turn open the way a thinking agent does, so the recall lands
		// while the message is genuinely in flight rather than after everything
		// has settled.
		<-ctx.Done()
		return chat.Result{}, ctx.Err()
	}
	t.Cleanup(func() { invokeRelayDM = old })

	h := newServeHandler(context.Background(), MountOptions{})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	if w := call(http.MethodPost, "/api/dms", `{"Agent":"codex"}`); w.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	w := call(http.MethodPost, "/api/dms/codex/messages", `{"text":"forget I asked"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("send status=%d body=%s", w.Code, w.Body.String())
	}
	var sent struct{ TS string }
	if err := json.Unmarshal(w.Body.Bytes(), &sent); err != nil {
		t.Fatal(err)
	}
	// THE HANDLE IS THE WHOLE POINT of the send response here. Without it the
	// browser has nothing to name, and "cancel" in a chat could only ever be a
	// button that reported success without doing anything.
	if sent.TS == "" {
		t.Fatal("the send answered with no record timestamp; nothing can be recalled")
	}

	w = call(http.MethodPost, "/api/dms/codex/recall", `{"ts":"`+sent.TS+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("recall status=%d body=%s", w.Code, w.Body.String())
	}
	var out RecallResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Verdict != RecallRetracted {
		t.Fatalf("verdict = %q, want %q", out.Verdict, RecallRetracted)
	}
	if out.Retracted != sent.TS {
		t.Errorf("retracted %q, want the record that was sent (%q)", out.Retracted, sent.TS)
	}

	// The chat's own projection must SHOW the withdrawal. A chat flattens every
	// record onto user/assistant, and under that projection a retraction would
	// arrive as an ordinary message from the human while the withdrawn line
	// still read as live — in the surface a person is most likely to be reading.
	get := call(http.MethodGet, "/api/dms/codex", "")
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var detail relayDMDetail
	if err := json.Unmarshal(get.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	var withdrawn, retraction *relayDMEvent
	for i := range detail.Events {
		switch {
		case detail.Events[i].Kind == "retraction":
			retraction = &detail.Events[i]
		case detail.Events[i].Text == "forget I asked":
			withdrawn = &detail.Events[i]
		}
	}
	if withdrawn == nil {
		t.Error("the withdrawn message is gone from the chat; it must stay and be marked")
	}
	if retraction == nil {
		t.Fatal("the chat's projection carries no retraction")
	}
	if retraction.Retracts != sent.TS {
		t.Errorf("retraction points at %q, want %q", retraction.Retracts, sent.TS)
	}
}
