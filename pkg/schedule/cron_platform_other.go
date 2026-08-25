//go:build !unix

package schedule

import "errors"

func validateJobPlatform(j *Job) error {
	if j.POSIXCron {
		return errors.New("POSIX crontab execution is unsupported on this non-Unix host: shell and umask semantics cannot be guaranteed")
	}
	if j.Kind == "at" {
		return errors.New("POSIX at/batch execution is unsupported on this non-Unix host: session, process-group, controlling-terminal, and umask semantics cannot be guaranteed")
	}
	return nil
}
