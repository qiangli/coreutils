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

	h := newServeHandler(context.Background())
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
