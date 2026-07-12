package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/frontmatter"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// Beads JSONL record types (subset of bd export schema).

type beadsDep struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

type beadsComment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type beadsIssue struct {
	RecordType         string         `json:"_type"`
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Description        string         `json:"description"`
	Design             string         `json:"design"`
	AcceptanceCriteria string         `json:"acceptance_criteria"`
	Notes              string         `json:"notes"`
	Status             string         `json:"status"`
	Priority           int            `json:"priority"`
	IssueType          string         `json:"issue_type"`
	Assignee           string         `json:"assignee"`
	Owner              string         `json:"owner"`
	ExternalRef        *string        `json:"external_ref"`
	CloseReason        string         `json:"close_reason"`
	CreatedAt          time.Time      `json:"created_at"`
	Labels             []string       `json:"labels"`
	Dependencies       []beadsDep     `json:"dependencies"`
	Comments           []beadsComment `json:"comments"`
	Parent             *string        `json:"parent"`
}

// Infra / non-work Beads types skipped by default.
var beadsInfraTypes = map[string]bool{
	"message": true, "molecule": true, "gate": true, "event": true,
	"agent": true, "role": true, "rig": true, "convoy": true,
}

// Default Beads issue_type → Pine prefix (+ display name for auto-register).
var beadsBuiltinTypes = map[string]config.TicketType{
	"bug":         {Prefix: "BUG", Name: "Bug"},
	"feature":     {Prefix: "FEAT", Name: "Feature"},
	"enhancement": {Prefix: "FEAT", Name: "Feature"},
	"feat":        {Prefix: "FEAT", Name: "Feature"},
	"epic":        {Prefix: "EPIC", Name: "Epic"},
	"task":        {Prefix: "TASK", Name: "Task"},
	"chore":       {Prefix: "CHORE", Name: "Chore"},
	"decision":    {Prefix: "DECISION", Name: "Decision"},
	"dec":         {Prefix: "DECISION", Name: "Decision"},
	"adr":         {Prefix: "DECISION", Name: "Decision"},
	"spike":       {Prefix: "SPIKE", Name: "Spike"},
	"story":       {Prefix: "STORY", Name: "Story"},
	"milestone":   {Prefix: "MILESTONE", Name: "Milestone"},
}

var beadsBuiltinStatus = map[string]string{
	"open":        "todo",
	"deferred":    "todo",
	"pinned":      "todo",
	"in_progress": "doing",
	"hooked":      "doing",
	"blocked":     "doing",
	"closed":      "done",
}

var beadsBuiltinPriority = map[int]string{
	0: "critical",
	1: "high",
	2: "medium",
	3: "low",
	4: "low",
}

// bdExport is indirected so tests can stub without a real bd binary.
var bdExport = realBDExport

func realBDExport(dir string) ([]byte, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return nil, fmt.Errorf("the Beads CLI 'bd' is required: install it (https://github.com/gastownhall/beads) or pass a JSONL file")
	}
	cmd := exec.Command("bd", "export")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("bd export failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("bd export failed: %w", err)
	}
	return out, nil
}

func newImportBeadsCmd() *cobra.Command {
	var (
		state, label, typeMapCSV, statusMapCSV string
		limit                                  int
		dryRun, noEnsureTypes                  bool
	)
	cmd := &cobra.Command{
		Use:   "beads [file.jsonl|-]",
		Short: "Import Beads issues as Pine tickets (via bd export or JSONL)",
		Long: `Import Beads issues as Pine tickets.

With no argument, runs 'bd export' in the current project (requires the bd CLI).
Pass a JSONL file path, or '-' to read JSONL from stdin.

Issue types map to Pine prefixes (bug→BUG, task→TASK, epic→EPIC, …); missing
prefixes are added to config.json unless --no-ensure-types. Each imported ticket
records beads: <id> so re-running skips issues already imported.

Epic parent-child links become Pine parent (when the parent is an epic).
Blocking dependencies become Pine deps. Other Beads link types are listed in
the ticket body under Related.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}

			raw, err := loadBeadsJSONL(args)
			if err != nil {
				return err
			}
			issues, skippedNonIssue, err := parseBeadsJSONL(raw)
			if err != nil {
				return err
			}

			typeOverrides := parseTypeMap(typeMapCSV)
			statusOverrides := parseTypeMap(statusMapCSV) // same CSV k=v parser
			out := cmd.OutOrStdout()

			// Idempotency set from existing tickets.
			already := map[string]string{} // beadsID → pineID
			for _, t := range s.All() {
				if id := ticketBeadsID(t); id != "" {
					already[id] = t.ID
				}
			}

			// Filter + classify.
			var work []beadsIssue
			skippedInfra, skippedFilter, skippedImported := 0, 0, 0
			for _, iss := range issues {
				if iss.ID == "" || strings.TrimSpace(iss.Title) == "" {
					continue
				}
				typKey := strings.ToLower(strings.TrimSpace(iss.IssueType))
				if beadsInfraTypes[typKey] {
					skippedInfra++
					continue
				}
				if label != "" && !beadsHasLabel(iss, label) {
					skippedFilter++
					continue
				}
				if state != "all" {
					st := strings.ToLower(iss.Status)
					if state == "open" && st == "closed" {
						skippedFilter++
						continue
					}
					if state == "closed" && st != "closed" {
						skippedFilter++
						continue
					}
				}
				if _, ok := already[iss.ID]; ok {
					skippedImported++
					continue
				}
				work = append(work, iss)
			}
			if limit > 0 && len(work) > limit {
				work = work[:limit]
			}

			// Ensure type prefixes exist before create.
			needed := map[string]config.TicketType{}
			for _, iss := range work {
				prefix, tt, ok := resolveBeadsType(iss.IssueType, typeOverrides)
				if !ok {
					fmt.Fprintf(out, "  ! skip %s: unknown issue_type %q\n", iss.ID, iss.IssueType)
					continue
				}
				needed[prefix] = tt
			}
			if !dryRun && !noEnsureTypes {
				if err := ensureBeadsTypes(s, needed); err != nil {
					return err
				}
			}

			// Sort: epics first so parent links are valid after pass 1.
			sort.SliceStable(work, func(i, j int) bool {
				ei := strings.EqualFold(work[i].IssueType, "epic")
				ej := strings.EqualFold(work[j].IssueType, "epic")
				if ei != ej {
					return ei
				}
				return work[i].ID < work[j].ID
			})

			idMap := map[string]string{} // beads → pine (including already-imported)
			for k, v := range already {
				idMap[k] = v
			}
			prefixOf := map[string]string{} // pineID → prefix
			for _, t := range s.All() {
				prefixOf[t.ID] = t.Prefix()
			}

			created, failed := 0, 0
			var createdIssues []beadsIssue

			for _, iss := range work {
				prefix, _, ok := resolveBeadsType(iss.IssueType, typeOverrides)
				if !ok {
					failed++
					continue
				}
				if noEnsureTypes {
					if _, found := s.Config().TypeByPrefix(prefix); !found {
						fmt.Fprintf(out, "  ! skip %s: type %s not in config (use without --no-ensure-types)\n", iss.ID, prefix)
						failed++
						continue
					}
				}
				status := mapBeadsStatus(iss.Status, statusOverrides, s.Board())
				prio := mapBeadsPriority(iss.Priority)
				body := composeBeadsBody(iss)

				if dryRun {
					fmt.Fprintf(out, "  would import %s [%s] %s\n", iss.ID, prefix, iss.Title)
					created++
					createdIssues = append(createdIssues, iss)
					continue
				}

				t, err := s.Create(store.CreateReq{
					Type:     prefix,
					Title:    iss.Title,
					Priority: prio,
					Labels:   append([]string(nil), iss.Labels...),
					Status:   status,
					Body:     body,
				})
				if err != nil {
					fmt.Fprintf(out, "  ! skipped %s (%v)\n", iss.ID, err)
					failed++
					continue
				}
				extras := beadsExtraFields(iss)
				if _, err := s.Update(t.ID, func(tt *ticket.Ticket) error {
					tt.Extra = append(tt.Extra, extras...)
					return nil
				}); err != nil {
					fmt.Fprintf(out, "  ! %s created but could not record beads provenance (%v)\n", t.ID, err)
				}
				idMap[iss.ID] = t.ID
				prefixOf[t.ID] = t.Prefix()
				fmt.Fprintf(out, "  %s ← %s %s\n", t.ID, iss.ID, iss.Title)
				created++
				createdIssues = append(createdIssues, iss)
			}

			// Pass 2: wire parent + blocking deps (skip in dry-run).
			if !dryRun {
				wireBeadsRelations(s, out, createdIssues, idMap, prefixOf)
			}

			verb := "imported"
			if dryRun {
				verb = "would import"
			}
			fmt.Fprintf(out, "\n%s %d, skipped %d already-imported", verb, created, skippedImported)
			if skippedFilter > 0 {
				fmt.Fprintf(out, ", %d filtered", skippedFilter)
			}
			if skippedInfra > 0 {
				fmt.Fprintf(out, ", %d infra skipped", skippedInfra)
			}
			if skippedNonIssue > 0 {
				fmt.Fprintf(out, ", %d non-issue lines skipped", skippedNonIssue)
			}
			if failed > 0 {
				fmt.Fprintf(out, ", %d failed", failed)
			}
			fmt.Fprintln(out)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&state, "state", "all", "issue state filter: open | closed | all")
	f.StringVar(&label, "label", "", "only import issues carrying this label")
	f.StringVar(&typeMapCSV, "type-map", "", "issue_type→prefix overrides, e.g. task=FEAT,chore=CHORE")
	f.StringVar(&statusMapCSV, "status-map", "", "status overrides, e.g. open=todo,closed=done")
	f.IntVar(&limit, "limit", 0, "maximum number of new issues to import (0 = unlimited)")
	f.BoolVar(&dryRun, "dry-run", false, "show what would be imported without writing")
	f.BoolVar(&noEnsureTypes, "no-ensure-types", false, "do not auto-add missing type prefixes to config.json")
	return cmd
}

func loadBeadsJSONL(args []string) ([]byte, error) {
	if len(args) == 0 {
		return bdExport(flagDir)
	}
	path := args[0]
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func parseBeadsJSONL(data []byte) (issues []beadsIssue, skippedNonIssue int, err error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Beads issues can be large (comments); allow big lines.
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var iss beadsIssue
		if err := json.Unmarshal(line, &iss); err != nil {
			return nil, 0, fmt.Errorf("JSONL line %d: %w", lineNo, err)
		}
		typ := strings.ToLower(strings.TrimSpace(iss.RecordType))
		switch typ {
		case "memory":
			skippedNonIssue++
			continue
		case "issue", "":
			// Empty _type: treat as issue if it looks like one (legacy export).
			if typ == "" && iss.ID == "" {
				skippedNonIssue++
				continue
			}
			issues = append(issues, iss)
		default:
			skippedNonIssue++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	return issues, skippedNonIssue, nil
}

func resolveBeadsType(issueType string, overrides map[string]string) (prefix string, tt config.TicketType, ok bool) {
	key := strings.ToLower(strings.TrimSpace(issueType))
	if key == "" {
		key = "task"
	}
	if beadsInfraTypes[key] {
		return "", config.TicketType{}, false
	}
	if p, found := overrides[key]; found {
		p = strings.ToUpper(strings.TrimSpace(p))
		name := p
		if builtin, ok := beadsBuiltinTypes[key]; ok {
			name = builtin.Name
		}
		return p, config.TicketType{Prefix: p, Name: name}, true
	}
	if builtin, found := beadsBuiltinTypes[key]; found {
		return builtin.Prefix, builtin, true
	}
	// Unknown custom work type: uppercase as prefix if it looks valid.
	up := strings.ToUpper(key)
	up = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, up)
	if up == "" || up[0] < 'A' || up[0] > 'Z' {
		return "", config.TicketType{}, false
	}
	name := strings.ToUpper(key[:1]) + key[1:]
	return up, config.TicketType{Prefix: up, Name: name}, true
}

func ensureBeadsTypes(s *store.Store, needed map[string]config.TicketType) error {
	cfg := s.Config()
	var added []config.TicketType
	for prefix, tt := range needed {
		if _, ok := cfg.TypeByPrefix(prefix); ok {
			continue
		}
		added = append(added, tt)
	}
	if len(added) == 0 {
		return nil
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Prefix < added[j].Prefix })
	newCfg := *cfg
	newCfg.Types = append(append([]config.TicketType(nil), cfg.Types...), added...)
	newCfg.Extra = cfg.Extra
	return s.SaveConfig(&newCfg)
}

func mapBeadsStatus(status string, overrides map[string]string, board *config.Board) string {
	key := strings.ToLower(strings.TrimSpace(status))
	var mapped string
	if v, ok := overrides[key]; ok {
		mapped = strings.ToLower(strings.TrimSpace(v))
	} else if v, ok := beadsBuiltinStatus[key]; ok {
		mapped = v
	} else if key != "" {
		mapped = key
	}
	if mapped == "" || board == nil || !board.HasStatus(mapped) {
		if board != nil {
			return board.FirstStatus()
		}
		return "todo"
	}
	return mapped
}

func mapBeadsPriority(p int) string {
	if v, ok := beadsBuiltinPriority[p]; ok {
		return v
	}
	return "medium"
}

func composeBeadsBody(iss beadsIssue) string {
	var b strings.Builder
	writeSection := func(heading, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		b.WriteString("\n# ")
		b.WriteString(heading)
		b.WriteString("\n\n")
		b.WriteString(content)
		b.WriteString("\n")
	}
	writeSection("Description", iss.Description)
	writeSection("Design", iss.Design)
	writeSection("Acceptance Criteria", iss.AcceptanceCriteria)
	writeSection("Notes", iss.Notes)

	var related []string
	for _, d := range iss.Dependencies {
		dt := strings.ToLower(strings.TrimSpace(d.Type))
		switch dt {
		case "blocks", "conditional-blocks", "waits-for", "parent-child":
			continue
		case "":
			continue
		default:
			target := d.DependsOnID
			if target == "" {
				continue
			}
			related = append(related, fmt.Sprintf("- %s → %s", dt, target))
		}
	}
	if len(related) > 0 {
		b.WriteString("\n# Related\n\n")
		b.WriteString(strings.Join(related, "\n"))
		b.WriteString("\n")
	}

	if len(iss.Comments) > 0 {
		b.WriteString("\n# Comments\n\n")
		for _, c := range iss.Comments {
			author := c.Author
			if author == "" {
				author = "unknown"
			}
			ts := ""
			if !c.CreatedAt.IsZero() {
				ts = c.CreatedAt.UTC().Format(time.RFC3339)
			}
			if ts != "" {
				fmt.Fprintf(&b, "- **%s** (%s): %s\n", author, ts, strings.TrimSpace(c.Text))
			} else {
				fmt.Fprintf(&b, "- **%s**: %s\n", author, strings.TrimSpace(c.Text))
			}
		}
	}

	out := b.String()
	if out == "" {
		return "\n"
	}
	return out
}

func beadsExtraFields(iss beadsIssue) []ticket.ExtraField {
	var extras []ticket.ExtraField
	add := func(key, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		extras = append(extras, ticket.ExtraField{Key: key, Node: frontmatter.Scalar(val)})
	}
	add("beads", iss.ID)
	add("beads_assignee", iss.Assignee)
	add("beads_owner", iss.Owner)
	add("beads_close_reason", iss.CloseReason)
	if iss.ExternalRef != nil {
		add("beads_external_ref", *iss.ExternalRef)
	}
	if !iss.CreatedAt.IsZero() {
		add("beads_created", iss.CreatedAt.UTC().Format(time.RFC3339))
	}
	return extras
}

func wireBeadsRelations(s *store.Store, out io.Writer, issues []beadsIssue, idMap, prefixOf map[string]string) {
	for _, iss := range issues {
		pineID, ok := idMap[iss.ID]
		if !ok {
			continue
		}
		parentBeads := beadsParentID(iss)
		var parentPine string
		if parentBeads != "" {
			if pid, ok := idMap[parentBeads]; ok && prefixOf[pid] == "EPIC" {
				parentPine = pid
			}
		}

		var deps []string
		seen := map[string]bool{}
		for _, d := range iss.Dependencies {
			dt := strings.ToLower(strings.TrimSpace(d.Type))
			switch dt {
			case "blocks", "conditional-blocks", "waits-for":
				target := d.DependsOnID
				if target == "" || target == iss.ID {
					continue
				}
				pid, ok := idMap[target]
				if !ok || pid == pineID || seen[pid] {
					continue
				}
				seen[pid] = true
				deps = append(deps, pid)
			}
		}

		if parentPine == "" && len(deps) == 0 {
			continue
		}

		safeDeps := make([]string, 0, len(deps))
		for _, dep := range deps {
			trial := append(append([]string(nil), safeDeps...), dep)
			if cyc := wouldCycle(s, pineID, trial); cyc != nil {
				fmt.Fprintf(out, "  ! %s: skip dep %s (would create cycle)\n", pineID, dep)
				continue
			}
			safeDeps = append(safeDeps, dep)
		}

		if _, err := s.Update(pineID, func(tt *ticket.Ticket) error {
			if parentPine != "" {
				tt.Parent = parentPine
			}
			if len(safeDeps) > 0 {
				tt.Deps = mergeDeps(tt.Deps, safeDeps)
			}
			return nil
		}); err != nil {
			fmt.Fprintf(out, "  ! %s: could not set relations (%v)\n", pineID, err)
		}
	}
}

func beadsParentID(iss beadsIssue) string {
	if iss.Parent != nil && strings.TrimSpace(*iss.Parent) != "" {
		return strings.TrimSpace(*iss.Parent)
	}
	for _, d := range iss.Dependencies {
		if strings.EqualFold(d.Type, "parent-child") && d.DependsOnID != "" {
			return d.DependsOnID
		}
	}
	return ""
}

func ticketBeadsID(t *ticket.Ticket) string {
	for _, e := range t.Extra {
		if e.Key == "beads" && e.Node != nil {
			return strings.TrimSpace(e.Node.Value)
		}
	}
	return ""
}

func beadsHasLabel(iss beadsIssue, want string) bool {
	want = strings.ToLower(want)
	for _, l := range iss.Labels {
		if strings.ToLower(l) == want {
			return true
		}
	}
	return false
}
