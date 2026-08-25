//go:build windows

package schedule

import "errors"

func validateJobPlatform(j *Job) error {
	if j.POSIXCron {
		return errors.New("POSIX crontab execution is unsupported on Windows: shell and umask semantics cannot be guaranteed")
	}
	return nil
}
