package server

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

// rawSnapshotTickets decodes /api/snapshot tickets as generic maps so a test
// can tell an absent "body" key from an empty one.
func rawSnapshotTickets(t *testing.T, body string) []map[string]any {
	t.Helper()
	var snap struct {
		Tickets []map[string]any `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(body), &snap); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	return snap.Tickets
}

func TestSnapshotBodiesQuery(t *testing.T) {
	ts := newTestServer(t)
	for i := 1; i <= 2; i++ {
		do(t, "POST", ts.URL+"/api/tickets", fmt.Sprintf(`{"type":"bug","title":"t%d","body":"hello body %d"}`, i, i), nil)
	}

	// Default: bodies included, so existing API clients are unaffected.
	_, full := do(t, "GET", ts.URL+"/api/snapshot", "", nil)
	tickets := rawSnapshotTickets(t, full)
	if len(tickets) != 2 {
		t.Fatalf("tickets = %d", len(tickets))
	}
	for _, tk := range tickets {
		if b, _ := tk["body"].(string); !strings.Contains(b, "hello body") {
			t.Errorf("default snapshot must carry bodies, got %v", tk["body"])
		}
	}

	for _, q := range []string{"?bodies=0", "?bodies=false"} {
		_, slim := do(t, "GET", ts.URL+"/api/snapshot"+q, "", nil)
		for _, tk := range rawSnapshotTickets(t, slim) {
			if _, has := tk["body"]; has {
				t.Errorf("%s: body must be omitted, got %v", q, tk["body"])
			}
			// The hash is what the client keys lazy-loaded bodies on.
			if h, _ := tk["hash"].(string); h == "" {
				t.Errorf("%s: hash must stay in the snapshot: %v", q, tk)
			}
		}
	}
}

func TestSnapshotWithoutBodiesIsSmall(t *testing.T) {
	ts := newTestServer(t)
	big := strings.Repeat("line of ticket evidence\\n", 200) // ~4.8 KB once JSON-escaped
	for i := 0; i < 60; i++ {
		do(t, "POST", ts.URL+"/api/tickets", fmt.Sprintf(`{"type":"feature","title":"t%d","body":"%s"}`, i, big), nil)
	}
	_, full := do(t, "GET", ts.URL+"/api/snapshot", "", nil)
	_, slim := do(t, "GET", ts.URL+"/api/snapshot?bodies=0", "", nil)
	if len(full) < 200_000 {
		t.Fatalf("test setup: full snapshot only %d bytes", len(full))
	}
	if len(slim)*10 > len(full) {
		t.Errorf("bodies=0 snapshot is %d bytes vs %d with bodies; want < 10%%", len(slim), len(full))
	}
}

// Off-branch tickets cannot be fetched by id (GET /api/tickets/{id} only serves
// the working tree), so the snapshot keeps their bodies even when asked not to.
func TestSnapshotKeepsOffBranchBodies(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")
	_, body := do(t, "GET", ts.URL+"/api/snapshot?bodies=0", "", nil)
	var feat, bug *view.Ticket
	tickets := snapshotTickets(t, body)
	for i := range tickets {
		switch tickets[i].ID {
		case "FEAT-3d4e5f":
			feat = &tickets[i]
		case "BUG-0a1b2c":
			bug = &tickets[i]
		}
	}
	if feat == nil || bug == nil {
		t.Fatalf("missing tickets in snapshot: %s", body)
	}
	if feat.Body == "" {
		t.Error("off-branch ticket must keep its body")
	}
	if bug.Body != "" {
		t.Errorf("local ticket body should be omitted, got %q", bug.Body)
	}
}

func gzipGet(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	// Setting Accept-Encoding ourselves stops the transport from transparently
	// decompressing, so the test sees what is on the wire.
	resp, raw := do(t, "GET", url, "", map[string]string{"Accept-Encoding": "gzip"})
	return resp, raw
}

func TestJSONIsGzipped(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x","body":"hello"}`, nil)

	resp, raw := gzipGet(t, ts.URL+"/api/snapshot")
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("snapshot should be gzipped, headers = %v", resp.Header)
	}
	zr, err := gzip.NewReader(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if len(rawSnapshotTickets(t, string(plain))) != 1 {
		t.Fatalf("decompressed snapshot is wrong: %s", plain)
	}

	// A client that does not ask for gzip gets plain JSON.
	resp, raw = do(t, "GET", ts.URL+"/api/snapshot", "", map[string]string{"Accept-Encoding": "identity"})
	if resp.Header.Get("Content-Encoding") != "" || !strings.HasPrefix(raw, "{") {
		t.Errorf("identity request must get plain JSON, got encoding %q", resp.Header.Get("Content-Encoding"))
	}
}

// The event stream must stay uncompressed and flush immediately, or live sync
// would buffer behind the compressor.
func TestEventStreamIsNotGzipped(t *testing.T) {
	ts := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("event stream must not be compressed, got %q", enc)
	}
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, ": connected") {
		t.Fatalf("first SSE line = %q (%v); the stream must flush through middleware", line, err)
	}
}

// BenchmarkSnapshot reports the snapshot size and cost for a repo shaped like a
// long-lived tracker: hundreds of tickets with large bodies (work evidence).
func BenchmarkSnapshot(b *testing.B) {
	st := newTestStore(b)
	body := "\n# Description\n\n" + strings.Repeat("A paragraph of ticket notes with `code` and a link.\n", 150) +
		"\n## Work Evidence\n\n```\n" + strings.Repeat(" internal/some/file.go | 12 ++++----\n", 100) + "```\n"
	for i := 0; i < 600; i++ {
		if _, err := st.Create(store.CreateReq{Type: "feature", Title: fmt.Sprintf("ticket %d", i), Body: body}); err != nil {
			b.Fatal(err)
		}
	}
	ts := httptest.NewServer(New(st, "bench").Handler())
	b.Cleanup(ts.Close)

	for _, tc := range []struct{ name, query string }{{"with-bodies", ""}, {"no-bodies", "?bodies=0"}} {
		b.Run(tc.name, func(b *testing.B) {
			var size int
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp, err := http.Get(ts.URL + "/api/snapshot" + tc.query)
				if err != nil {
					b.Fatal(err)
				}
				data, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				size = len(data)
			}
			b.ReportMetric(float64(size), "bytes/resp")
		})
	}
}
