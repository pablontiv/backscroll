package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
)

func newAnnotateCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		uuid    string
		path    string
		ordinal int
		kind    string
		label   string
	)

	cmd := &cobra.Command{
		Use:          "annotate",
		Short:        "Annotate a message (agent-classification loop write surface)",
		SilenceUsage: true,
		Long: `Annotate a message with a classification label. Validates message existence
before writing. Supports both uuid (preferred) and legacy source_path+ordinal
fallback. Re-annotating the same (source_path, ordinal, kind) replaces the label.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected positional argument %q", args[0])
			}
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runAnnotate(cmd.Context(), stdout, stderr, startup.Config, uuid, path, ordinal, kind, label)
		},
	}

	cmd.Flags().StringVar(&uuid, "uuid", "", "Message uuid (preferred identity)")
	cmd.Flags().StringVar(&path, "path", "", "Message source_path (legacy fallback)")
	cmd.Flags().IntVar(&ordinal, "ordinal", -1, "Message ordinal (legacy fallback; with --path)")
	cmd.Flags().StringVar(&kind, "kind", "", "Annotation kind (required; e.g., 'correction')")
	cmd.Flags().StringVar(&label, "label", "", "Annotation label (free-form; required)")

	cmd.MarkFlagRequired("kind")
	cmd.MarkFlagRequired("label")

	return cmd
}

func runAnnotate(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config,
	uuid, path string, ordinal int, kind, label string) (retErr error) {

	// Early flag validation
	if uuid == "" && (path == "" || ordinal < 0) {
		return fmt.Errorf("must provide either --uuid or both --path and --ordinal")
	}

	if kind == "" || label == "" {
		return fmt.Errorf("--kind and --label are required")
	}

	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, false, false)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	// Upsert annotation
	if err := db.UpsertAnnotation(uuid, path, ordinal, kind, label); err != nil {
		return fmt.Errorf("upsert annotation: %w", err)
	}

	if uuid != "" {
		_, _ = fmt.Fprintf(stdout, "Annotated %s as %s=%s\n", uuid, kind, label)
	} else {
		_, _ = fmt.Fprintf(stdout, "Annotated %s:%d as %s=%s\n", path, ordinal, kind, label)
	}

	return nil
}
