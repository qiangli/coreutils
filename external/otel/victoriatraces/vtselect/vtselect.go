// Package vtselect re-exports the VictoriaTraces vtselect subsystem.
package vtselect

import "github.com/VictoriaMetrics/VictoriaTraces/app/vtselect"

var (
	Init           = vtselect.Init
	Stop           = vtselect.Stop
	RequestHandler = vtselect.RequestHandler
)
