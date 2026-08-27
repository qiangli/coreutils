//go:build !unix

package edcmd

type edSignals struct{}

func startEdSignals() *edSignals          { return &edSignals{} }
func (*edSignals) stop()                  {}
func (*edSignals) poll() string           { return "" }
func (*edSignals) channel() <-chan string { return nil }
