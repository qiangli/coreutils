package meet

import (
	"context"
	"net/http"
)

// Handler is the web room as an http.Handler: the embedded SPA at /, the JSON
// API under /api/, and the /observe WebSocket — the same mux `meet serve`
// listens with.
//
// It exists so a host other than the cobra command can serve the room. The
// first such host is the browser e2e harness (web/e2e), which must serve THIS
// checkout's SPA rather than whatever binary happens to be installed —
// otherwise the tests would silently pass against someone else's build. An
// embedder (outpost, a desktop shell) is the same shape.
//
// The SPA is only present when built with -tags meetspa; without it the
// handler still serves the API and explains the missing UI at /.
func Handler(ctx context.Context) http.Handler {
	return newServeHandler(ctx)
}
