// Package watch turns raw filesystem notifications under .pine/ into debounced,
// classified change events. It never interprets fsnotify op flags directly; it
// only reports "something happened at this path", and the coordinator reconciles
// by re-reading. It watches .pine/, .pine/tickets/, and .pine/learnings/
// (config, board, tickets, learnings); attachment changes are broadcast by the
// API, not the watcher (see design).
package watch

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Kind categorizes which store artifact a path belongs to.
type Kind int

const (
	KindOther Kind = iota
	KindTicket
	KindConfig
	KindBoard
	KindLearning
)

// Event is a debounced, classified change at a path.
type Event struct {
	Kind Kind
	Path string
	ID   string // ticket/learning id when Kind is KindTicket or KindLearning
}

var ticketFileRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9a-hj-km-np-tv-z]+\.md$`)

// debounceWindow coalesces bursts of events (e.g. editor atomic saves).
const debounceWindow = 150 * time.Millisecond

// Watcher emits batches of debounced events.
type Watcher struct {
	pineDir string
	fsw     *fsnotify.Watcher
	out     chan []Event
	done    chan struct{}
}

// New starts watching .pine, .pine/tickets, and .pine/learnings.
func New(pineDir string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		pineDir: pineDir,
		fsw:     fsw,
		out:     make(chan []Event, 16),
		done:    make(chan struct{}),
	}
	// Best-effort: watch the root and known subdirectories.
	_ = fsw.Add(pineDir)
	_ = fsw.Add(filepath.Join(pineDir, "tickets"))
	_ = fsw.Add(filepath.Join(pineDir, "learnings"))
	go w.loop()
	return w, nil
}

// Events returns the channel of debounced event batches.
func (w *Watcher) Events() <-chan []Event { return w.out }

// Close stops watching.
func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}

func (w *Watcher) loop() {
	pending := map[string]Event{}
	timer := time.NewTimer(debounceWindow)
	timer.Stop()
	timerActive := false

	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := make([]Event, 0, len(pending))
		for _, ev := range pending {
			batch = append(batch, ev)
		}
		pending = map[string]Event{}
		select {
		case w.out <- batch:
		case <-w.done:
		}
	}

	arm := func() {
		if timerActive {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer.Reset(debounceWindow)
		timerActive = true
	}

	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// A newly created tickets/ or learnings/ directory should be watched too.
			base := filepath.Base(ev.Name)
			if ev.Op&fsnotify.Create != 0 && (base == "tickets" || base == "learnings") {
				_ = w.fsw.Add(ev.Name)
			}
			if e, relevant := w.classify(ev.Name); relevant {
				pending[ev.Name] = e
				arm()
			}
		case <-timer.C:
			timerActive = false
			flush()
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
		}
	}
}

// classify maps a path to an event, or reports it as irrelevant. Editor temp
// files and dotfiles are ignored.
func (w *Watcher) classify(path string) (Event, bool) {
	base := filepath.Base(path)
	if ignoredName(base) {
		return Event{}, false
	}
	rel, err := filepath.Rel(w.pineDir, path)
	if err != nil {
		return Event{}, false
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "config.json":
		return Event{Kind: KindConfig, Path: path}, true
	case rel == "board.json":
		return Event{Kind: KindBoard, Path: path}, true
	case strings.HasPrefix(rel, "tickets/"):
		if ticketFileRe.MatchString(base) {
			id := strings.TrimSuffix(base, ".md")
			return Event{Kind: KindTicket, Path: path, ID: id}, true
		}
	case strings.HasPrefix(rel, "learnings/"):
		if ticketFileRe.MatchString(base) {
			id := strings.TrimSuffix(base, ".md")
			return Event{Kind: KindLearning, Path: path, ID: id}, true
		}
	}
	return Event{}, false
}

func ignoredName(base string) bool {
	if base == "" || strings.HasPrefix(base, ".") {
		return true
	}
	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".swx") {
		return true
	}
	if base == "4913" { // vim's write-probe file
		return true
	}
	return false
}
