// Package vmstorage re-exports the VictoriaMetrics storage subsystem.
package vmstorage

import "github.com/VictoriaMetrics/VictoriaMetrics/app/vmstorage"

var (
	Init           = vmstorage.Init
	Stop           = vmstorage.Stop
	RequestHandler = vmstorage.RequestHandler
)
