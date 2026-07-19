package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/sequences"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newPatternsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		kind          string
		project       string
		allProjects   bool
		tag           string
		limit         int
		offset        int
		jsonFormat    bool
		robotFormat   bool
		indexedOnly   bool
		minSupport    int
		minConfidence float64
		pending       bool
		batch         int
		minLength     int
		maxLength     int
		after         string
		before        string
		trend         bool
	)

	cmd := &cobra.Command{
		Use:   "patterns",
		Short: "Discover deterministic patterns in tool events, templates, sequences, and corrections",
		Long: `Patterns computes census aggregations over tool events (commands, failures),
error message templates, frequent tool-call sequences (PrefixSpan mining),
and message-level corrections to expose actionable pattern candidates
for agent loop classification.

Use --kind to select aggregation type (required; supported: commands, failures, templates, sequences, corrections).
Use --project to filter to a single project.
Use --tag to filter by session tags (e.g., debugging, testing).
Use --min-support for template/sequence filtering (default 3; minimum occurrences).
Use --min-length, --max-length for sequence pattern length bounds (default 2, 6).
Use --min-confidence for correction filtering (default 0.6; detector confidence threshold).
Use --limit, --offset for pagination.
Use --json, --robot for output formats.
Use --indexed-only to skip auto-sync (read existing index only).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected positional argument %q", args[0])
			}
			return runPatterns(stdout, stderr, kind, project, allProjects, tag, limit, offset, jsonFormat, robotFormat, indexedOnly, minSupport, minConfidence, pending, batch, minLength, maxLength, after, before, trend)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Aggregation kind (required: commands|failures|templates|sequences|corrections)")
	cmd.Flags().StringVar(&project, "project", "", "Filter to single project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query across all projects (default if --project not set)")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by session tag")
	cmd.Flags().IntVar(&limit, "limit", 20, "Result limit")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().IntVar(&minSupport, "min-support", 3, "Minimum template/sequence occurrence count")
	cmd.Flags().IntVar(&minLength, "min-length", 2, "Minimum sequence pattern length (for --kind sequences, default 2)")
	cmd.Flags().IntVar(&maxLength, "max-length", 6, "Maximum sequence pattern length (for --kind sequences, default 6; prevents combinatorial explosion)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0.6, "Minimum detector confidence (for --kind corrections)")
	cmd.Flags().StringVar(&after, "after", "", "Filter after date (ISO 8601, for --kind sequences)")
	cmd.Flags().StringVar(&before, "before", "", "Filter before date (ISO 8601, for --kind sequences)")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	cmd.Flags().BoolVar(&pending, "pending", false, "Only corrections without a 'correction' annotation (checkpoint resume)")
	cmd.Flags().IntVar(&batch, "batch", 0, "Alias for --limit (batch size for loop)")
	cmd.Flags().BoolVar(&trend, "trend", false, "Week-over-week bucketing (--kind commands|failures only)")

	cmd.MarkFlagRequired("kind")

	return cmd
}

func runPatterns(stdout, stderr io.Writer,
	kind string, project string, allProjects bool, tag string,
	limit, offset int, jsonFormat, robotFormat, indexedOnly bool, minSupport int, minConfidence float64, pending bool, batch int,
	minLength, maxLength int, after, before string, trend bool) error {

	// Early flag validation before DB open
	validKinds := map[string]bool{
		"commands":    true,
		"failures":    true,
		"templates":   true,
		"sequences":   true,
		"corrections": true,
	}
	if !validKinds[kind] {
		return fmt.Errorf("unsupported --kind %q (supported: commands, failures, templates, sequences, corrections)", kind)
	}

	if trend && kind != "commands" && kind != "failures" {
		return fmt.Errorf("--trend only supported for --kind commands|failures, got %q", kind)
	}

	if project != "" && allProjects {
		return fmt.Errorf("--project and --all-projects are mutually exclusive")
	}

	if limit < 0 || offset < 0 {
		return fmt.Errorf("--limit and --offset must be >= 0")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Auto-sync unless --indexed-only
	if !indexedOnly {
		if err := maybeAutoSync(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: auto-sync failed: %v; using cached index\n", err)
		}
	}

	// Derive effective project
	if project == "" && !allProjects {
		project = effectiveProject(project, allProjects)
	}

	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	opts := storage.AggregateOptions{
		Project:     project,
		Tag:         tag,
		Limit:       limit,
		Offset:      offset,
		TrendWeekly: trend,
	}

	// Execute aggregation
	if kind == "commands" {
		if trend {
			var excluded int
			results, err := db.AggregateCommandsTrend(opts, &excluded)
			if err != nil {
				return fmt.Errorf("aggregate commands trend: %w", err)
			}

			if excluded > 0 {
				_, _ = fmt.Fprintf(stderr, "trend: %d events excluded (no timestamp)\n", excluded)
			}

			if len(results) == 0 {
				if !jsonFormat && !robotFormat {
					_, _ = fmt.Fprintf(stdout, "No patterns found. Try --min-support <lower> or --min-length <lower>.\n")
				} else if jsonFormat {
					_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"kind\":\"commands\",\"trends\":[]}\n")
				}
				writeSearchHints(stderr, allProjects, true)
				return nil
			}

			// Group results by week for text/json output
			weekMap := make(map[string][]storage.CommandPatternWeekly)
			var weeks []string
			for _, r := range results {
				if _, exists := weekMap[r.Week]; !exists {
					weeks = append(weeks, r.Week)
				}
				weekMap[r.Week] = append(weekMap[r.Week], r)
			}

			if jsonFormat {
				type trendBucket struct {
					Week     string                         `json:"week"`
					Patterns []storage.CommandPatternWeekly `json:"patterns"`
				}
				var trends []trendBucket
				for _, w := range weeks {
					trends = append(trends, trendBucket{Week: w, Patterns: weekMap[w]})
				}
				data := map[string]interface{}{
					"count":  len(results),
					"kind":   "commands",
					"trends": trends,
				}
				if err := json.NewEncoder(stdout).Encode(data); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else if robotFormat {
				_, _ = fmt.Fprintf(stdout, "*** Trends ***\n")
				for weekIdx, w := range weeks {
					_, _ = fmt.Fprintf(stdout, "week_%d=%s\n", weekIdx, w)
					for i, p := range weekMap[w] {
						_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
						_, _ = fmt.Fprintf(stdout, "result_%d_command_head=%s\n", i, p.CommandHead)
						_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
					}
				}
				_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
			} else {
				_, _ = fmt.Fprintf(stdout, "Trends in Commands (Week-over-Week)\n")
				_, _ = fmt.Fprintf(stdout, "==================================\n\n")
				for _, w := range weeks {
					_, _ = fmt.Fprintf(stdout, "%s\n", w)
					for i, p := range weekMap[w] {
						_, _ = fmt.Fprintf(stdout, "  %d. %s %s (%d times)\n", i+1, p.ToolName, p.CommandHead, p.Count)
					}
					_, _ = fmt.Fprintf(stdout, "\n")
				}
			}
		} else {
			results, err := db.AggregateCommands(opts)
			if err != nil {
				return fmt.Errorf("aggregate commands: %w", err)
			}

			if len(results) == 0 {
				if !jsonFormat && !robotFormat {
					_, _ = fmt.Fprintf(stdout, "No patterns found.\n")
				} else if jsonFormat {
					_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"patterns\":[]}\n")
				}
				writeSearchHints(stderr, allProjects, true)
				return nil
			}

			if jsonFormat {
				data := map[string]interface{}{
					"count":    len(results),
					"kind":     "commands",
					"patterns": results,
				}
				if err := json.NewEncoder(stdout).Encode(data); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else if robotFormat {
				_, _ = fmt.Fprintf(stdout, "*** Commands ***\n")
				for i, p := range results {
					_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
					_, _ = fmt.Fprintf(stdout, "result_%d_command_head=%s\n", i, p.CommandHead)
					_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
				}
				_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
			} else {
				_, _ = fmt.Fprintf(stdout, "Top Commands\n")
				_, _ = fmt.Fprintf(stdout, "============\n\n")
				for i, p := range results {
					_, _ = fmt.Fprintf(stdout, "%d. %s %s (%d times)\n", i+1, p.ToolName, p.CommandHead, p.Count)
				}
			}
		}

	} else if kind == "failures" {
		if trend {
			var excluded int
			results, err := db.AggregateFailuresTrend(opts, &excluded)
			if err != nil {
				return fmt.Errorf("aggregate failures trend: %w", err)
			}

			if excluded > 0 {
				_, _ = fmt.Fprintf(stderr, "trend: %d events excluded (no timestamp)\n", excluded)
			}

			if len(results) == 0 {
				if !jsonFormat && !robotFormat {
					_, _ = fmt.Fprintf(stdout, "No patterns found. Try --min-support <lower> or --min-length <lower>.\n")
				} else if jsonFormat {
					_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"kind\":\"failures\",\"trends\":[]}\n")
				}
				writeSearchHints(stderr, allProjects, true)
				return nil
			}

			// Group results by week for text/json output
			weekMap := make(map[string][]storage.FailurePatternWeekly)
			var weeks []string
			for _, r := range results {
				if _, exists := weekMap[r.Week]; !exists {
					weeks = append(weeks, r.Week)
				}
				weekMap[r.Week] = append(weekMap[r.Week], r)
			}

			if jsonFormat {
				type trendBucket struct {
					Week     string                         `json:"week"`
					Patterns []storage.FailurePatternWeekly `json:"patterns"`
				}
				var trends []trendBucket
				for _, w := range weeks {
					trends = append(trends, trendBucket{Week: w, Patterns: weekMap[w]})
				}
				data := map[string]interface{}{
					"count":  len(results),
					"kind":   "failures",
					"trends": trends,
				}
				if err := json.NewEncoder(stdout).Encode(data); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else if robotFormat {
				_, _ = fmt.Fprintf(stdout, "*** Trends ***\n")
				for weekIdx, w := range weeks {
					_, _ = fmt.Fprintf(stdout, "week_%d=%s\n", weekIdx, w)
					for i, p := range weekMap[w] {
						exitCodeStr := "?"
						if p.ExitCode != nil {
							exitCodeStr = fmt.Sprintf("%d", *p.ExitCode)
						}
						_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
						_, _ = fmt.Fprintf(stdout, "result_%d_is_error=%v\n", i, p.IsError)
						_, _ = fmt.Fprintf(stdout, "result_%d_exit_code=%s\n", i, exitCodeStr)
						_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
						_, _ = fmt.Fprintf(stdout, "result_%d_coverage=%d/%d\n", i, p.Count, p.SignalledEvents)
					}
				}
				_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
			} else {
				_, _ = fmt.Fprintf(stdout, "Trends in Failures (Week-over-Week)\n")
				_, _ = fmt.Fprintf(stdout, "===================================\n\n")
				for _, w := range weeks {
					_, _ = fmt.Fprintf(stdout, "%s\n", w)
					for i, p := range weekMap[w] {
						exitCodeStr := "?"
						if p.ExitCode != nil {
							exitCodeStr = fmt.Sprintf("%d", *p.ExitCode)
						}
						_, _ = fmt.Fprintf(stdout, "  %d. %s (is_error=%v, exit_code=%s) — %d occurrences\n",
							i+1, p.ToolName, p.IsError, exitCodeStr, p.Count)
					}
					_, _ = fmt.Fprintf(stdout, "\n")
				}
			}
		} else {
			results, err := db.AggregateFailures(opts)
			if err != nil {
				return fmt.Errorf("aggregate failures: %w", err)
			}

			if len(results) == 0 {
				if !jsonFormat && !robotFormat {
					_, _ = fmt.Fprintf(stdout, "No failure patterns found.\n")
				} else if jsonFormat {
					_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"patterns\":[]}\n")
				}
				writeSearchHints(stderr, allProjects, true)
				return nil
			}

			if jsonFormat {
				data := map[string]interface{}{
					"count":    len(results),
					"kind":     "failures",
					"patterns": results,
				}
				if err := json.NewEncoder(stdout).Encode(data); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else if robotFormat {
				_, _ = fmt.Fprintf(stdout, "*** Failures ***\n")
				for i, p := range results {
					exitCodeStr := "?"
					if p.ExitCode != nil {
						exitCodeStr = fmt.Sprintf("%d", *p.ExitCode)
					}
					_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
					_, _ = fmt.Fprintf(stdout, "result_%d_is_error=%v\n", i, p.IsError)
					_, _ = fmt.Fprintf(stdout, "result_%d_exit_code=%s\n", i, exitCodeStr)
					_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
					_, _ = fmt.Fprintf(stdout, "result_%d_coverage=%d/%d\n", i, p.Count, p.SignalledEvents)
				}
				_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
			} else {
				_, _ = fmt.Fprintf(stdout, "Failure Patterns\n")
				_, _ = fmt.Fprintf(stdout, "================\n\n")
				if len(results) > 0 {
					_, _ = fmt.Fprintf(stdout, "Signalled events (with error signal): %d\n\n", results[0].SignalledEvents)
				}
				for i, p := range results {
					exitCodeStr := "?"
					if p.ExitCode != nil {
						exitCodeStr = fmt.Sprintf("%d", *p.ExitCode)
					}
					_, _ = fmt.Fprintf(stdout, "%d. %s (is_error=%v, exit_code=%s) — %d occurrences\n",
						i+1, p.ToolName, p.IsError, exitCodeStr, p.Count)
				}
			}
		}

	} else if kind == "templates" {
		templateOpts := storage.TemplateQueryOpts{
			MinSupport: minSupport,
			Project:    project,
			Tag:        tag,
		}
		results, err := db.AggregateTemplates(templateOpts)
		if err != nil {
			return fmt.Errorf("aggregate templates: %w", err)
		}

		if len(results) == 0 {
			if !jsonFormat && !robotFormat {
				_, _ = fmt.Fprintf(stdout, "No templates found.\n")
			} else if jsonFormat {
				_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"patterns\":[]}\n")
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		// Apply pagination
		if offset > len(results) {
			offset = len(results)
		}
		if limit <= 0 {
			limit = 20
		}
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]

		if jsonFormat {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "templates",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Templates ***\n")
			for i, r := range results {
				_, _ = fmt.Fprintf(stdout, "result_%d_template_id=%d\n", i, r.TemplateID)
				_, _ = fmt.Fprintf(stdout, "result_%d_template=%q\n", i, r.TemplateText)
				_, _ = fmt.Fprintf(stdout, "result_%d_occurrence_count=%d\n", i, r.OccurrenceCount)
			}
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
		} else {
			_, _ = fmt.Fprintf(stdout, "Found %d templates (min_support=%d):\n\n", len(results), minSupport)
			for i, row := range results {
				_, _ = fmt.Fprintf(stdout, "%d. [%s]\n", i+1, row.Signature[:8])
				_, _ = fmt.Fprintf(stdout, "   Text: %s\n", row.TemplateText)
				_, _ = fmt.Fprintf(stdout, "   Occurrences: %d\n", row.OccurrenceCount)
				_, _ = fmt.Fprintf(stdout, "   Projects: %v\n", row.ProjectsAffected)
				_, _ = fmt.Fprintf(stdout, "   Sample UUIDs: %v\n\n", row.SampleUUIDs)
			}
		}

	} else if kind == "corrections" {
		// --batch is alias for --limit
		if batch > 0 && limit == 20 {
			limit = batch
		}

		correctionOpts := storage.CorrectionAggOpts{
			Project:       project,
			MinConfidence: minConfidence,
			Limit:         limit,
			Offset:        offset,
			PendingOnly:   pending,
		}
		results, err := db.AggregateCorrections(correctionOpts)
		if err != nil {
			return fmt.Errorf("aggregate corrections: %w", err)
		}

		if len(results) == 0 {
			if !jsonFormat && !robotFormat {
				_, _ = fmt.Fprintf(stdout, "No correction candidates found.\n")
			} else if jsonFormat {
				_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"kind\":\"corrections\",\"patterns\":[]}\n")
			}
			if pending {
				_, _ = fmt.Fprintf(stderr, "hint: no pending corrections. All candidates have been annotated, or use --pending=false to view all.\n")
			} else {
				writeSearchHints(stderr, allProjects, true)
			}
			return nil
		}

		if jsonFormat {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "corrections",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Corrections ***\n")
			for i, c := range results {
				_, _ = fmt.Fprintf(stdout, "result_%d_uuid=%s\n", i, c.UUID)
				_, _ = fmt.Fprintf(stdout, "result_%d_source_path=%s\n", i, c.SourcePath)
				_, _ = fmt.Fprintf(stdout, "result_%d_ordinal=%d\n", i, c.Ordinal)
				_, _ = fmt.Fprintf(stdout, "result_%d_detectors=%s\n", i, strings.Join(c.Detectors, ","))
				_, _ = fmt.Fprintf(stdout, "result_%d_max_confidence=%.2f\n", i, c.MaxConfidence)
				_, _ = fmt.Fprintf(stdout, "result_%d_text_snippet=%q\n", i, c.TextSnippet)
			}
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
		} else {
			_, _ = fmt.Fprintf(stdout, "Correction Candidates\n")
			_, _ = fmt.Fprintf(stdout, "====================\n\n")
			for i, c := range results {
				_, _ = fmt.Fprintf(stdout, "%d. UUID: %s\n", i+1, c.UUID)
				_, _ = fmt.Fprintf(stdout, "   Source: %s (ordinal %d)\n", c.SourcePath, c.Ordinal)
				_, _ = fmt.Fprintf(stdout, "   Detectors: %s (max confidence: %.2f)\n", strings.Join(c.Detectors, ", "), c.MaxConfidence)
				_, _ = fmt.Fprintf(stdout, "   Text: %s\n\n", c.TextSnippet)
			}
		}

	} else if kind == "sequences" {
		// Defaults for sequences
		if minSupport == 0 {
			minSupport = 3
		}
		if minLength == 0 {
			minLength = 2
		}
		if maxLength == 0 {
			maxLength = 6
		}

		// Limit/Offset paginate mined patterns below, never the input corpus.
		seqs, err := db.LoadToolSequences(storage.LoadSequencesOpts{
			Project: project,
			After:   after,
			Before:  before,
		})
		if err != nil {
			// A load failure (e.g. malformed categories.toml) must fail the
			// command — exit code zero would be indistinguishable from a
			// legitimate empty result for scripts and agents.
			return fmt.Errorf("load sequences: %w", err)
		}

		patterns := sequences.Mine(seqs, minSupport, minLength, maxLength)
		// Paginate the mined patterns (the input corpus is never truncated).
		if offset > 0 {
			if offset >= len(patterns) {
				patterns = nil
			} else {
				patterns = patterns[offset:]
			}
		}
		if limit > 0 && len(patterns) > limit {
			patterns = patterns[:limit]
		}
		if len(patterns) == 0 {
			if !jsonFormat && !robotFormat {
				_, _ = fmt.Fprintf(stdout, "No patterns found. Try --min-support <lower> or --min-length <lower>.\n")
			} else if jsonFormat {
				_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"kind\":\"sequences\",\"patterns\":[]}\n")
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		// Format results
		totalSessions := len(seqs)
		if jsonFormat {
			type sequencePattern struct {
				Pattern string `json:"pattern"`
				Support int    `json:"support"`
				Share   string `json:"share"`
			}
			var results []sequencePattern
			for _, p := range patterns {
				share := float64(p.Support) * 100.0 / float64(totalSessions)
				results = append(results, sequencePattern{
					Pattern: strings.Join(p.Items, " → "),
					Support: p.Support,
					Share:   fmt.Sprintf("%.1f%%", share),
				})
			}
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "sequences",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Sequences ***\n")
			for i, p := range patterns {
				share := float64(p.Support) * 100.0 / float64(totalSessions)
				_, _ = fmt.Fprintf(stdout, "result_%d_pattern=%s\n", i, strings.Join(p.Items, " → "))
				_, _ = fmt.Fprintf(stdout, "result_%d_support=%d\n", i, p.Support)
				_, _ = fmt.Fprintf(stdout, "result_%d_share=%.1f%%\n", i, share)
			}
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(patterns))
		} else {
			_, _ = fmt.Fprintf(stdout, "Frequent Tool Sequences (exploratory)\n")
			_, _ = fmt.Fprintf(stdout, "======================================\n\n")
			_, _ = fmt.Fprintf(stdout, "Total sessions analyzed: %d\n\n", totalSessions)
			for i, p := range patterns {
				share := float64(p.Support) * 100.0 / float64(totalSessions)
				_, _ = fmt.Fprintf(stdout, "%d. %s\n", i+1, strings.Join(p.Items, " → "))
				_, _ = fmt.Fprintf(stdout, "   Support: %d sessions (%.1f%%)\n\n", p.Support, share)
			}
		}
	}

	return nil
}
