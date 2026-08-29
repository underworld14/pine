//go:build unix

package selfupdate

import (
	"errors"
	"syscall"
)

func isBusyRenameError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ETXTBSY) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ETXTBSY, syscall.EBUSY:
			return true
		}
	}
	return false
}
