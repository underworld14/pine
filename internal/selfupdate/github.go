package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultAPI = "https://api.github.com"

// Client talks to the GitHub Releases API (or a test stand-in).
type Client struct {
	HTTP    *http.Client
	APIBase string // e.g. https://api.github.com — overridable in tests
	Owner   string
	Repo    string
	Token   string // optional; from GITHUB_TOKEN / GH_TOKEN
	Version string // pine version for User-Agent
}

// NewClient builds a Client with defaults from the environment.
func NewClient(pineVersion string) *Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &Client{
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		APIBase: defaultAPI,
		Owner:   DefaultOwner,
		Repo:    DefaultRepo,
		Token:   token,
		Version: pineVersion,
	}
}

// Release is a subset of the GitHub release JSON we need.
type Release struct {
	TagName string `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a release downloadable file.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestRelease fetches /repos/{owner}/{repo}/releases/latest.
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", strings.TrimRight(c.apiBase(), "/"), c.owner(), c.repo())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("latest release has no tag_name")
	}
	return &rel, nil
}

// Download fetches url into memory (release archives are small).
func (c *Client) Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("download %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	// Cap at 256 MiB — release binaries are far smaller.
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

func (c *Client) setHeaders(req *http.Request) {
	ua := "pine"
	if c.Version != "" {
		ua = "pine/" + c.Version
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) apiBase() string {
	if c.APIBase != "" {
		return c.APIBase
	}
	return defaultAPI
}

func (c *Client) owner() string {
	if c.Owner != "" {
		return c.Owner
	}
	return DefaultOwner
}

func (c *Client) repo() string {
	if c.Repo != "" {
		return c.Repo
	}
	return DefaultRepo
}

// FindAssetURL returns the browser download URL for name, or "".
func (r *Release) FindAssetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}
