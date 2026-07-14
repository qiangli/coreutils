// Package vtinsert re-exports the VictoriaTraces vtinsert subsystem.
package vtinsert

import "github.com/VictoriaMetrics/VictoriaTraces/app/vtinsert"

var (
	Init           = vtinsert.Init
	Stop           = vtinsert.Stop
	RequestHandler = vtinsert.RequestHandler
)
