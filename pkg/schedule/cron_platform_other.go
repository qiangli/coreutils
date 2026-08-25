//go:build !unix

package schedule

import "errors"

func validateJobPlatform(j *Job) error {
	if j.POSIXCron {
		return errors.New("POSIX crontab execution is unsupported on this non-Unix host: shell and umask semantics cannot be guaranteed")
	}
	return nil
}
