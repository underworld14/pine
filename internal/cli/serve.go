package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/server"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/workspace"
)

const defaultPort = 3412

func newServeCmd() *cobra.Command {
	var (
		port  int
		host  string
		open  bool
		dev   bool
		only  bool // serve only the cwd repo, ignoring the registry
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Pine web UI and API on localhost",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, port, host, open, dev, only)
		},
	}
	f := cmd.Flags()
	f.IntVar(&port, "port", defaultPort, "port to listen on")
	f.StringVar(&host, "host", "127.0.0.1", "host to bind (localhost only by design)")
	f.BoolVar(&open, "open", true, "open the default browser after starting (use --open=false to skip)")
	f.BoolVar(&dev, "dev", false, "proxy non-API requests to the Vite dev server (localhost:5173)")
	f.BoolVar(&only, "only", false, "serve only the current repo, ignoring other repos registered in ~/.pine")
	return cmd
}

func runServe(cmd *cobra.Command, port int, host string, open, dev, only bool) error {
	reg, err := buildServeRegistry(only)
	if err != nil {
		return err
	}
	srv := server.NewWithRegistry(reg, version)

	var label string
	if reg.IsWorkspace() {
		label = fmt.Sprintf("%d repos", len(reg.All()))
	} else {
		label = fmt.Sprintf("%q", reg.Active().Config().Project.Name)
	}

	var handler http.Handler = srv.Handler()
	if dev {
		handler = devProxy(handler)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot bind %s: %w — is Pine already running? try --port", addr, err)
	}
	uiURL := "http://" + addr

	closeSync := srv.StartLiveSync()
	defer closeSync()

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Pine serving %s on %s\n", label, uiURL)
	if reg.IsWorkspace() {
		active := reg.ActiveAlias()
		for _, e := range srv.Registry().Entries() {
			marker := ""
			if e["alias"] == active {
				marker = "  (active)"
			}
			fmt.Fprintf(out, "  · %-12s %s%s\n", e["alias"], e["repo"], marker)
		}
		fmt.Fprintln(out, "  switch repos from the dashboard's repo switcher")
	}
	if dev {
		fmt.Fprintln(out, "dev mode: proxying UI to http://localhost:5173")
	}
	fmt.Fprintln(out, "Press Ctrl+C to stop.")

	httpServer := &http.Server{Handler: handler}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()

	if open {
		_ = browser.OpenURL(uiURL)
	}

	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		fmt.Fprintln(out, "\nShutting down…")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutCtx)
	}
}

// buildServeRegistry assembles the store registry for `pine serve`.
//
// By default every repo registered in ~/.pine/repos.json (auto-populated by
// `pine init`) is opened, with the cwd repo as active. `--only` restricts
// serving to just the cwd repo and is read-only on the registry (it never
// writes ~/.pine/repos.json). When no repo is registered and the cwd has no
// .pine, this falls back to the legacy single-repo openStore error path.
func buildServeRegistry(only bool) (*server.StoreRegistry, error) {
	if only {
		st, err := openStore()
		if err != nil {
			return nil, err
		}
		return server.SingleRegistry(st), nil
	}

	// Auto-register the cwd repo if it has a .pine (covers repos that were
	// `pine init`'d before auto-registration existed, or whose ~/.pine was wiped).
	// Skip the write when it's already registered — serve should be read-only on
	// the registry in the common case. (`--only` never reaches here.)
	if pineDir, err := findPineDir(flagDir); err == nil {
		cwdRoot := filepath.Dir(pineDir)
		if reg, rerr := workspace.Load(); rerr == nil && reg.Find(cwdRoot) == nil {
			if _, werr := workspace.RegisterPath(cwdRoot, ""); werr != nil {
				fmt.Fprintf(os.Stderr, "pine: could not auto-register cwd repo: %v\n", werr)
			}
		}
	}

	repos, err := workspace.ListAll()
	if err != nil {
		return nil, err
	}

	ordered := []string{}
	stores := map[string]*store.Store{}
	paths := map[string]string{}
	for _, re := range repos {
		pineDir, err := workspace.ResolvePineDir(re.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pine: skipping %s (%s): %v\n", re.Alias, re.Path, err)
			continue
		}
		st, err := store.Open(pineDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pine: skipping %s (%s): %v\n", re.Alias, re.Path, err)
			continue
		}
		ordered = append(ordered, re.Alias)
		stores[re.Alias] = st
		paths[re.Alias] = re.Path
	}

	if len(ordered) == 0 {
		// No registered repos resolve; fall back to the cwd store (errors if absent).
		st, err := openStore()
		if err != nil {
			return nil, err
		}
		return server.SingleRegistry(st), nil
	}

	reg, err := server.NewRegistry(ordered, stores, paths)
	if err != nil {
		return nil, err
	}
	// Active = the cwd repo when it's registered, else the first.
	if pineDir, err := findPineDir(flagDir); err == nil {
		cwdRoot := filepath.Dir(pineDir)
		for _, a := range ordered {
			if paths[a] == cwdRoot {
				_ = reg.SetActive(a)
				break
			}
		}
	}
	return reg, nil
}

// devProxy routes API and attachment requests to the Go server and everything
// else to the Vite dev server for frontend hot reloading.
func devProxy(api http.Handler) http.Handler {
	target, _ := url.Parse("http://localhost:5173")
	proxy := httputil.NewSingleHostReverseProxy(target)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/attachments") {
			api.ServeHTTP(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
