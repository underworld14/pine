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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/izzadev/pine/internal/server"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

const defaultPort = 3412

func newServeCmd() *cobra.Command {
	var (
		port      int
		host      string
		open, dev bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Pine web UI and API on localhost",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, port, host, open, dev)
		},
	}
	f := cmd.Flags()
	f.IntVar(&port, "port", defaultPort, "port to listen on")
	f.StringVar(&host, "host", "127.0.0.1", "host to bind (localhost only by design)")
	f.BoolVar(&open, "open", false, "open the browser after starting")
	f.BoolVar(&dev, "dev", false, "proxy non-API requests to the Vite dev server (localhost:5173)")
	return cmd
}

func runServe(cmd *cobra.Command, port int, host string, open, dev bool) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	srv := server.New(st, version)

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

	// Start live sync (watcher + coordinator) so external edits push to the UI.
	closeSync := srv.StartLiveSync()
	defer closeSync()

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Pine serving %q on %s\n", st.Config().Project.Name, uiURL)
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
