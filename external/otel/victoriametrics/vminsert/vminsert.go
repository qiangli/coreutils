// Package vminsert re-exports the VictoriaMetrics ingest subsystem.
//
// It serves /opentelemetry/v1/metrics NATIVELY — which is what lets the OTel Collector go:
// the collector's only remaining job was to receive OTLP and hand metrics to Prometheus.
package vminsert

import "github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert"

var (
	Init           = vminsert.Init
	Stop           = vminsert.Stop
	RequestHandler = vminsert.RequestHandler
)
