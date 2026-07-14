package stack

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/qiangli/coreutils/external/otel/victorialogs/httpserver"
	"github.com/qiangli/coreutils/external/otel/victorialogs/insertutil"
	"github.com/qiangli/coreutils/external/otel/victoriatraces/vtinsert"
	"github.com/qiangli/coreutils/external/otel/victoriatraces/vtselect"
	"github.com/qiangli/coreutils/external/otel/victoriatraces/vtstorage"
)

// VictoriaTracesComponent runs VictoriaTraces in-process, and REPLACES JAEGER.
//
// The numbers, measured before anything was cut:
//
//	jaeger        2,240 transitive dependencies
//	victorialogs    113
//
// Twenty times the weight for the same job — storing telemetry and serving a UI over it.
// VictoriaTraces is the same engine as VictoriaLogs (it literally reuses
// logstorage), ships its own vmui, and INGESTS OTLP NATIVELY — which is what lets the
// OTel Collector go too, since the collector existed only to fan OTLP out to Jaeger and
// Prometheus.
//
// This was blocked for one reason, and it was not a technical one: go.mod carried
// `replace VictoriaLogs => qiangli/VictoriaLogs`, and VictoriaTraces would not compile
// against that fork. The fork turned out to have ZERO commits of its own — a pure mirror,
// pinned three months stale, patching nothing. It existed only because VictoriaLogs'
// released tags (v1.113–v1.121) point at commits whose go.mod still declared the
// VictoriaMetrics module path, so the tags are unusable and only a pseudo-version works.
// Requiring upstream BY COMMIT does the same job with no fork at all.
//
// A dependency you cannot explain is a dependency you cannot maintain.
type VictoriaTracesComponent struct {
	port       int
	dataDir    string
	pathPrefix string
	healthy    atomic.Bool
	cancel     context.CancelFunc
}

// NewVictoriaTracesComponent creates an in-process VictoriaTraces component.
func NewVictoriaTracesComponent(port int, dataDir string) *VictoriaTracesComponent {
	return &VictoriaTracesComponent{port: port, dataDir: dataDir}
}

func (v *VictoriaTracesComponent) Name() string           { return "victoria-traces" }
func (v *VictoriaTracesComponent) SetPathPrefix(p string) { v.pathPrefix = p }

func (v *VictoriaTracesComponent) Start(ctx context.Context) error {
	// Check the port BEFORE starting: the VictoriaMetrics httpserver calls logger.Fatalf
	// (→ os.Exit) on bind failure, which would take the whole process with it.
	if !IsPortAvailable(v.port) {
		return fmt.Errorf("victoria-traces: port %d already in use", v.port)
	}

	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	// VictoriaTraces shares the VictoriaMetrics flag machinery with VictoriaLogs, and
	// they share the SAME global flag set — so storageDataPath is set per-component right
	// before Init reads it, and must not be set again afterwards.
	_ = flag.Set("storageDataPath", v.dataDir+"/data")
	if v.pathPrefix != "" {
		_ = flag.Set("http.pathPrefix", v.pathPrefix)
	}
	if !flag.Parsed() {
		flag.CommandLine.Parse([]string{})
	}

	cleanStaleFlock(v.dataDir + "/data")

	// Same init order as upstream's app/victoria-traces/main.go. It matters: vtinsert
	// needs the storage registered before it starts accepting writes.
	vtstorage.Init()
	vtselect.Init()
	insertutil.SetLogRowsStorage(&vtstorage.Storage{})
	vtinsert.Init()

	listenAddrs := []string{fmt.Sprintf("127.0.0.1:%d", v.port)}
	go httpserver.Serve(listenAddrs, v.requestHandler, httpserver.ServeOptions{})

	v.healthy.Store(true)
	slog.Info("victoria-traces: started", "port", v.port, "dataDir", v.dataDir)

	go func() {
		<-ctx.Done()
		_ = httpserver.Stop(listenAddrs)
		vtinsert.Stop()
		vtselect.Stop()
		vtstorage.Stop()
		v.healthy.Store(false)
		slog.Info("victoria-traces: stopped")
	}()

	return nil
}

func (v *VictoriaTracesComponent) Stop(_ context.Context) error {
	if v.cancel != nil {
		v.cancel()
	}
	return nil
}

func (v *VictoriaTracesComponent) Healthy() bool          { return v.healthy.Load() }
func (v *VictoriaTracesComponent) HTTPHandler() http.Handler { return nil }
func (v *VictoriaTracesComponent) Port() int              { return v.port }

// requestHandler delegates to the VictoriaTraces subsystems.
//
// vtinsert serves the OTLP ingest path (/insert/opentelemetry/v1/traces) NATIVELY — no
// collector in front of it. vtselect serves the query API and the built-in vmui.
func (v *VictoriaTracesComponent) requestHandler(w http.ResponseWriter, r *http.Request) bool {
	if vtinsert.RequestHandler(w, r) {
		return true
	}
	if vtselect.RequestHandler(w, r) {
		return true
	}
	if vtstorage.RequestHandler(w, r) {
		return true
	}
	if r.URL.Path == "/" || r.URL.Path == "" {
		target := "/select/vmui/"
		if v.pathPrefix != "" {
			target = v.pathPrefix + target
		}
		http.Redirect(w, r, target, http.StatusFound)
		return true
	}
	return false
}
