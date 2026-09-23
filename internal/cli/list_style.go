package cli

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

// listStyle paints tree tokens. The zero value is a no-op (plain text), which
// tests rely on so substring assertions stay ANSI-free.
type listStyle struct {
	color bool
}

// listStyleFor enables color only when w is a TTY and NO_COLOR is unset.
func listStyleFor(w io.Writer) listStyle {
	if os.Getenv("NO_COLOR") != "" {
		return listStyle{}
	}
	f, ok := w.(*os.File)
	if !ok {
		return listStyle{}
	}
	return listStyle{color: term.IsTerminal(int(f.Fd()))}
}

func (s listStyle) wrap(code, text string) string {
	if !s.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s listStyle) dim(text string) string {
	return s.wrap("2", text)
}

func (s listStyle) status(v view.Ticket) string {
	icon := statusIcon(v)
	if v.Blocked && v.Status != "done" {
		return s.wrap("33", icon)
	}
	switch v.Status {
	case "todo":
		return s.wrap("2", icon)
	case "doing":
		return s.wrap("32", icon)
	case "testing":
		return s.wrap("34", icon)
	case "done":
		return s.wrap("2;32", icon)
	default:
		return s.wrap("2", icon)
	}
}

func (s listStyle) priority(p string) string {
	token := "● " + p
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "critical":
		return s.wrap("31", token)
	case "high":
		return s.wrap("33", token)
	case "medium":
		return s.wrap("34", token)
	default:
		return s.wrap("2", token)
	}
}

func (s listStyle) typeBadge(prefix string) string {
	up := strings.ToUpper(strings.TrimSpace(prefix))
	if up == "" {
		return ""
	}
	badge := "[" + strings.ToLower(up) + "]"
	switch up {
	case "BUG":
		return s.wrap("35", badge)
	case "FEAT":
		return s.wrap("36", badge)
	case "EPIC":
		return s.wrap("32", badge)
	default:
		return s.wrap("2", badge)
	}
}

func (s listStyle) phase(p string) string {
	return s.wrap("2;36", "["+p+"]")
}

func (s listStyle) depAnn(ann string) string {
	return s.wrap("33", ann)
}

func ticketTypePrefix(v view.Ticket) string {
	if t := strings.ToUpper(strings.TrimSpace(v.Type)); t != "" {
		return t
	}
	return ticket.PrefixOf(v.ID)
}
