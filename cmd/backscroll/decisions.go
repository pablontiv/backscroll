package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newDecisionsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decisions",
		Short: "Query and analyze indexed decision records",
		Long:  `Commands for querying, extracting, and analyzing decision records from the index.`,
	}
	cmd.AddCommand(
		newDecisionsQueryCmd(stdout),
		newDecisionsContextCmd(stdout),
		newDecisionsExtractCmd(stdout),
		newDecisionsConflictsCmd(stdout),
		newDecisionsReplayCmd(stdout),
	)
	return cmd
}

func openDecisionDB(cfg *config.Config) (*storage.Database, error) {
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}

// decisionFrontmatter parses simple key: value YAML frontmatter into a map.
func decisionFrontmatter(text string) map[string]string {
	fm := make(map[string]string)
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "---") {
		return fm
	}
	rest := trimmed[3:]
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		return fm
	}
	yamlPart := rest[:closeIdx]
	for _, line := range strings.Split(yamlPart, "\n") {
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])
		if key != "" {
			fm[key] = val
		}
	}
	return fm
}

// decisionMetadata extracts id, title, status, scope, and is_accepted from a record.
func decisionMetadata(text, sourcePath string) (id, title, status string, scope *string, isAccepted bool) {
	fm := decisionFrontmatter(text)

	id = fm["id"]
	status = fm["status"]
	if status == "" {
		status = "proposed"
	}
	if s, ok := fm["scope"]; ok && s != "" {
		scope = &s
	}
	isAccepted = strings.EqualFold(status, "accepted")

	// Title from first heading or fallback to filename
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			title = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			break
		}
	}
	if title == "" {
		base := filepath.Base(sourcePath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return
}

func computeFreshness(timestamp *string) string {
	if timestamp == nil {
		return "unknown"
	}
	ts := *timestamp
	if strings.HasPrefix(ts, "2026") {
		return "active"
	}
	if strings.HasPrefix(ts, "2025") || strings.HasPrefix(ts, "202") {
		return "stale"
	}
	return "unknown"
}

func normalizeStatement(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	return strings.Join(words, " ")
}

func computeClusterID(statement string) string {
	norm := normalizeStatement(statement)
	runes := []rune(norm)
	if len(runes) > 40 {
		runes = runes[:40]
	}
	return string(runes)
}

func matchesDecisionPattern(line string) bool {
	lower := strings.ToLower(line)
	patterns := []string{
		"we decided to ", "decision: ", "decided: ",
		"we have decided ", "the decision is ",
		"we will use ", "we are using ", "we should ",
		"we need to ", "going forward ",
		"we will not ", "we must not ",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func confidenceForSnippet(snippet string) float64 {
	lower := strings.ToLower(snippet)
	switch {
	case strings.HasPrefix(lower, "we decided to "),
		strings.HasPrefix(lower, "decision: "),
		strings.HasPrefix(lower, "decided: "),
		strings.HasPrefix(lower, "we will not "),
		strings.HasPrefix(lower, "we must not "):
		return 0.90
	case strings.HasPrefix(lower, "we will use "),
		strings.HasPrefix(lower, "we are using "),
		strings.HasPrefix(lower, "we have decided "),
		strings.HasPrefix(lower, "the decision is "):
		return 0.75
	case strings.HasPrefix(lower, "we need to "),
		strings.HasPrefix(lower, "going forward "):
		return 0.60
	case strings.HasPrefix(lower, "we should ") && !strings.HasPrefix(lower, "we should not "):
		return 0.60
	default:
		return 0.0
	}
}

func extractStatementFromSnippet(snippet string) string {
	lower := strings.ToLower(snippet)
	prefixes := []string{
		"we decided to ", "decision: ", "decided: ",
		"we have decided to ", "the decision is ",
		"we will use ", "we are using ",
		"we should ", "we need to ", "going forward ",
		"we will not ", "we must not ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(snippet[len(p):])
		}
	}
	return strings.TrimSpace(snippet)
}

func extractSignificantWords(text string) []string {
	var words []string
	for _, w := range strings.Fields(normalizeStatement(text)) {
		if len(w) > 4 {
			words = append(words, w)
		}
	}
	return words
}

func countKeywordOverlap(s1, s2 string) int {
	w1 := extractSignificantWords(s1)
	w2 := extractSignificantWords(s2)
	count := 0
	for _, w := range w1 {
		for _, w2w := range w2 {
			if w == w2w {
				count++
				break
			}
		}
	}
	return count
}

// ---- decisions query ----

type decisionRecord struct {
	ID         string                 `json:"id,omitempty"`
	Title      string                 `json:"title"`
	Status     string                 `json:"status"`
	Scope      *string                `json:"scope,omitempty"`
	SourcePath string                 `json:"source_path"`
	Provenance map[string]interface{} `json:"provenance"`
	IsAccepted bool                   `json:"is_accepted"`
	Excerpt    string                 `json:"excerpt"`
	Freshness  string                 `json:"freshness,omitempty"`
	LastSeen   *string                `json:"last_seen,omitempty"`
}

func newDecisionsQueryCmd(stdout io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		statusFilt  string
		scopeFilt   string
		jsonOut     bool
		limit       int
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query indexed decision records",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			db, err := openDecisionDB(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			proj := effectiveProject(project, allProjects)
			var projPtr *string
			if proj != "" {
				projPtr = &proj
			}
			src := "decision"
			records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
				Project: projPtr,
				Source:  &src,
				Limit:   limit,
			})
			if err != nil {
				return fmt.Errorf("query records: %w", err)
			}

			count := 0
			for _, rec := range records {
				id, title, status, scope, isAccepted := decisionMetadata(rec.Text, rec.SourcePath)

				if statusFilt != "" && !strings.EqualFold(status, statusFilt) {
					continue
				}
				if scopeFilt != "" {
					if scope == nil || !strings.EqualFold(*scope, scopeFilt) {
						continue
					}
				}

				runes := []rune(rec.Text)
				if len(runes) > 200 {
					runes = runes[:200]
				}
				excerpt := string(runes)
				freshness := computeFreshness(rec.Timestamp)

				dr := decisionRecord{
					ID:         id,
					Title:      title,
					Status:     status,
					Scope:      scope,
					SourcePath: rec.SourcePath,
					Provenance: map[string]interface{}{
						"source":    rec.Source,
						"timestamp": rec.Timestamp,
						"ordinal":   rec.Ordinal,
					},
					IsAccepted: isAccepted,
					Excerpt:    excerpt,
					Freshness:  freshness,
					LastSeen:   rec.Timestamp,
				}

				if jsonOut {
					if err := json.NewEncoder(stdout).Encode(dr); err != nil {
						return err
					}
				} else {
					indicator := "[ ]"
					if isAccepted {
						indicator = "[v]"
					}
					scopeStr := "unspecified"
					if scope != nil {
						scopeStr = *scope
					}
					_, _ = fmt.Fprintf(stdout, "%s %s\n   Status: %s | Scope: %s | Freshness: %s | %s\n\n",
						indicator, title, status, scopeStr, freshness, rec.SourcePath)
				}
				count++
			}

			if !jsonOut && count == 0 {
				_, _ = fmt.Fprintln(stdout, "No decisions found.")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().StringVar(&statusFilt, "status", "", "Filter by status (accepted, proposed, rejected, ...)")
	cmd.Flags().StringVar(&scopeFilt, "scope", "", "Filter by scope (technical, architectural, ...)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON Lines")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum records (0 = no limit)")
	return cmd
}

// ---- decisions context ----

type decisionContext struct {
	Project     *string          `json:"project"`
	Decisions   []decisionRecord `json:"decisions"`
	TotalTokens int              `json:"total_tokens"`
}

func newDecisionsContextCmd(stdout io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		maxTokens   int
		jsonOut     bool
	)
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Get bounded decision context for LLM injection",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			db, err := openDecisionDB(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			proj := effectiveProject(project, allProjects)
			var projPtr *string
			if proj != "" {
				projPtr = &proj
			}
			src := "decision"
			records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
				Project: projPtr,
				Source:  &src,
			})
			if err != nil {
				return fmt.Errorf("query records: %w", err)
			}

			var decisions []decisionRecord
			totalTokens := 0

			for _, rec := range records {
				id, title, status, scope, isAccepted := decisionMetadata(rec.Text, rec.SourcePath)

				maxChars := maxTokens * 4
				runes := []rune(rec.Text)
				if len(runes) > maxChars {
					runes = runes[:maxChars]
				}
				excerpt := string(runes)
				tokenEst := len(strings.Fields(excerpt)) + 10
				if totalTokens+tokenEst > maxTokens {
					break
				}

				freshness := computeFreshness(rec.Timestamp)
				dr := decisionRecord{
					ID:         id,
					Title:      title,
					Status:     status,
					Scope:      scope,
					SourcePath: rec.SourcePath,
					Provenance: map[string]interface{}{
						"source":    rec.Source,
						"timestamp": rec.Timestamp,
						"ordinal":   rec.Ordinal,
					},
					IsAccepted: isAccepted,
					Excerpt:    excerpt,
					Freshness:  freshness,
					LastSeen:   rec.Timestamp,
				}
				totalTokens += tokenEst
				decisions = append(decisions, dr)
			}

			ctx := decisionContext{
				Project:     projPtr,
				Decisions:   decisions,
				TotalTokens: totalTokens,
			}
			if decisions == nil {
				ctx.Decisions = []decisionRecord{}
			}

			if jsonOut {
				return json.NewEncoder(stdout).Encode(ctx)
			}

			projLabel := "(all)"
			if projPtr != nil {
				projLabel = *projPtr
			}
			_, _ = fmt.Fprintf(stdout, "Decision Context (approx %d tokens)\n", totalTokens)
			_, _ = fmt.Fprintf(stdout, "Project: %s\n", projLabel)
			_, _ = fmt.Fprintf(stdout, "Decisions: %d\n\n", len(decisions))
			for _, d := range decisions {
				indicator := "[ ]"
				if d.IsAccepted {
					indicator = "[v]"
				}
				scopeStr := "unspecified"
				if d.Scope != nil {
					scopeStr = *d.Scope
				}
				_, _ = fmt.Fprintf(stdout, "%s %s\n   Status: %s | Scope: %s\n\n",
					indicator, d.Title, d.Status, scopeStr)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 8000, "Maximum tokens for context")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	return cmd
}

// ---- decisions extract ----

type candidateEvidence struct {
	SourcePath string  `json:"source_path"`
	Snippet    string  `json:"snippet"`
	Timestamp  *string `json:"timestamp,omitempty"`
}

type decisionCandidate struct {
	Statement  string              `json:"statement"`
	Status     string              `json:"status"`
	Confidence float64             `json:"confidence"`
	ClusterID  string              `json:"cluster_id"`
	Evidence   []candidateEvidence `json:"evidence"`
}

func newDecisionsExtractCmd(stdout io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		since       string
		limit       int
	)
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract decision candidates from indexed session records using heuristics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			db, err := openDecisionDB(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			proj := effectiveProject(project, allProjects)
			var projPtr *string
			if proj != "" {
				projPtr = &proj
			}
			src := "session"
			records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
				Project: projPtr,
				Source:  &src,
			})
			if err != nil {
				return fmt.Errorf("query records: %w", err)
			}

			type clusterEntry struct {
				maxConf  float64
				evidence []candidateEvidence
			}
			clusters := make(map[string]*clusterEntry)

			for _, rec := range records {
				if since != "" && rec.Timestamp != nil && *rec.Timestamp < since {
					continue
				}
				if since != "" && rec.Timestamp == nil {
					continue
				}

				for _, line := range strings.Split(rec.Text, "\n") {
					trimmed := strings.TrimSpace(line)
					if len(trimmed) < 20 || len(trimmed) > 500 {
						continue
					}
					if !matchesDecisionPattern(trimmed) {
						continue
					}
					conf := confidenceForSnippet(trimmed)
					if conf <= 0.0 {
						continue
					}
					statement := extractStatementFromSnippet(trimmed)
					clusterID := computeClusterID(statement)

					ev := candidateEvidence{
						SourcePath: rec.SourcePath,
						Snippet:    trimmed,
						Timestamp:  rec.Timestamp,
					}

					if e, ok := clusters[clusterID]; ok {
						if conf > e.maxConf {
							e.maxConf = conf
						}
						e.evidence = append(e.evidence, ev)
					} else {
						clusters[clusterID] = &clusterEntry{maxConf: conf, evidence: []candidateEvidence{ev}}
					}
				}
			}

			var candidates []decisionCandidate
			for clusterID, entry := range clusters {
				stmt := ""
				if len(entry.evidence) > 0 {
					stmt = extractStatementFromSnippet(entry.evidence[0].Snippet)
				}
				candidates = append(candidates, decisionCandidate{
					Statement:  stmt,
					Status:     "candidate",
					Confidence: entry.maxConf,
					ClusterID:  clusterID,
					Evidence:   entry.evidence,
				})
			}

			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].Confidence != candidates[j].Confidence {
					return candidates[i].Confidence > candidates[j].Confidence
				}
				return candidates[i].ClusterID < candidates[j].ClusterID
			})

			if limit > 0 && len(candidates) > limit {
				candidates = candidates[:limit]
			}

			for _, c := range candidates {
				if err := json.NewEncoder(stdout).Encode(c); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().StringVar(&since, "since", "", "Only sessions since this date (ISO 8601, e.g. 2026-01-01)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum candidates to emit (0 = no limit)")
	return cmd
}

// ---- decisions conflicts ----

type conflictHint struct {
	ConflictType       string  `json:"conflict_type"`
	ExistingDecisionID *string `json:"existing_decision_id,omitempty"`
	ExistingStatement  string  `json:"existing_statement"`
	ExistingStatus     string  `json:"existing_status"`
	SourcePath         string  `json:"source_path"`
	Explanation        string  `json:"explanation"`
}

type proposalInput struct {
	ID        *string `json:"id"`
	Statement string  `json:"statement"`
	Status    *string `json:"status"`
	Scope     *string `json:"scope"`
}

func detectConflicts(proposal proposalInput, existing []struct {
	id, text, status string
	scope            *string
	sourcePath       string
}) []conflictHint {
	var hints []conflictHint
	propNorm := normalizeStatement(proposal.Statement)
	propPrefix := propNorm
	if len([]rune(propPrefix)) > 60 {
		propPrefix = string([]rune(propPrefix)[:60])
	}

	for _, ex := range existing {
		exNorm := normalizeStatement(ex.text)
		exPrefix := exNorm
		if len([]rune(exPrefix)) > 60 {
			exPrefix = string([]rune(exPrefix)[:60])
		}

		// Superseded check
		if strings.EqualFold(ex.status, "superseded") && proposal.ID != nil && *proposal.ID == ex.id {
			hints = append(hints, conflictHint{
				ConflictType:       "superseded",
				ExistingDecisionID: &ex.id,
				ExistingStatement:  ex.text,
				ExistingStatus:     ex.status,
				SourcePath:         ex.sourcePath,
				Explanation:        fmt.Sprintf("proposal is replacing a known superseded record with id %s", *proposal.ID),
			})
			continue
		}

		// Duplicate check
		if propPrefix == exPrefix {
			id := ex.id
			hints = append(hints, conflictHint{
				ConflictType:       "duplicate",
				ExistingDecisionID: &id,
				ExistingStatement:  ex.text,
				ExistingStatus:     ex.status,
				SourcePath:         ex.sourcePath,
				Explanation:        "proposal statement matches an existing decision (normalized prefix)",
			})
			continue
		}

		// Potential conflict: accepted + same scope + keyword overlap
		if strings.EqualFold(ex.status, "accepted") {
			scopesMatch := false
			switch {
			case proposal.Scope == nil && ex.scope == nil:
				scopesMatch = true
			case proposal.Scope != nil && ex.scope != nil:
				scopesMatch = strings.EqualFold(*proposal.Scope, *ex.scope)
			}
			if scopesMatch && countKeywordOverlap(proposal.Statement, ex.text) >= 2 {
				id := ex.id
				hints = append(hints, conflictHint{
					ConflictType:       "potential_conflict",
					ExistingDecisionID: &id,
					ExistingStatement:  ex.text,
					ExistingStatus:     ex.status,
					SourcePath:         ex.sourcePath,
					Explanation: fmt.Sprintf(
						"accepted decision with same scope may conflict (%d keywords overlap)",
						countKeywordOverlap(proposal.Statement, ex.text)),
				})
			}
		}
	}
	return hints
}

func newDecisionsConflictsCmd(stdout io.Writer) *cobra.Command {
	var (
		proposalJSON string
		project      string
		allProjects  bool
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "conflicts",
		Short: "Detect conflicts between a proposed decision and the indexed corpus",
		RunE: func(cmd *cobra.Command, args []string) error {
			var rawProposal string
			if proposalJSON != "" {
				rawProposal = proposalJSON
			} else {
				buf := new(strings.Builder)
				b := make([]byte, 4096)
				for {
					n, err := os.Stdin.Read(b)
					if n > 0 {
						buf.Write(b[:n])
					}
					if err != nil {
						break
					}
				}
				rawProposal = buf.String()
			}
			var proposal proposalInput
			if err := json.Unmarshal([]byte(rawProposal), &proposal); err != nil {
				return fmt.Errorf("parse proposal JSON: %w", err)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			db, err := openDecisionDB(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			proj := effectiveProject(project, allProjects)
			var projPtr *string
			if proj != "" {
				projPtr = &proj
			}
			src := "decision"
			records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
				Project: projPtr,
				Source:  &src,
			})
			if err != nil {
				return fmt.Errorf("query records: %w", err)
			}

			type exEntry struct {
				id, text, status string
				scope            *string
				sourcePath       string
			}
			var existing []struct {
				id, text, status string
				scope            *string
				sourcePath       string
			}
			for _, rec := range records {
				id, _, status, scope, _ := decisionMetadata(rec.Text, rec.SourcePath)
				existing = append(existing, exEntry{
					id: id, text: rec.Text, status: status, scope: scope, sourcePath: rec.SourcePath,
				})
			}

			hints := detectConflicts(proposal, existing)

			if jsonOut {
				if hints == nil {
					hints = []conflictHint{}
				}
				return json.NewEncoder(stdout).Encode(hints)
			}
			if len(hints) == 0 {
				_, _ = fmt.Fprintln(stdout, "No conflicts found.")
				return nil
			}
			scopeStr := "unspecified"
			if proposal.Scope != nil {
				scopeStr = *proposal.Scope
			}
			_, _ = fmt.Fprintf(stdout, "Conflict Analysis:\nProposal: %s (scope: %s)\n\n",
				proposal.Statement, scopeStr)
			for i, h := range hints {
				idStr := "(no id)"
				if h.ExistingDecisionID != nil {
					idStr = *h.ExistingDecisionID
				}
				_, _ = fmt.Fprintf(stdout, "%d. [%s] %s at %s\n   Statement: %s\n   Status: %s\n   Note: %s\n\n",
					i+1, strings.ToUpper(h.ConflictType), idStr, h.SourcePath,
					h.ExistingStatement, h.ExistingStatus, h.Explanation)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&proposalJSON, "proposal-json", "", "JSON string with proposal (or read from stdin)")
	cmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON output")
	return cmd
}

// ---- decisions replay ----

type coveredDecision struct {
	ID         *string `json:"id,omitempty"`
	Statement  string  `json:"statement"`
	SourcePath string  `json:"source_path"`
}

type missedDecision struct {
	ID        *string `json:"id,omitempty"`
	Statement string  `json:"statement"`
}

type staleDecision struct {
	ID         *string `json:"id,omitempty"`
	Statement  string  `json:"statement"`
	SourcePath string  `json:"source_path"`
}

type conflictDecision struct {
	ID             *string `json:"id,omitempty"`
	Statement      string  `json:"statement"`
	ExpectedStatus string  `json:"expected_status"`
	FoundStatus    string  `json:"found_status"`
	SourcePath     string  `json:"source_path"`
}

type replayReport struct {
	Fixture       string             `json:"fixture"`
	CorpusRecords int                `json:"corpus_records"`
	Expected      int                `json:"expected"`
	Covered       []coveredDecision  `json:"covered"`
	Missed        []missedDecision   `json:"missed"`
	Stale         []staleDecision    `json:"stale"`
	Conflicts     []conflictDecision `json:"conflicts"`
	SourceCounts  map[string]int     `json:"source_counts"`
	CoveragePct   float64            `json:"coverage_pct"`
}

type fixtureDecision struct {
	ID        *string `json:"id"`
	Statement string  `json:"statement"`
	Status    *string `json:"status"`
}

type replayFixture struct {
	ExpectedDecisions []fixtureDecision `json:"expected_decisions"`
}

func newDecisionsReplayCmd(stdout io.Writer) *cobra.Command {
	var (
		fixturePath string
		project     string
		allProjects bool
		jsonOut     bool
	)
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Evaluate fixture coverage in indexed decision corpus",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			db, err := openDecisionDB(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			proj := effectiveProject(project, allProjects)
			var projPtr *string
			if proj != "" {
				projPtr = &proj
			}
			records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
				Project: projPtr,
			})
			if err != nil {
				return fmt.Errorf("query records: %w", err)
			}

			fixtureData, err := os.ReadFile(fixturePath)
			if err != nil {
				return fmt.Errorf("read fixture: %w", err)
			}
			var fixture replayFixture
			if err := json.Unmarshal(fixtureData, &fixture); err != nil {
				return fmt.Errorf("parse fixture: %w", err)
			}

			sourceCounts := make(map[string]int)
			for _, rec := range records {
				sourceCounts[rec.Source]++
			}

			var covered []coveredDecision
			var missed []missedDecision
			var stale []staleDecision
			var conflicts []conflictDecision

			for _, expected := range fixture.ExpectedDecisions {
				expNorm := normalizeStatement(expected.Statement)
				var foundSourcePath string
				var foundStatus *string
				found := false

				for _, rec := range records {
					recNorm := normalizeStatement(rec.Text)
					if strings.Contains(recNorm, expNorm) {
						found = true
						foundSourcePath = rec.SourcePath
						if rec.Source == "decision" {
							_, _, status, _, _ := decisionMetadata(rec.Text, rec.SourcePath)
							foundStatus = &status
						}
						break
					}
				}

				if !found {
					missed = append(missed, missedDecision{ID: expected.ID, Statement: expected.Statement})
					continue
				}

				if foundStatus != nil && strings.EqualFold(*foundStatus, "superseded") {
					stale = append(stale, staleDecision{
						ID: expected.ID, Statement: expected.Statement, SourcePath: foundSourcePath,
					})
					continue
				}

				if expected.Status != nil && foundStatus != nil &&
					!strings.EqualFold(*expected.Status, *foundStatus) {
					conflicts = append(conflicts, conflictDecision{
						ID: expected.ID, Statement: expected.Statement,
						ExpectedStatus: *expected.Status, FoundStatus: *foundStatus,
						SourcePath: foundSourcePath,
					})
					continue
				}

				covered = append(covered, coveredDecision{
					ID: expected.ID, Statement: expected.Statement, SourcePath: foundSourcePath,
				})
			}

			expectedCount := len(fixture.ExpectedDecisions)
			coveragePct := 100.0
			if expectedCount > 0 {
				coveragePct = float64(len(covered)) / float64(expectedCount) * 100.0
			}

			report := replayReport{
				Fixture:       fixturePath,
				CorpusRecords: len(records),
				Expected:      expectedCount,
				Covered:       covered,
				Missed:        missed,
				Stale:         stale,
				Conflicts:     conflicts,
				SourceCounts:  sourceCounts,
				CoveragePct:   coveragePct,
			}
			if report.Covered == nil {
				report.Covered = []coveredDecision{}
			}
			if report.Missed == nil {
				report.Missed = []missedDecision{}
			}
			if report.Stale == nil {
				report.Stale = []staleDecision{}
			}
			if report.Conflicts == nil {
				report.Conflicts = []conflictDecision{}
			}

			if jsonOut {
				return json.NewEncoder(stdout).Encode(report)
			}
			_, _ = fmt.Fprintf(stdout, "Replay Report: %s\n", report.Fixture)
			_, _ = fmt.Fprintf(stdout, "Corpus records: %d\n", report.CorpusRecords)
			_, _ = fmt.Fprintf(stdout, "Expected decisions: %d\n", report.Expected)
			_, _ = fmt.Fprintf(stdout, "Coverage: %.1f%%\n\n", report.CoveragePct)

			if len(report.Covered) > 0 {
				_, _ = fmt.Fprintf(stdout, "Covered (%d):\n", len(report.Covered))
				for _, d := range report.Covered {
					_, _ = fmt.Fprintf(stdout, "  + %s\n", d.Statement)
				}
				_, _ = fmt.Fprintln(stdout)
			}
			if len(report.Missed) > 0 {
				_, _ = fmt.Fprintf(stdout, "Missed (%d):\n", len(report.Missed))
				for _, d := range report.Missed {
					_, _ = fmt.Fprintf(stdout, "  - %s\n", d.Statement)
				}
				_, _ = fmt.Fprintln(stdout)
			}
			if len(report.Stale) > 0 {
				_, _ = fmt.Fprintf(stdout, "Stale (%d):\n", len(report.Stale))
				for _, d := range report.Stale {
					_, _ = fmt.Fprintf(stdout, "  ~ %s\n", d.Statement)
				}
				_, _ = fmt.Fprintln(stdout)
			}
			if len(report.Conflicts) > 0 {
				_, _ = fmt.Fprintf(stdout, "Conflicts (%d):\n", len(report.Conflicts))
				for _, d := range report.Conflicts {
					_, _ = fmt.Fprintf(stdout, "  ! %s (expected: %s, found: %s)\n",
						d.Statement, d.ExpectedStatus, d.FoundStatus)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fixturePath, "fixture", "", "Path to fixture file (JSON)")
	if err := cmd.MarkFlagRequired("fixture"); err != nil {
		panic(err)
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON")
	return cmd
}
