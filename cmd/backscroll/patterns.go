package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
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
		minSupport    int
		minConfidence float64
		pending       bool
		batch         int
		minLength     int
		maxLength     int
		after         string
		before        string
		trend         bool
		origin        string
	)

	cmd := &cobra.Command{
		Use:          "patterns",
		Short:        "Discover deterministic patterns in tool events, templates, sequences, and corrections",
		SilenceUsage: true,
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
Use --origin for parser-backed correction origin filtering.
Use --limit, --offset for pagination.
Use --json, --robot for output formats.`,
		Args: func(cmd *cobra.Command, args []string) error {
			return validateCommandBeforeStartup(cmd, args, func(_ *cobra.Command, args []string) error {
				if len(args) > 0 {
					return fmt.Errorf("unexpected positional argument %q", args[0])
				}
				return nil
			}, func() error {
				if err := validatePatternsRequest(kind, project, allProjects, limit, offset, trend); err != nil {
					return err
				}
				if err := validatePatternsNumbers(minSupport, minConfidence, batch, minLength, maxLength); err != nil {
					return err
				}
				if err := validatePatternsOrigin(kind, origin); err != nil {
					return err
				}
				resolved, err := effectiveProject(project, allProjects)
				if err == nil {
					project = resolved
				}
				return err
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runPatterns(cmd.Context(), stdout, stderr, startup.Config, kind, project, allProjects, tag, limit, offset,
				invocationOutputMode(cmd), minSupport, minConfidence, pending, batch,
				minLength, maxLength, after, before, trend, origin)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Aggregation kind (required: commands|failures|templates|sequences|corrections)")
	cmd.Flags().StringVar(&project, "project", "", "Filter to single project (mutually exclusive with --all-projects)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query across all projects (default: project inferred from current working directory)")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by session tag")
	cmd.Flags().IntVar(&limit, "limit", 20, "Result limit (0 = unlimited)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().IntVar(&minSupport, "min-support", 3, "Minimum template/sequence occurrence count (>=0; sequences: 0=default 3)")
	cmd.Flags().IntVar(&minLength, "min-length", 2, "Minimum sequence pattern length (for --kind sequences, >=0; 0=default 2)")
	cmd.Flags().IntVar(&maxLength, "max-length", 6, "Maximum sequence pattern length (for --kind sequences, >=0; 0=default 6; prevents combinatorial explosion)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0.6, "Minimum detector confidence (finite 0..1; for --kind corrections)")
	cmd.Flags().StringVar(&after, "after", "", "Filter on or after date (inclusive ISO 8601, for --kind sequences)")
	cmd.Flags().StringVar(&before, "before", "", "Filter before date (exclusive ISO 8601, for --kind sequences)")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().BoolVar(&pending, "pending", false, "Only corrections without a 'correction' annotation (checkpoint resume)")
	cmd.Flags().IntVar(&batch, "batch", 0, "Alias for --limit (batch size for loop)")
	cmd.Flags().BoolVar(&trend, "trend", false, "Week-over-week bucketing (--kind commands|failures only)")
	cmd.Flags().StringVar(&origin, "origin", "", "Filter parser-backed origin (--kind corrections only: human|assistant|system|automation|unknown)")

	cmd.MarkFlagRequired("kind")

	cmd.MarkFlagsMutuallyExclusive("project", "all-projects")
	describeOutputModes(cmd)
	return cmd
}

func validatePatternsRequest(kind, project string, allProjects bool, limit, offset int, trend bool) error {
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
	return nil
}

func validatePatternsOrigin(kind, origin string) error {
	if origin == "" {
		return nil
	}
	if !models.ValidMessageOrigin(models.MessageOrigin(origin)) {
		return fmt.Errorf("unsupported --origin %q (supported: human, assistant, system, automation, unknown)", origin)
	}
	if kind != "corrections" {
		return fmt.Errorf("--origin only supported for --kind corrections, got %q", kind)
	}
	return nil
}

func runPatterns(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config,
	kind string, project string, allProjects bool, tag string,
	limit, offset int, mode outputMode, minSupport int, minConfidence float64, pending bool, batch int,
	minLength, maxLength int, after, before string, trend bool, origin string) (retErr error) {

	if err := validatePatternsRequest(kind, project, allProjects, limit, offset, trend); err != nil {
		return err
	}
	if err := validatePatternsOrigin(kind, origin); err != nil {
		return err
	}

	project, err := effectiveProject(project, allProjects)
	if err != nil {
		return err
	}

	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, mode)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

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
				if mode == outputText {
					if err := writeOutput(stdout, "No patterns found. Try --min-support <lower> or --min-length <lower>.\n"); err != nil {
						return err
					}
				} else if mode == outputJSON {
					if err := writeOutput(stdout, "{\"count\":0,\"kind\":\"commands\",\"trends\":[]}\n"); err != nil {
						return err
					}
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

			if mode == outputJSON {
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
			} else if mode == outputRobot {
				if err := writeOutput(stdout, "*** Trends ***\n"); err != nil {
					return err
				}
				for weekIdx, w := range weeks {
					if err := writeOutput(stdout, "week_%d=%s\n", weekIdx, w); err != nil {
						return err
					}
					for i, p := range weekMap[w] {
						if err := writeOutput(stdout, "week_%d_result_%d_tool_name=%s\n", weekIdx, i, p.ToolName); err != nil {
							return err
						}
						if err := writeOutput(stdout, "week_%d_result_%d_command_head=%s\n", weekIdx, i, p.CommandHead); err != nil {
							return err
						}
						if err := writeOutput(stdout, "week_%d_result_%d_count=%d\n", weekIdx, i, p.Count); err != nil {
							return err
						}
					}
				}
				if err := writeOutput(stdout, "*** Total: %d patterns ***\n", len(results)); err != nil {
					return err
				}
			} else {
				if err := writeOutput(stdout, "Trends in Commands (Week-over-Week)\n"); err != nil {
					return err
				}
				if err := writeOutput(stdout, "==================================\n\n"); err != nil {
					return err
				}
				for _, w := range weeks {
					if err := writeOutput(stdout, "%s\n", w); err != nil {
						return err
					}
					for i, p := range weekMap[w] {
						if err := writeOutput(stdout, "  %d. %s %s (%d times)\n", i+1, p.ToolName, p.CommandHead, p.Count); err != nil {
							return err
						}
					}
					if err := writeOutput(stdout, "\n"); err != nil {
						return err
					}
				}
			}
		} else {
			results, err := db.AggregateCommands(opts)
			if err != nil {
				return fmt.Errorf("aggregate commands: %w", err)
			}

			if len(results) == 0 {
				if mode == outputText {
					if err := writeOutput(stdout, "No patterns found.\n"); err != nil {
						return err
					}
				} else if mode == outputJSON {
					if err := writeOutput(stdout, "{\"count\":0,\"patterns\":[]}\n"); err != nil {
						return err
					}
				}
				writeSearchHints(stderr, allProjects, true)
				return nil
			}

			if mode == outputJSON {
				data := map[string]interface{}{
					"count":    len(results),
					"kind":     "commands",
					"patterns": results,
				}
				if err := json.NewEncoder(stdout).Encode(data); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else if mode == outputRobot {
				if err := writeOutput(stdout, "*** Commands ***\n"); err != nil {
					return err
				}
				for i, p := range results {
					if err := writeOutput(stdout, "result_%d_tool_name=%s\n", i, p.ToolName); err != nil {
						return err
					}
					if err := writeOutput(stdout, "result_%d_command_head=%s\n", i, p.CommandHead); err != nil {
						return err
					}
					if err := writeOutput(stdout, "result_%d_count=%d\n", i, p.Count); err != nil {
						return err
					}
				}
				if err := writeOutput(stdout, "*** Total: %d patterns ***\n", len(results)); err != nil {
					return err
				}
			} else {
				if err := writeOutput(stdout, "Top Commands\n"); err != nil {
					return err
				}
				if err := writeOutput(stdout, "============\n\n"); err != nil {
					return err
				}
				for i, p := range results {
					if err := writeOutput(stdout, "%d. %s %s (%d times)\n", i+1, p.ToolName, p.CommandHead, p.Count); err != nil {
						return err
					}
				}
			}
		}

	} else if kind == "failures" {
		return renderFailurePatterns(stdout, stderr, db, opts, trend, mode == outputJSON, mode == outputRobot, allProjects)

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
			if mode == outputText {
				if err := writeOutput(stdout, "No templates found.\n"); err != nil {
					return err
				}
			} else if mode == outputJSON {
				if err := writeOutput(stdout, "{\"count\":0,\"patterns\":[]}\n"); err != nil {
					return err
				}
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		// Apply pagination
		if offset > len(results) {
			offset = len(results)
		}
		end := len(results)
		if limit > 0 && limit < len(results)-offset {
			end = offset + limit
		}
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]

		if mode == outputJSON {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "templates",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if mode == outputRobot {
			if err := writeOutput(stdout, "*** Templates ***\n"); err != nil {
				return err
			}
			for i, r := range results {
				if err := writeOutput(stdout, "result_%d_template_id=%d\n", i, r.TemplateID); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_template=%q\n", i, r.TemplateText); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_occurrence_count=%d\n", i, r.OccurrenceCount); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_normalization_version=%d\n", i, r.NormalizationVersion); err != nil {
					return err
				}
			}
			if err := writeOutput(stdout, "*** Total: %d patterns ***\n", len(results)); err != nil {
				return err
			}
		} else {
			if err := writeOutput(stdout, "Found %d templates (min_support=%d):\n\n", len(results), minSupport); err != nil {
				return err
			}
			for i, row := range results {
				if err := writeOutput(stdout, "%d. [%s]\n", i+1, row.Signature[:8]); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Text: %s\n", row.TemplateText); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Occurrences: %d\n", row.OccurrenceCount); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Normalization: v%d\n", row.NormalizationVersion); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Projects: %v\n", row.ProjectsAffected); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Sample UUIDs: %v\n\n", row.SampleUUIDs); err != nil {
					return err
				}
			}
		}

	} else if kind == "corrections" {
		// --batch is alias for --limit
		if batch > 0 && limit == 20 {
			limit = batch
		}

		correctionOpts := storage.CorrectionAggOpts{
			Tag:           tag,
			Project:       project,
			Origin:        models.MessageOrigin(origin),
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
			if mode == outputText {
				if err := writeOutput(stdout, "No correction candidates found.\n"); err != nil {
					return err
				}
			} else if mode == outputJSON {
				if err := writeOutput(stdout, "{\"count\":0,\"kind\":\"corrections\",\"patterns\":[]}\n"); err != nil {
					return err
				}
			}
			if pending {
				_, _ = fmt.Fprintf(stderr, "hint: no pending corrections. All candidates have been annotated, or use --pending=false to view all.\n")
			} else {
				writeSearchHints(stderr, allProjects, true)
			}
			return nil
		}

		if mode == outputJSON {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "corrections",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if mode == outputRobot {
			if err := writeOutput(stdout, "*** Corrections ***\n"); err != nil {
				return err
			}
			for i, c := range results {
				if err := writeOutput(stdout, "result_%d_uuid=%s\n", i, c.UUID); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_source_path=%s\n", i, c.SourcePath); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_ordinal=%d\n", i, c.Ordinal); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_detectors=%s\n", i, strings.Join(c.Detectors, ",")); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_max_confidence=%.2f\n", i, c.MaxConfidence); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_text_snippet=%q\n", i, c.TextSnippet); err != nil {
					return err
				}
				if origin != "" {
					if err := writeOutput(stdout, "result_%d_origin=%s\n", i, c.Origin); err != nil {
						return err
					}
				}
			}
			if err := writeOutput(stdout, "*** Total: %d patterns ***\n", len(results)); err != nil {
				return err
			}
		} else {
			if err := writeOutput(stdout, "Correction Candidates\n"); err != nil {
				return err
			}
			if err := writeOutput(stdout, "====================\n\n"); err != nil {
				return err
			}
			for i, c := range results {
				if err := writeOutput(stdout, "%d. UUID: %s\n", i+1, c.UUID); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Source: %s (ordinal %d)\n", c.SourcePath, c.Ordinal); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Detectors: %s (max confidence: %.2f)\n", strings.Join(c.Detectors, ", "), c.MaxConfidence); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Text: %s\n", c.TextSnippet); err != nil {
					return err
				}
				if origin != "" {
					if err := writeOutput(stdout, "   Origin: %s\n", c.Origin); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(stdout); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
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
			Tag:     tag,
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
			if mode == outputText {
				if err := writeOutput(stdout, "No patterns found. Try --min-support <lower> or --min-length <lower>.\n"); err != nil {
					return err
				}
			} else if mode == outputJSON {
				if err := writeOutput(stdout, "{\"count\":0,\"kind\":\"sequences\",\"patterns\":[]}\n"); err != nil {
					return err
				}
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		// Format results
		totalSessions := len(seqs)
		if mode == outputJSON {
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
		} else if mode == outputRobot {
			if err := writeOutput(stdout, "*** Sequences ***\n"); err != nil {
				return err
			}
			for i, p := range patterns {
				share := float64(p.Support) * 100.0 / float64(totalSessions)
				if err := writeOutput(stdout, "result_%d_pattern=%s\n", i, strings.Join(p.Items, " → ")); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_support=%d\n", i, p.Support); err != nil {
					return err
				}
				if err := writeOutput(stdout, "result_%d_share=%.1f%%\n", i, share); err != nil {
					return err
				}
			}
			if err := writeOutput(stdout, "*** Total: %d patterns ***\n", len(patterns)); err != nil {
				return err
			}
		} else {
			if err := writeOutput(stdout, "Frequent Tool Sequences (exploratory)\n"); err != nil {
				return err
			}
			if err := writeOutput(stdout, "======================================\n\n"); err != nil {
				return err
			}
			if err := writeOutput(stdout, "Total sessions analyzed: %d\n\n", totalSessions); err != nil {
				return err
			}
			for i, p := range patterns {
				share := float64(p.Support) * 100.0 / float64(totalSessions)
				if err := writeOutput(stdout, "%d. %s\n", i+1, strings.Join(p.Items, " → ")); err != nil {
					return err
				}
				if err := writeOutput(stdout, "   Support: %d sessions (%.1f%%)\n\n", p.Support, share); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
