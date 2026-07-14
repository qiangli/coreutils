// Package vtstorage re-exports the VictoriaTraces storage subsystem.
package vtstorage

import "github.com/VictoriaMetrics/VictoriaTraces/app/vtstorage"

// Storage is the trace store. insertutil.SetLogRowsStorage takes it — VictoriaTraces
// reuses VictoriaLogs' logstorage engine, which is why the two components share an
// insertutil and why VictoriaTraces was blocked on the VictoriaLogs fork.
type Storage = vtstorage.Storage

var (
	Init           = vtstorage.Init
	Stop           = vtstorage.Stop
	RequestHandler = vtstorage.RequestHandler
)
