//go:build !unix

package killcmd

import (
	"fmt"
	"os"
)

type nativeSignal int

func invalidSignal() nativeSignal { return -1 }
func signalByName(name string) nativeSignal {
	if name == "TERM" {
		return 15
	}
	if name == "0" || name == "EXIT" {
		return 0
	}
	return -1
}
func signalName(number int) (string, bool) {
	if number == 0 {
		return "EXIT", true
	}
	if number == 15 {
		return "TERM", true
	}
	return "", false
}
func signalNames() []string                    { return []string{"TERM"} }
func signalFromNumber(number int) nativeSignal { return nativeSignal(number) }
func signalNumber(sig nativeSignal) int        { return int(sig) }
func maxSignalNumber() int                     { return 15 }
func sendSignal(pid int, sig nativeSignal) error {
	if pid <= 0 {
		return fmt.Errorf("process groups are not supported on this platform")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if sig == 0 {
		return nil
	}
	return p.Signal(os.Kill)
}
