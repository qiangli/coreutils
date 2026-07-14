// Package vmselect re-exports the VictoriaMetrics query subsystem.
package vmselect

import "github.com/VictoriaMetrics/VictoriaMetrics/app/vmselect"

var (
	Init           = vmselect.Init
	Stop           = vmselect.Stop
	RequestHandler = vmselect.RequestHandler
)
