package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	picokitoutput "github.com/pablontiv/picokit/output"
	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

const (
	contextDefaultWindow    = 5
	contextMaxWindow        = 50
	contextMaxTextRunes     = 4000
	contextDefaultMaxTokens = 2000
	contextMinMaxTokens     = 64
	contextMaxMaxTokens     = 16384
)

type contextCommandOptions struct {
	uuid          string
	sourcePath    string
	ordinal       int64
	before        int
	after         int
	maxTokens     int
	jsonFormat    bool
	robotFormat   bool
	uuidSet       bool
	sourcePathSet bool
	ordinalSet    bool
}

type contextOutputFormat uint8

const (
	contextTextFormat contextOutputFormat = iota
	contextJSONFormat
	contextRobotFormat
)

type contextAnchorOutput struct {
	UUID       *string `json:"uuid"`
	SourcePath string  `json:"source_path"`
	Ordinal    int64   `json:"ordinal"`
}

type contextRecordOutput struct {
	UUID        *string `json:"uuid"`
	SourcePath  string  `json:"source_path"`
	Ordinal     int64   `json:"ordinal"`
	Role        string  `json:"role"`
	Origin      string  `json:"origin"`
	Timestamp   *string `json:"timestamp"`
	ContentType string  `json:"content_type"`
	Source      string  `json:"source"`
	Text        string  `json:"text"`
	IsAnchor    bool    `json:"is_anchor"`
}

type contextEnvelope struct {
	Anchor    contextAnchorOutput   `json:"anchor"`
	Records   []contextRecordOutput `json:"records"`
	Truncated bool                  `json:"truncated"`
	Omitted   int                   `json:"omitted"`
}

func newContextCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := contextCommandOptions{
		before:    contextDefaultWindow,
		after:     contextDefaultWindow,
		maxTokens: contextDefaultMaxTokens,
	}

	cmd := &cobra.Command{
		Use:          "context",
		Short:        "Show indexed records around an exact anchor",
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			opts.captureSelectorFlags(cmd)
			return validateCommandBeforeStartup(cmd, args, cobra.NoArgs, func() error {
				return validateContextRequest(opts)
			})
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.captureSelectorFlags(cmd)
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runContext(cmd.Context(), stdout, stderr, startup.Config, opts)
		},
	}

	cmd.Flags().StringVar(&opts.uuid, "uuid", "", "Exact record UUID anchor")
	cmd.Flags().StringVar(&opts.sourcePath, "source-path", "", "Exact indexed source path anchor (with --ordinal)")
	cmd.Flags().Int64Var(&opts.ordinal, "ordinal", 0, "Exact record ordinal anchor (with --source-path)")
	cmd.Flags().IntVar(&opts.before, "before", contextDefaultWindow, "Records before the anchor (0-50)")
	cmd.Flags().IntVar(&opts.after, "after", contextDefaultWindow, "Records after the anchor (0-50)")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", contextDefaultMaxTokens, "Maximum Picokit tokens in the complete successful payload; diagnostics are exempt (64-16384)")
	cmd.Flags().BoolVar(&opts.jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&opts.robotFormat, "robot", false, "Output as line-oriented robot data")

	return cmd
}

func (opts *contextCommandOptions) captureSelectorFlags(cmd *cobra.Command) {
	opts.uuidSet = cmd.Flags().Changed("uuid")
	opts.sourcePathSet = cmd.Flags().Changed("source-path")
	opts.ordinalSet = cmd.Flags().Changed("ordinal")
}

func validateContextRequest(opts contextCommandOptions) error {
	uuidSelector := opts.uuidSet && !opts.sourcePathSet && !opts.ordinalSet
	pathOrdinalSelector := !opts.uuidSet && opts.sourcePathSet && opts.ordinalSet
	if !uuidSelector && !pathOrdinalSelector {
		return fmt.Errorf("provide exactly one anchor selector: --uuid or --source-path with --ordinal")
	}
	if uuidSelector && opts.uuid == "" {
		return fmt.Errorf("--uuid must not be empty")
	}
	if pathOrdinalSelector && opts.sourcePath == "" {
		return fmt.Errorf("--source-path must not be empty")
	}
	if opts.before < 0 || opts.before > contextMaxWindow {
		return fmt.Errorf("--before must be between 0 and %d", contextMaxWindow)
	}
	if opts.after < 0 || opts.after > contextMaxWindow {
		return fmt.Errorf("--after must be between 0 and %d", contextMaxWindow)
	}
	if opts.maxTokens < contextMinMaxTokens || opts.maxTokens > contextMaxMaxTokens {
		return fmt.Errorf("--max-tokens must be between %d and %d", contextMinMaxTokens, contextMaxMaxTokens)
	}
	if opts.jsonFormat && opts.robotFormat {
		return fmt.Errorf("--json and --robot are mutually exclusive")
	}
	return nil
}

func runContext(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, opts contextCommandOptions) (retErr error) {
	if err := validateContextRequest(opts); err != nil {
		return err
	}

	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, opts.jsonFormat, opts.robotFormat)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	query := storage.ContextRecordQuery{Before: opts.before, After: opts.after}
	if opts.uuidSet {
		query.UUID = &opts.uuid
	} else {
		query.SourcePath = &opts.sourcePath
		query.Ordinal = &opts.ordinal
	}
	records, err := db.QueryContextRecords(ctx, query)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrContextRecordNotFound):
			return writeContextDiagnostic(stdout, stderr, "context_not_found", err.Error(), opts)
		case errors.Is(err, storage.ErrContextRecordAmbiguous):
			return writeContextDiagnostic(stdout, stderr, "context_ambiguous", err.Error(), opts)
		default:
			return fmt.Errorf("query context records: %w", err)
		}
	}

	format := contextTextFormat
	if opts.jsonFormat {
		format = contextJSONFormat
	} else if opts.robotFormat {
		format = contextRobotFormat
	}
	payload, successfulPayloadFits, err := contextSuccessfulPayloadWithinBudget(records, format, opts.maxTokens)
	if err != nil {
		return fmt.Errorf("format context records: %w", err)
	}
	if !successfulPayloadFits {
		return writeContextDiagnostic(stdout, stderr, "context_budget_too_small",
			fmt.Sprintf("--max-tokens %d cannot fit the anchor and required metadata", opts.maxTokens), opts)
	}
	if _, err := stdout.Write(payload); err != nil {
		return fmt.Errorf("write context records: %w", err)
	}
	return nil
}

func writeContextDiagnostic(stdout, stderr io.Writer, code, summary string, opts contextCommandOptions) error {
	return refuseIndex(stdout, stderr, compat.Diagnostic{Code: compat.Code(code), Summary: summary}, opts.jsonFormat, opts.robotFormat)
}

func contextSuccessfulPayloadWithinBudget(records []storage.ContextRecord, format contextOutputFormat, maxTokens int) ([]byte, bool, error) {
	normalized := make([]contextRecordOutput, len(records))
	anchorIndex := -1
	for i, record := range records {
		normalized[i] = contextOutputRecord(record)
		if record.Anchor {
			if anchorIndex >= 0 {
				return nil, false, fmt.Errorf("multiple anchor records returned")
			}
			anchorIndex = i
		}
	}
	if anchorIndex < 0 {
		return nil, false, fmt.Errorf("anchor record missing from context window")
	}

	anchor := contextAnchorOutput{
		UUID:       normalized[anchorIndex].UUID,
		SourcePath: normalized[anchorIndex].SourcePath,
		Ordinal:    normalized[anchorIndex].Ordinal,
	}
	first, last := 0, len(normalized)
	for {
		included := normalized[first:last]
		omitted := len(normalized) - len(included)
		payload, err := renderContextPayload(contextEnvelope{
			Anchor:    anchor,
			Records:   included,
			Truncated: omitted > 0,
			Omitted:   omitted,
		}, format)
		if err != nil {
			return nil, false, err
		}
		// Count the complete, escaped successful payload as one unit. Per-line
		// or per-record estimates round independently and can undercount.
		// Structured diagnostics are emitted separately and are intentionally
		// exempt from this successful-payload budget.
		if picokitoutput.TokenCount(string(payload)) <= maxTokens {
			return payload, true, nil
		}
		if len(included) == 1 {
			return nil, false, nil
		}

		anchorPosition := anchorIndex - first
		leftDistance := anchorPosition
		rightDistance := len(included) - 1 - anchorPosition
		if rightDistance >= leftDistance && rightDistance > 0 {
			last--
		} else {
			first++
		}
	}
}

func contextOutputRecord(record storage.ContextRecord) contextRecordOutput {
	return contextRecordOutput{
		UUID:        record.UUID,
		SourcePath:  record.SourcePath,
		Ordinal:     record.Ordinal,
		Role:        record.Role,
		Origin:      string(record.Origin),
		Timestamp:   record.Timestamp,
		ContentType: record.ContentType,
		Source:      record.Source,
		Text:        truncateContextText(record.Text),
		IsAnchor:    record.Anchor,
	}
}

func truncateContextText(text string) string {
	runes := []rune(text)
	if len(runes) <= contextMaxTextRunes {
		return text
	}
	return string(runes[:contextMaxTextRunes])
}

func renderContextPayload(envelope contextEnvelope, format contextOutputFormat) ([]byte, error) {
	switch format {
	case contextJSONFormat:
		payload, err := json.Marshal(envelope)
		if err != nil {
			return nil, err
		}
		return append(payload, '\n'), nil
	case contextRobotFormat:
		return []byte(renderContextRobot(envelope)), nil
	case contextTextFormat:
		return []byte(renderContextText(envelope)), nil
	default:
		return nil, fmt.Errorf("unknown context output format %d", format)
	}
}

func renderContextText(envelope contextEnvelope) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Context anchor: uuid=%s source_path=%s ordinal=%d\n",
		contextNullableText(envelope.Anchor.UUID), strconv.Quote(envelope.Anchor.SourcePath), envelope.Anchor.Ordinal)
	fmt.Fprintf(&out, "Records: %d | truncated=%t | omitted=%d\n", len(envelope.Records), envelope.Truncated, envelope.Omitted)
	for i, record := range envelope.Records {
		fmt.Fprintf(&out, "\nRecord %d\n", i)
		fmt.Fprintf(&out, "  uuid: %s\n", contextNullableText(record.UUID))
		fmt.Fprintf(&out, "  source_path: %s\n", strconv.Quote(record.SourcePath))
		fmt.Fprintf(&out, "  ordinal: %d\n", record.Ordinal)
		fmt.Fprintf(&out, "  role: %s\n", strconv.Quote(record.Role))
		fmt.Fprintf(&out, "  origin: %s\n", strconv.Quote(record.Origin))
		fmt.Fprintf(&out, "  timestamp: %s\n", contextNullableText(record.Timestamp))
		fmt.Fprintf(&out, "  content_type: %s\n", strconv.Quote(record.ContentType))
		fmt.Fprintf(&out, "  source: %s\n", strconv.Quote(record.Source))
		fmt.Fprintf(&out, "  text: %s\n", strconv.Quote(record.Text))
		fmt.Fprintf(&out, "  is_anchor: %t\n", record.IsAnchor)
	}
	return out.String()
}

func renderContextRobot(envelope contextEnvelope) string {
	var out strings.Builder
	fmt.Fprintf(&out, "anchor_uuid=%s\n", contextNullableRobot(envelope.Anchor.UUID))
	fmt.Fprintf(&out, "anchor_source_path=%s\n", contextRobotEscape(envelope.Anchor.SourcePath))
	fmt.Fprintf(&out, "anchor_ordinal=%d\n", envelope.Anchor.Ordinal)
	fmt.Fprintf(&out, "records=%d\n", len(envelope.Records))
	fmt.Fprintf(&out, "truncated=%t\n", envelope.Truncated)
	fmt.Fprintf(&out, "omitted=%d\n", envelope.Omitted)
	for i, record := range envelope.Records {
		fmt.Fprintf(&out, "record_%d_uuid=%s\n", i, contextNullableRobot(record.UUID))
		fmt.Fprintf(&out, "record_%d_source_path=%s\n", i, contextRobotEscape(record.SourcePath))
		fmt.Fprintf(&out, "record_%d_ordinal=%d\n", i, record.Ordinal)
		fmt.Fprintf(&out, "record_%d_role=%s\n", i, contextRobotEscape(record.Role))
		fmt.Fprintf(&out, "record_%d_origin=%s\n", i, contextRobotEscape(record.Origin))
		fmt.Fprintf(&out, "record_%d_timestamp=%s\n", i, contextNullableRobot(record.Timestamp))
		fmt.Fprintf(&out, "record_%d_content_type=%s\n", i, contextRobotEscape(record.ContentType))
		fmt.Fprintf(&out, "record_%d_source=%s\n", i, contextRobotEscape(record.Source))
		fmt.Fprintf(&out, "record_%d_text=%s\n", i, contextRobotEscape(record.Text))
		fmt.Fprintf(&out, "record_%d_is_anchor=%t\n", i, record.IsAnchor)
	}
	return out.String()
}

func contextNullableText(value *string) string {
	if value == nil {
		return "null"
	}
	return strconv.Quote(*value)
}

func contextNullableRobot(value *string) string {
	if value == nil {
		return "null"
	}
	return contextRobotEscape(*value)
}

func contextRobotEscape(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}
