package selfupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Result is the outcome of a check or upgrade.
type Result struct {
	Current     string
	Latest      string
	Asset       string
	NeedsUpdate bool
	Updated     bool
	InstallHint string // human note about how this binary looks installed
}

// Options control Check / Upgrade.
type Options struct {
	CurrentVersion string
	Force          bool
	// Executable overrides os.Executable (tests).
	Executable string
	// GOOS/GOARCH override runtime (tests).
	GOOS   string
	GOARCH string
	Client *Client
}

// Check reports whether a newer release is available (no download of the binary).
func Check(ctx context.Context, opt Options) (*Result, error) {
	res, _, err := plan(ctx, opt)
	return res, err
}

// Upgrade downloads and installs the latest release when needed (or when Force).
func Upgrade(ctx context.Context, opt Options) (*Result, error) {
	res, rel, err := plan(ctx, opt)
	if err != nil {
		return res, err
	}
	if !res.NeedsUpdate && !opt.Force {
		return res, nil
	}

	c := opt.client()
	assetURL := rel.FindAssetURL(res.Asset)
	if assetURL == "" {
		return res, fmt.Errorf("release %s has no asset %s", res.Latest, res.Asset)
	}
	checksumURL := rel.FindAssetURL("checksums.txt")
	if checksumURL == "" {
		return res, fmt.Errorf("release %s has no checksums.txt", res.Latest)
	}

	sumBody, err := c.Download(ctx, checksumURL)
	if err != nil {
		return res, fmt.Errorf("download checksums: %w", err)
	}
	wantSum, err := ChecksumFor(string(sumBody), res.Asset)
	if err != nil {
		return res, err
	}

	archive, err := c.Download(ctx, assetURL)
	if err != nil {
		return res, fmt.Errorf("download %s: %w", res.Asset, err)
	}
	if err := VerifySHA256(archive, wantSum); err != nil {
		return res, err
	}

	bin, err := ExtractBinary(archive, res.Asset, opt.goos())
	if err != nil {
		return res, err
	}

	exe, err := opt.executable()
	if err != nil {
		return res, err
	}
	if err := ReplaceExecutable(exe, bin); err != nil {
		return res, err
	}
	res.Updated = true
	res.NeedsUpdate = true
	return res, nil
}

func plan(ctx context.Context, opt Options) (*Result, *Release, error) {
	c := opt.client()
	rel, err := c.LatestRelease(ctx)
	if err != nil {
		return nil, nil, err
	}
	latest := NormalizeVersion(rel.TagName)
	asset, err := AssetName(latest, opt.goos(), opt.goarch())
	if err != nil {
		return nil, rel, err
	}
	cur := opt.CurrentVersion
	res := &Result{
		Current:     cur,
		Latest:      latest,
		Asset:       asset,
		NeedsUpdate: NeedsUpgrade(cur, latest),
		InstallHint: installHint(opt),
	}
	if rel.FindAssetURL(asset) == "" {
		return res, rel, fmt.Errorf("release %s has no asset %s for this platform", latest, asset)
	}
	return res, rel, nil
}

func (o Options) client() *Client {
	if o.Client != nil {
		return o.Client
	}
	return NewClient(o.CurrentVersion)
}

func (o Options) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o Options) goarch() string {
	if o.GOARCH != "" {
		return o.GOARCH
	}
	return runtime.GOARCH
}

func (o Options) executable() (string, error) {
	if o.Executable != "" {
		return o.Executable, nil
	}
	return ExecutablePath()
}

func installHint(opt Options) string {
	exe, err := opt.executable()
	if err != nil {
		return ""
	}
	dir := filepath.Clean(filepath.Dir(exe))
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	gobin := os.Getenv("GOBIN")
	candidates := []string{}
	if gobin != "" {
		candidates = append(candidates, filepath.Clean(gobin))
	}
	if gopath != "" {
		candidates = append(candidates, filepath.Clean(filepath.Join(gopath, "bin")))
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		candidates = append(candidates, filepath.Clean(filepath.Join(home, "go", "bin")))
	}
	for _, c := range candidates {
		if c != "" && c == dir {
			return "installed via go (GOBIN/GOPATH/bin); upgrading to the official release binary"
		}
	}
	if strings.Contains(dir, string(filepath.Separator)+"go"+string(filepath.Separator)+"bin") {
		return "looks like a Go bin install; upgrading to the official release binary"
	}
	return "release/binary install"
}
