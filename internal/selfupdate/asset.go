package selfupdate

import (
	"fmt"
	"runtime"
	"strings"
)

// DefaultOwner and DefaultRepo identify the GitHub project that publishes
// Pine release archives.
const (
	DefaultOwner = "underworld14"
	DefaultRepo  = "pine"
)

// AssetName returns the goreleaser archive name for version/os/arch
// (e.g. pine_0.7.1_darwin_arm64.tar.gz).
func AssetName(version, goos, goarch string) (string, error) {
	v := NormalizeVersion(version)
	if v == "" {
		return "", fmt.Errorf("empty version")
	}
	osName, arch, err := mapPlatform(goos, goarch)
	if err != nil {
		return "", err
	}
	ext := ".tar.gz"
	if osName == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("pine_%s_%s_%s%s", v, osName, arch, ext), nil
}

// CurrentAssetName is AssetName for the running process's GOOS/GOARCH.
func CurrentAssetName(version string) (string, error) {
	return AssetName(version, runtime.GOOS, runtime.GOARCH)
}

func mapPlatform(goos, goarch string) (osName, arch string, err error) {
	switch strings.ToLower(goos) {
	case "darwin", "linux", "windows":
		osName = strings.ToLower(goos)
	default:
		return "", "", fmt.Errorf("unsupported OS %q", goos)
	}
	switch strings.ToLower(goarch) {
	case "amd64", "arm64":
		arch = strings.ToLower(goarch)
	default:
		return "", "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	return osName, arch, nil
}

// BinaryName is the executable name inside a release archive.
func BinaryName(goos string) string {
	if strings.ToLower(goos) == "windows" {
		return "pine.exe"
	}
	return "pine"
}
