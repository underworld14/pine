//go:build !unix

package selfupdate

func isBusyRenameError(err error) bool {
	return false
}
