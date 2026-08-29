package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ReplaceExecutable writes newBinary over path. On Unix it prefers an atomic
// rename-over of the target (safe while the process is still running); on
// busy/ETXTBSY or on Windows it falls back to move-aside then replace.
func ReplaceExecutable(path string, newBinary []byte) error {
	if path == "" {
		return fmt.Errorf("empty executable path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("stat %s: %w", abs, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", abs)
	}

	dir := filepath.Dir(abs)
	tmp, err := os.CreateTemp(dir, ".pine-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp in %s (is it writable?): %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(newBinary); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp binary: %w", err)
	}
	if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	// Durability: flush before we promote the file into place.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Rename(tmpPath, abs); err == nil {
			cleanup = false
			_ = os.Chmod(abs, fi.Mode().Perm()|0o111)
			return nil
		} else if !isBusyRenameError(err) {
			return fmt.Errorf("install new binary: %w", err)
		}
		// Fall through to move-aside on ETXTBSY / equivalent.
	}

	if err := replaceViaMoveAside(abs, tmpPath); err != nil {
		return err
	}
	cleanup = false
	if runtime.GOOS != "windows" {
		_ = os.Chmod(abs, fi.Mode().Perm()|0o111)
	}
	return nil
}

func replaceViaMoveAside(abs, tmpPath string) error {
	backup := abs + ".old"
	_ = os.Remove(backup) // leftover from a previous upgrade

	if err := os.Rename(abs, backup); err != nil {
		return fmt.Errorf("move aside current binary: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		_ = os.Rename(backup, abs)
		return fmt.Errorf("install new binary: %w", err)
	}
	// On Windows the running process may still lock .old until exit — ignore.
	_ = os.Remove(backup)
	return nil
}

// ExecutablePath resolves the path of the running pine binary.
func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
