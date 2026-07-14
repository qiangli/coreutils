package stack

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmselect/promql"

	"github.com/qiangli/coreutils/external/otel/victorialogs/httpserver"
	"github.com/qiangli/coreutils/external/otel/victoriametrics/vminsert"
	"github.com/qiangli/coreutils/external/otel/victoriametrics/vmselect"
	"github.com/qiangli/coreutils/external/otel/victoriametrics/vmstorage"
)

// VictoriaMetricsComponent runs VictoriaMetrics in-process, and REPLACES PROMETHEUS —
// which, in turn, is what lets the OTel COLLECTOR go.
//
// The chain mattered and it is worth stating, because it is why the collector could not
// simply be deleted on its own:
//
//	the collector received OTLP and handed metrics to a prometheusexporter
//	prometheus SCRAPED that exporter
//	so deleting the collector deleted the only path metrics had
//
// VictoriaMetrics ingests OTLP NATIVELY at /opentelemetry/v1/metrics. With it, every
// signal has a store that speaks OTLP directly, and the collector's entire remaining job
// is fan-out for callers who send all three signals to one address — 833 dependencies to
// save a caller from setting three environment variables.
//
//	prometheus     556 deps
//	collector      833 deps
//	victoria       ~113 (the same engine already storing logs and traces)
type VictoriaMetricsComponent struct {
	port       int
	dataDir    string
	pathPrefix string
	healthy    atomic.Bool
	cancel     context.CancelFunc
}

// NewVictoriaMetricsComponent creates an in-process VictoriaMetrics component.
func NewVictoriaMetricsComponent(port int, dataDir string) *VictoriaMetricsComponent {
	return &VictoriaMetricsComponent{port: port, dataDir: dataDir}
}

func (v *VictoriaMetricsComponent) Name() string           { return "victoria-metrics" }
func (v *VictoriaMetricsComponent) SetPathPrefix(p string) { v.pathPrefix = p }

func (v *VictoriaMetricsComponent) Start(ctx context.Context) error {
	// The VictoriaMetrics httpserver calls logger.Fatalf (→ os.Exit) on bind failure,
	// which would take the whole process with it. Check first.
	if !IsPortAvailable(v.port) {
		return fmt.Errorf("victoria-metrics: port %d already in use", v.port)
	}

	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	// All three Victoria components share ONE global flag set, so storageDataPath is set
	// immediately before the Init that reads it.
	_ = flag.Set("storageDataPath", v.dataDir+"/data")
	if v.pathPrefix != "" {
		_ = flag.Set("http.pathPrefix", v.pathPrefix)
	}
	if !flag.Parsed() {
		flag.CommandLine.Parse([]string{})
	}

	cleanStaleFlock(v.dataDir + "/data")

	// Same order and arguments as upstream app/victoria-metrics/main.go. The concurrency
	// and queue-duration values are upstream's own defaults; this is a single-node
	// developer stack, not a cluster.
	const maxConcurrentSelects = 16
	const maxSelectQueueDuration = 10 * time.Second

	vmstorage.Init(maxConcurrentSelects, promql.ResetRollupResultCacheIfNeeded)
	vmselect.Init(maxConcurrentSelects, maxSelectQueueDuration)
	vminsert.Init()

	listenAddrs := []string{fmt.Sprintf("127.0.0.1:%d", v.port)}
	go httpserver.Serve(listenAddrs, v.requestHandler, httpserver.ServeOptions{})

	v.healthy.Store(true)
	slog.Info("victoria-metrics: started", "port", v.port, "dataDir", v.dataDir)

	go func() {
		<-ctx.Done()
		_ = httpserver.Stop(listenAddrs)
		vminsert.Stop()
		vmstorage.Stop()
		vmselect.Stop()
		v.healthy.Store(false)
		slog.Info("victoria-metrics: stopped")
	}()

	return nil
}

func (v *VictoriaMetricsComponent) Stop(_ context.Context) error {
	if v.cancel != nil {
		v.cancel()
	}
	return nil
}

func (v *VictoriaMetricsComponent) Healthy() bool             { return v.healthy.Load() }
func (v *VictoriaMetricsComponent) HTTPHandler() http.Handler { return nil }
func (v *VictoriaMetricsComponent) Port() int                 { return v.port }

// requestHandler delegates to the VictoriaMetrics subsystems.
//
// vminsert serves /opentelemetry/v1/metrics natively; vmselect serves PromQL and vmui.
func (v *VictoriaMetricsComponent) requestHandler(w http.ResponseWriter, r *http.Request) bool {
	if vminsert.RequestHandler(w, r) {
		return true
	}
	if vmselect.RequestHandler(w, r) {
		return true
	}
	if vmstorage.RequestHandler(w, r) {
		return true
	}
	if r.URL.Path == "/" || r.URL.Path == "" {
		target := "/vmui/"
		if v.pathPrefix != "" {
			target = v.pathPrefix + target
		}
		http.Redirect(w, r, target, http.StatusFound)
		return true
	}
	return false
}
