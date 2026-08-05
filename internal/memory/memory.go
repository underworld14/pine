// Package memory manages project MEMORY.md and topic files under .pine/memory/.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// FileMEMORY is the always-on project constitution under .pine/.
	FileMEMORY = "MEMORY.md"
	// DirTopics is the directory of append-only topic files under .pine/.
	DirTopics = "memory"

	// AutoMinScore is the minimum top recommendation score for auto-append.
	AutoMinScore = 0.55
	// AutoMinGap is the minimum score gap between #1 and #2 for auto-append.
	AutoMinGap = 0.12
	// SoftTopicThreshold below which we also suggest NEW:<slug>.
	SoftTopicThreshold = 0.4

	// ContextMEMORYCap is the max bytes of MEMORY.md injected into pine context.
	ContextMEMORYCap = 3500
)

// DefaultMEMORY is the seed content written by pine init / first append.
const DefaultMEMORY = `# Project memory

Stable preferences, conventions, and rules for this repository.
Agents: prefer appending here (or a topic under memory/) over creating new LRN-* files.
Ticket-scoped one-shots still use ` + "`pine learn --scope ticket`" + `.

## Preferences

## Conventions

## Gotchas

## Log
`

var topicSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Topic is one .pine/memory/<slug>.md file.
type Topic struct {
	Slug    string
	RelPath string // memory/<slug>.md
	Title   string
	Body    string
	Links   []string // typed graph refs (ticket / LRN-* / memory/<slug> / MEMORY)
	Updated time.Time
}

// Recommendation is a suggested destination for a new insight.
type Recommendation struct {
	Path   string  `json:"path"` // MEMORY.md | memory/<slug>.md | NEW:<slug>
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// PineDir helpers.
func MemoryPath(pineDir string) string { return filepath.Join(pineDir, FileMEMORY) }
func TopicsDir(pineDir string) string  { return filepath.Join(pineDir, DirTopics) }
func TopicPath(pineDir, slug string) string {
	return filepath.Join(TopicsDir(pineDir), Slugify(slug)+".md")
}

// EnsureLayout creates memory/ and seeds the project MEMORY.md if missing.
func EnsureLayout(pineDir string) error { return ensureLayout(pineDir, DefaultMEMORY) }

// ensureLayout creates memory/ under pineDir and writes seed to MEMORY.md only
// when it is absent.
//
// Ordering invariant for the machine-wide store: AppendMEMORY and AppendTopic
// call EnsureLayout (the project seed) internally, so every global write must
// reach EnsureGlobalLayout first. The inner call then finds MEMORY.md present
// and no-ops instead of re-seeding ~/.pine with project-flavoured text.
func ensureLayout(pineDir, seed string) error {
	if err := os.MkdirAll(TopicsDir(pineDir), 0o755); err != nil {
		return err
	}
	path := MemoryPath(pineDir)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(seed), 0o644)
}

// Slugify normalizes a topic name to a filename-safe slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = topicSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "general"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

// ReadMEMORY returns MEMORY.md contents (empty string if missing).
func ReadMEMORY(pineDir string) (string, error) {
	data, err := os.ReadFile(MemoryPath(pineDir))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListTopics returns all topic files sorted by slug.
func ListTopics(pineDir string) ([]Topic, error) {
	dir := TopicsDir(pineDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Topic
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		t, err := ReadTopic(pineDir, slug)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// ReadTopic loads one topic by slug.
func ReadTopic(pineDir, slug string) (Topic, error) {
	slug = Slugify(slug)
	path := TopicPath(pineDir, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		return Topic{}, err
	}
	body := string(data)
	updated := time.Time{}
	var links []string
	if fi, err := os.Stat(path); err == nil {
		updated = fi.ModTime().UTC()
	}
	if fm, rest, ok := splitFrontmatterRaw(body); ok {
		body = rest
		if u, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint(fm["updated"]))); err == nil {
			updated = u
		}
		links = decodeLinksAny(fm["links"])
	}
	title := slug
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}
	return Topic{
		Slug:    slug,
		RelPath: filepath.ToSlash(filepath.Join(DirTopics, slug+".md")),
		Title:   title,
		Body:    body,
		Links:   links,
		Updated: updated,
	}, nil
}

// AppendOpts controls an append write.
type AppendOpts struct {
	Text  string
	Cites []string
	Now   time.Time
}

// AppendMEMORY appends a dated bullet under ## Log.
func AppendMEMORY(pineDir string, opts AppendOpts) error {
	if err := EnsureLayout(pineDir); err != nil {
		return err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return fmt.Errorf("insight text is required")
	}
	path := MemoryPath(pineDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, "## Log") {
		content = strings.TrimRight(content, "\n") + "\n\n## Log\n"
	}
	bullet := formatBullet(now, text, opts.Cites)
	content = appendUnderHeading(content, "## Log", bullet)
	return os.WriteFile(path, []byte(content), 0o644)
}

// AppendTopic appends a bullet to memory/<slug>.md, creating the file if needed.
func AppendTopic(pineDir, slug string, opts AppendOpts) error {
	if err := EnsureLayout(pineDir); err != nil {
		return err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return fmt.Errorf("insight text is required")
	}
	slug = Slugify(slug)
	path := TopicPath(pineDir, slug)

	var content string
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	} else if os.IsNotExist(err) {
		content = fmt.Sprintf("---\ntopic: %s\nupdated: %s\n---\n\n# %s\n\n", slug, now.Format(time.RFC3339), slug)
	} else {
		return err
	}

	// Refresh updated in frontmatter when present; preserve links.
	if fm, rest, ok := splitFrontmatterRaw(content); ok {
		fm["topic"] = slug
		fm["updated"] = now.Format(time.RFC3339)
		content = joinFrontmatterRaw(fm) + rest
	}

	bullet := formatBullet(now, text, opts.Cites)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += bullet + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// SetTopicLinks replaces the links frontmatter on a topic file, creating the
// file with a stub body when it does not yet exist.
func SetTopicLinks(pineDir, slug string, links []string) error {
	if err := EnsureLayout(pineDir); err != nil {
		return err
	}
	slug = Slugify(slug)
	path := TopicPath(pineDir, slug)
	now := time.Now().UTC()
	var content string
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	} else if os.IsNotExist(err) {
		content = fmt.Sprintf("---\ntopic: %s\nupdated: %s\n---\n\n# %s\n\n", slug, now.Format(time.RFC3339), slug)
	} else {
		return err
	}
	fm, rest, ok := splitFrontmatterRaw(content)
	if !ok {
		fm = map[string]any{}
		rest = content
	}
	fm["topic"] = slug
	fm["updated"] = now.Format(time.RFC3339)
	if len(links) == 0 {
		delete(fm, "links")
	} else {
		fm["links"] = append([]string(nil), links...)
	}
	return os.WriteFile(path, []byte(joinFrontmatterRaw(fm)+rest), 0o644)
}

func formatBullet(now time.Time, text string, cites []string) string {
	line := fmt.Sprintf("- %s: %s", now.Format("2006-01-02"), text)
	if len(cites) > 0 {
		line += " (cites: " + strings.Join(cites, ", ") + ")"
	}
	return line
}

func appendUnderHeading(content, heading, bullet string) string {
	idx := strings.Index(content, heading)
	if idx < 0 {
		return strings.TrimRight(content, "\n") + "\n\n" + heading + "\n" + bullet + "\n"
	}
	rest := content[idx+len(heading):]
	// Find next ## heading after this one.
	next := strings.Index(rest, "\n## ")
	if next < 0 {
		body := strings.TrimRight(rest, "\n")
		return content[:idx+len(heading)] + body + "\n" + bullet + "\n"
	}
	before := strings.TrimRight(rest[:next], "\n")
	after := rest[next:]
	return content[:idx+len(heading)] + before + "\n" + bullet + "\n" + after
}

func splitFrontmatter(content string) (map[string]string, string, bool) {
	raw, rest, ok := splitFrontmatterRaw(content)
	if !ok {
		return nil, content, false
	}
	fm := map[string]string{}
	for k, v := range raw {
		fm[k] = fmt.Sprint(v)
	}
	return fm, rest, true
}

// splitFrontmatterRaw keeps typed YAML values (needed for links arrays).
func splitFrontmatterRaw(content string) (map[string]any, string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content, false
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, content, false
	}
	raw := rest[:end]
	body := rest[end+5:]
	var node map[string]any
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		return nil, content, false
	}
	if node == nil {
		node = map[string]any{}
	}
	return node, body, true
}

func joinFrontmatter(fm map[string]string) string {
	raw := make(map[string]any, len(fm))
	for k, v := range fm {
		raw[k] = v
	}
	return joinFrontmatterRaw(raw)
}

func joinFrontmatterRaw(fm map[string]any) string {
	keys := []string{"topic", "updated", "links"}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range keys {
		v, ok := fm[k]
		if !ok {
			continue
		}
		seen[k] = true
		writeFMValue(&b, k, v)
	}
	var extra []string
	for k := range fm {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		writeFMValue(&b, k, fm[k])
	}
	b.WriteString("---\n")
	return b.String()
}

func writeFMValue(b *strings.Builder, k string, v any) {
	switch x := v.(type) {
	case []string:
		fmt.Fprintf(b, "%s:\n", k)
		for _, s := range x {
			fmt.Fprintf(b, "  - %s\n", s)
		}
	case []any:
		fmt.Fprintf(b, "%s:\n", k)
		for _, item := range x {
			fmt.Fprintf(b, "  - %s\n", fmt.Sprint(item))
		}
	default:
		fmt.Fprintf(b, "%s: %s\n", k, fmt.Sprint(v))
	}
}

func decodeLinksAny(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" && s != "<nil>" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}

// ResolveTo normalizes a --to argument into ("memory"|"topic"|"new", pathOrSlug).
func ResolveTo(to string) (kind, value string, err error) {
	to = filepath.ToSlash(strings.TrimSpace(to))
	to = strings.TrimPrefix(to, "./")
	to = strings.TrimPrefix(to, ".pine/")
	switch {
	case to == "" || strings.EqualFold(to, FileMEMORY) || strings.EqualFold(to, "memory.md"):
		return "memory", FileMEMORY, nil
	case strings.HasPrefix(to, "NEW:"):
		return "new", Slugify(strings.TrimPrefix(to, "NEW:")), nil
	case strings.HasPrefix(to, DirTopics+"/"):
		base := strings.TrimSuffix(filepath.Base(to), ".md")
		return "topic", Slugify(base), nil
	case strings.HasSuffix(to, ".md") && !strings.Contains(to, "/"):
		// bare analytics.md → topic
		return "topic", Slugify(strings.TrimSuffix(to, ".md")), nil
	default:
		// treat as topic slug
		if strings.Contains(to, "/") {
			return "", "", fmt.Errorf("unknown --to path %q (use MEMORY.md or memory/<topic>.md)", to)
		}
		return "topic", Slugify(to), nil
	}
}

// TruncateForContext returns a MEMORY body capped for pine context. srcLabel
// names the file to read in full and must match the store the body came from
// (".pine/MEMORY.md", "~/.pine/MEMORY.md", …) — a caller that gets this wrong
// points the reader at someone else's memory.
func TruncateForContext(body string, capBytes int, srcLabel string) string {
	if capBytes <= 0 {
		capBytes = ContextMEMORYCap
	}
	if len(body) <= capBytes {
		return body
	}
	cut := body[:capBytes]
	if i := strings.LastIndex(cut, "\n"); i > capBytes/2 {
		cut = cut[:i]
	}
	return cut + "\n\n… truncated — see `" + srcLabel + "`\n"
}
