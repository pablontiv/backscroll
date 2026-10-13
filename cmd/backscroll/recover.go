package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/spf13/cobra"
)

func newRecoverCmd(stdout, stderr io.Writer, syncService *startupSyncService, execute func(context.Context, recovery.Options) (recovery.Report, error)) *cobra.Command {
	if syncService == nil {
		syncService = newStartupSyncService()
	}
	if execute == nil {
		execute = recovery.Execute
	}
	var from string
	var dryRun bool
	fromValue := singleUseStringValue{target: &from}

	cmd := &cobra.Command{
		Use:          "recover",
		Short:        "Recover stranded database rows into the configured database",
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			return validateCommandBeforeStartup(cmd, args, cobra.NoArgs, func() error {
				if from == "" {
					return fmt.Errorf("--from path must not be empty")
				}
				return nil
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.timing.report != nil {
				defer startup.timing.report.render(stderr)
			}
			cfg := startup.Config
			if cfg == nil {
				loaded, err := config.Load()
				if err != nil {
					return fmt.Errorf("load config for recovery: %w", err)
				}
				cfg = loaded
			}
			report, err := execute(cmd.Context(), recovery.Options{
				ActivePath: cfg.DatabasePath,
				FromPath:   from,
				DryRun:     dryRun,
			})
			if err != nil {
				if backupPath, ok := recovery.RestorableBackupPath(err); ok {
					_, _ = fmt.Fprintf(stderr, "manual recovery backup path: %s\n", backupPath)
				}
				return fmt.Errorf("recovery failed: %w", err)
			}
			if !dryRun {
				if err := syncService.sync(cmd.Context(), cfg, stderr, startup.timing); err != nil {
					installedPath := report.ActivePath
					if installedPath == "" {
						installedPath = cfg.DatabasePath
					}
					_, _ = fmt.Fprintf(stderr, "recovery replacement installed at: %s\n", installedPath)
					if report.BackupPath != "" {
						_, _ = fmt.Fprintf(stderr, "manual recovery backup path: %s\n", report.BackupPath)
					}
					return fmt.Errorf("post-recovery sync: %w", err)
				}
			}
			return printRecoveryReport(stdout, report, dryRun)
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

func printRecoveryReport(w io.Writer, report recovery.Report, dryRun bool) error {
	if dryRun {
		if _, err := fmt.Fprintln(w, "recovery dry run"); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(w, "recovery complete"); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	if err := writeOutput(w, "active path: %s\n", report.ActivePath); err != nil {
		return err
	}
	if err := writeOutput(w, "replacement target: %s\n", report.ActivePath); err != nil {
		return err
	}
	if err := writeOutput(w, "backup path: %s\n", report.BackupPath); err != nil {
		return err
	}
	for i, count := range report.InputCounts {
		if err := writeOutput(w, "input %d records: %d\n", i+1, count); err != nil {
			return err
		}
	}
	if err := writeOutput(w, "exact duplicates: %d\n", report.ExactDuplicates); err != nil {
		return err
	}
	if err := writeOutput(w, "final count: %d\n", report.FinalCount); err != nil {
		return err
	}
	for i, shape := range report.Shapes {
		if err := writeOutput(w, "input %d shape: version=%d signature=%s\n", i+1, shape.AppliedVersion, shape.Signature); err != nil {
			return err
		}
	}
	for _, conflict := range report.Conflicts {
		if err := writeOutput(w, "diagnostic: %s: %s\n", conflict.Code, strings.TrimSpace(conflict.Summary)); err != nil {
			return err
		}
	}
	return nil
}
