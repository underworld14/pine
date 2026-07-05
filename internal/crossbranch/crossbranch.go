// Package crossbranch aggregates tickets that live on other git branches into a
// read-only overlay for the board. It reads each branch's ticket files straight
// from its committed git tree (via gitx) without checking anything out, so a
// ticket created on a feature branch is visible from wherever you are.
//
// The design is index-first, hydrate-later: branches are enumerated and their
// ticket files listed cheaply (no content read), then only the winning copy of
// each off-branch ID is hydrated with `git show`. Correctness relies on hash
// IDs, where an ID is globally unique — so the same ID on two branches is the
// same ticket and may be merged. Sequential-ID repos must not aggregate (their
// IDs collide across branches); callers gate on IDStyle == "hash".
//
// This package depends only on gitx and ticket — never on store — so the local
// write path is never made git-aware.
package crossbranch

import (
	"context"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/ticket"
)

const (
	defaultMaxBranches = 50
	defaultMaxTickets  = 500
	defaultTicketsPath = ".pine/tickets"
	indexConcurrency   = 4
	hydrateConcurrency = 8
)

// Off is a ticket that lives on another branch (not the checked-out working
// tree), hydrated read-only from that branch's git tree.
type Off struct {
	Ticket *ticket.Ticket
	Branch string // the branch the winning copy came from
}

// Options configures a cross-branch scan.
type Options struct {
	Enabled     bool
	IDStyle     string           // aggregation only runs for "hash"
	ActiveDays  int              // only branches whose tip is within N days; <=0 = no window
	TicketsPath string           // repo-root-relative tickets dir; default ".pine/tickets"
	Now         func() time.Time // injectable clock for tests
	MaxBranches int              // cap on branches scanned (0 = default 50)
	MaxTickets  int              // cap on off-branch tickets returned (0 = default 500)
}

// candidate is one off-branch copy of a ticket ID, indexed without a content read.
type candidate struct {
	branch   string
	sha      string
	path     string
	branchAt time.Time
}

// Compute enumerates recent branches (skipping currentBranch), lists ticket
// files per pinned branch-tip SHA, and for every ID not in localIDs hydrates the
// copies and keeps the one with the newest frontmatter Updated (tie-break: branch
// commit date). It returns nil when disabled or when IDStyle != "hash". It is
// best-effort: git failures degrade to fewer results and never panic.
func Compute(ctx context.Context, g gitx.Client, currentBranch string, localIDs map[string]bool, opts Options) []Off {
	if !opts.Enabled || opts.IDStyle != "hash" {
		return nil
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	maxBranches := opts.MaxBranches
	if maxBranches <= 0 {
		maxBranches = defaultMaxBranches
	}
	maxTickets := opts.MaxTickets
	if maxTickets <= 0 {
		maxTickets = defaultMaxTickets
	}
	ticketsPath := opts.TicketsPath
	if ticketsPath == "" {
		ticketsPath = defaultTicketsPath
	}

	// 1. Enumerate branches, drop the current one, apply the recency window,
	//    then keep the most-recent maxBranches.
	var cutoff time.Time
	if opts.ActiveDays > 0 {
		cutoff = now().AddDate(0, 0, -opts.ActiveDays)
	}
	var picked []gitx.Branch
	for _, b := range g.Branches(ctx) {
		if b.Name == "" || b.SHA == "" || b.Name == currentBranch {
			continue
		}
		if !cutoff.IsZero() && b.CommitDate.Before(cutoff) {
			continue
		}
		picked = append(picked, b)
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].CommitDate.After(picked[j].CommitDate) })
	if len(picked) > maxBranches {
		picked = picked[:maxBranches]
	}
	if len(picked) == 0 {
		return nil
	}

	// 2. Index ticket files across branches (no content read), concurrently.
	var mu sync.Mutex
	index := map[string][]candidate{}
	runBounded(ctx, len(picked), indexConcurrency, func(i int) {
		b := picked[i]
		files := g.ListTreeFiles(ctx, b.SHA, ticketsPath)
		if len(files) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, f := range files {
			base := path.Base(f)
			if !strings.HasSuffix(base, ".md") {
				continue
			}
			id := strings.TrimSuffix(base, ".md")
			if !ticket.ValidID(id) || localIDs[id] {
				continue // malformed name, or the working tree already owns this ID
			}
			// Never cross-branch-merge a sequential-form ID: those collide across
			// branches, so aggregating them is unsafe. This backstops the config
			// gate for repos whose stale config.json still defaults idStyle to
			// "hash" despite using sequential IDs.
			if ticket.IsSequentialID(id) {
				continue
			}
			index[id] = append(index[id], candidate{branch: b.Name, sha: b.SHA, path: f, branchAt: b.CommitDate})
		}
	})
	if len(index) == 0 {
		return nil
	}

	// 3. Cap the number of distinct off-branch IDs, keeping the most recent.
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		di, dj := bestDate(index[ids[i]]), bestDate(index[ids[j]])
		if di.Equal(dj) {
			return ids[i] < ids[j]
		}
		return di.After(dj)
	})
	if len(ids) > maxTickets {
		ids = ids[:maxTickets]
	}

	// 4. Hydrate the winning copy of each ID.
	out := make([]Off, 0, len(ids))
	var outMu sync.Mutex
	runBounded(ctx, len(ids), hydrateConcurrency, func(i int) {
		id := ids[i]
		var best *ticket.Ticket
		var bestBranch string
		var bestKey time.Time
		for _, c := range index[id] {
			content, ok := g.ShowFile(ctx, c.sha, c.path)
			if !ok {
				continue
			}
			tk := ticket.Parse(id, content)
			if tk == nil {
				continue
			}
			key := tk.Updated
			if key.IsZero() {
				key = c.branchAt // no frontmatter timestamp: fall back to branch commit time
			}
			if best == nil || key.After(bestKey) {
				best, bestBranch, bestKey = tk, c.branch, key
			}
		}
		if best == nil {
			return
		}
		if best.Updated.IsZero() {
			best.Updated = bestKey // surface a sensible timestamp to the UI
		}
		outMu.Lock()
		out = append(out, Off{Ticket: best, Branch: bestBranch})
		outMu.Unlock()
	})

	sort.Slice(out, func(i, j int) bool { return out[i].Ticket.ID < out[j].Ticket.ID })
	return out
}

// bestDate returns the newest branch commit date among an ID's candidates.
func bestDate(cs []candidate) time.Time {
	var t time.Time
	for _, c := range cs {
		if c.branchAt.After(t) {
			t = c.branchAt
		}
	}
	return t
}

// runBounded runs fn(0..n-1) across at most `workers` goroutines. It stops
// doing work once ctx is cancelled.
func runBounded(ctx context.Context, n, workers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > n {
		workers = n
	}
	ch := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				if ctx.Err() != nil {
					continue // drain without work once cancelled
				}
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		ch <- i
	}
	close(ch)
	wg.Wait()
}
