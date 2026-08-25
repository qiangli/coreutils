//go:build windows

package sttycmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

const posixVDisable = byte(0xff)

func validateMode(mode string) error { return fmt.Errorf("%s is not supported on this platform", mode) }
func validateSpeed(baud uint64) error {
	return fmt.Errorf("terminal speed %d is not supported on this platform", baud)
}
func validateControlChar(name string, ccLen int) error {
	return fmt.Errorf("control character %s is not supported on this platform", name)
}

func applyMode(fd int, mode string) error {
	return fmt.Errorf("%s is not supported on this platform", mode)
}

func applyValue(fd int, name string, value uint8) error {
	return fmt.Errorf("%s is not supported on this platform", name)
}

func applyWindowSize(fd int, rows, cols int) error {
	return fmt.Errorf("window size is not supported on this platform")
}

func getTerminalState(fd int) (*terminalState, error) {
	return nil, fmt.Errorf("terminal settings are not supported on this platform")
}

func setTerminalState(fd int, state *terminalState) error {
	return fmt.Errorf("terminal settings are not supported on this platform")
}

func applySpeed(fd int, which string, baud uint64) error {
	return fmt.Errorf("terminal speeds are not supported on this platform")
}

func applyControlChar(fd int, name string, value uint8) error {
	return fmt.Errorf("control characters are not supported on this platform")
}

func outputBaud(state *terminalState) uint64 { return 0 }

func printReadableSettings(rc *tool.RunContext, state *terminalState, rows, cols int, all bool) {}
