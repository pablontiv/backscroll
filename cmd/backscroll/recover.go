package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/spf13/cobra"
)

var recoverExecute = recovery.Execute
var recoverPostInstallSync = maybeAutoSync

func newRecoverCmd(stdout, stderr io.Writer) *cobra.Command {
	var from string
	var dryRun bool
	fromValue := singleUseStringValue{target: &from}

	cmd := &cobra.Command{
		Use:          "recover",
		Short:        "Recover stranded database rows into the configured database",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			startupFailure := startup.startupFailure()
			cfg := startup.Config
			if cfg == nil {
				loaded, err := config.Load()
				if err != nil {
					return errors.Join(startupFailure, fmt.Errorf("load config for recovery: %w", err))
				}
				cfg = loaded
			}
			report, err := recoverExecute(cmd.Context(), recovery.Options{
				ActivePath: cfg.DatabasePath,
				FromPath:   from,
				DryRun:     dryRun,
			})
			if err != nil {
				if backupPath, ok := recovery.RestorableBackupPath(err); ok {
					_, _ = fmt.Fprintf(stderr, "manual recovery backup path: %s\n", backupPath)
				}
				return errors.Join(startupFailure, fmt.Errorf("recovery failed: %w", err))
			}
			if !dryRun {
				if err := recoverPostInstallSync(cfg, stderr); err != nil {
					return errors.Join(startupFailure, fmt.Errorf("post-recovery sync: %w", err))
				}
			}
			printRecoveryReport(stdout, report, dryRun)
			return nil
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().Var(&fromValue, "from", "path to one stranded Backscroll database")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan and report recovery without writing files")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

type singleUseStringValue struct {
	target *string
	set    bool
}

func (v *singleUseStringValue) Set(value string) error {
	if v.set {
		return fmt.Errorf("flag --from may be set only once")
	}
	*v.target = value
	v.set = true
	return nil
}

func (v *singleUseStringValue) String() string {
	if v.target == nil {
		return ""
	}
	return *v.target
}

func (v *singleUseStringValue) Type() string { return "string" }

func printRecoveryReport(w io.Writer, report recovery.Report, dryRun bool) {
	if dryRun {
		_, _ = fmt.Fprintln(w, "recovery dry run")
	} else {
		_, _ = fmt.Fprintln(w, "recovery complete")
	}
	_, _ = fmt.Fprintf(w, "active path: %s\n", report.ActivePath)
	_, _ = fmt.Fprintf(w, "replacement target: %s\n", report.ActivePath)
	_, _ = fmt.Fprintf(w, "backup path: %s\n", report.BackupPath)
	for i, count := range report.InputCounts {
		_, _ = fmt.Fprintf(w, "input %d records: %d\n", i+1, count)
	}
	_, _ = fmt.Fprintf(w, "exact duplicates: %d\n", report.ExactDuplicates)
	_, _ = fmt.Fprintf(w, "final count: %d\n", report.FinalCount)
	for i, shape := range report.Shapes {
		_, _ = fmt.Fprintf(w, "input %d shape: version=%d signature=%s\n", i+1, shape.AppliedVersion, shape.Signature)
	}
	for _, conflict := range report.Conflicts {
		_, _ = fmt.Fprintf(w, "diagnostic: %s: %s\n", conflict.Code, strings.TrimSpace(conflict.Summary))
	}
}
