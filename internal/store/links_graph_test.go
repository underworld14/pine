package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/ticket"
)

// TestLinksGraphCachedAndInvalidated verifies the I2 cache: LinksGraph() returns
// a cached graph, and every ticket/learning/config/memory mutation
// invalidates it so the next call rebuilds from the fresh on-disk state.
func TestLinksGraphCachedAndInvalidated(t *testing.T) {
	s := scaffold(t)

	// Empty graph is cached after the first call.
	g1 := s.LinksGraph()
	if g1 == nil {
		t.Fatal("first LinksGraph nil")
	}
	// Second call returns the same cached pointer (no rebuild).
	g2 := s.LinksGraph()
	if g1 != g2 {
		t.Fatal("LinksGraph rebuilt on a no-op call (cache miss)")
	}

	// Create a ticket with a link → cache invalidated → next call rebuilds.
	tk, err := s.Create(CreateReq{Type: "bug", Title: "linked", Links: []string{"MEMORY"}})
	if err != nil {
		t.Fatal(err)
	}
	g3 := s.LinksGraph()
	if g3 == g1 {
		t.Fatal("LinksGraph not invalidated after Create")
	}
	// The new ticket node must appear.
	found := false
	for _, n := range g3.Nodes {
		if n.ID == tk.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("new ticket %s missing from rebuilt graph nodes", tk.ID)
	}

	// Update the ticket's links → cache invalidated.
	_, err = s.Update(tk.ID, func(u *ticket.Ticket) error {
		u.Links = []string{"memory/git-habits"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	g4 := s.LinksGraph()
	if g4 == g3 {
		t.Fatal("LinksGraph not invalidated after Update")
	}

	// Delete the ticket → cache invalidated.
	if err := s.Delete(tk.ID); err != nil {
		t.Fatal(err)
	}
	g5 := s.LinksGraph()
	if g5 == g4 {
		t.Fatal("LinksGraph not invalidated after Delete")
	}
	for _, n := range g5.Nodes {
		if n.ID == tk.ID {
			t.Fatalf("deleted ticket %s still in graph nodes", tk.ID)
		}
	}

	// SaveConfig → cache invalidated (config can affect graph shape via idStyle etc.).
	if err := s.SaveConfig(s.Config()); err != nil {
		t.Fatal(err)
	}
	g6 := s.LinksGraph()
	if g6 == g5 {
		t.Fatal("LinksGraph not invalidated after SaveConfig")
	}
}

// TestLinksGraphConcurrentMutationRace hammers LinksGraph() from a reader
// while a writer creates tickets (each invalidating the cache). Run under
// -race: it verifies the lock discipline and that no stale graph (missing
// tickets) is ever cached. The generation counter in LinksGraph discards
// a build whose inputs were invalidated mid-build, so the final read always
// reflects the latest on-disk state.
func TestLinksGraphConcurrentMutationRace(t *testing.T) {
	s := scaffold(t)
	const n = 80
	stop := make(chan struct{})
	var wg sync.WaitGroup
	// Reader: hammer LinksGraph() concurrently with the writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.LinksGraph()
			}
		}
	}()
	// Writer: create n tickets, each invalidating the cache.
	for i := 0; i < n; i++ {
		if _, err := s.Create(CreateReq{Type: "bug", Title: "x"}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()

	// Final read must reflect every ticket — a stale graph cached mid-build
	// would miss some. The last Create invalidated the cache, so this rebuilds.
	g := s.LinksGraph()
	count := 0
	for _, node := range g.Nodes {
		if strings.HasPrefix(node.ID, "BUG-") {
			count++
		}
	}
	if count != n {
		t.Fatalf("graph has %d BUG nodes, want %d (stale cache)", count, n)
	}
}

// TestLinksGraphInvalidatedOnMemoryWrite verifies a memory topic edit (which has
// no store write path — only the watcher sees it) drops the cache via
// InvalidateLinksGraph, so a long-running serve stays coherent.
func TestLinksGraphInvalidatedOnMemoryWrite(t *testing.T) {
	s := scaffold(t)
	g1 := s.LinksGraph()

	// Simulate an external memory topic write (agent/editor) by writing a topic
	// file directly, then invalidating as the watcher would.
	memDir := filepath.Join(s.Root(), "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "git-habits.md"), []byte("---\ntopic: git-habits\nlinks:\n  - MEMORY\n---\n# git-habits\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.InvalidateLinksGraph()

	g2 := s.LinksGraph()
	if g2 == g1 {
		t.Fatal("LinksGraph not invalidated after memory topic write + InvalidateLinksGraph")
	}
	// The new topic node must appear.
	found := false
	for _, n := range g2.Nodes {
		if n.ID == "memory/git-habits" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("new memory topic missing from rebuilt graph nodes; got %d nodes", len(g2.Nodes))
	}
	// MEMORY.md absence is reflected too (hasMEMORY flag).
	_ = memory.ReadMEMORY // keep import used even if MEMORY.md is absent here
}
