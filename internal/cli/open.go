package cli

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func newOpenCmd() *cobra.Command {
	var (
		port int
		host string
	)
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open the Pine web UI, starting the server if needed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			uiURL := "http://" + addr
			if serverHealthy(uiURL) {
				fmt.Fprintf(cmd.OutOrStdout(), "Pine already running — opening %s\n", uiURL)
				return browser.OpenURL(uiURL)
			}
			// Not running: start serving and open the browser.
			return runServe(cmd, port, host, true, false)
		},
	}
	f := cmd.Flags()
	f.IntVar(&port, "port", defaultPort, "port to use")
	f.StringVar(&host, "host", "127.0.0.1", "host to use")
	return cmd
}

// serverHealthy reports whether a Pine server is already answering on base.
func serverHealthy(base string) bool {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(base + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
