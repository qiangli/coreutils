package batchcmd

import (
	"fmt"
	"strings"
	"time"
)

func batchLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.Local, nil
	}
	name := strings.TrimPrefix(tz, ":")
	switch strings.ToUpper(name) {
	case "UTC", "UTC0", "GMT", "GMT0":
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid TZ %q", tz)
	}
	return loc, nil
}
