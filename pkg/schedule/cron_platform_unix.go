//go:build !windows

package schedule

func validateJobPlatform(*Job) error { return nil }
