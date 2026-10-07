package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablontiv/backscroll/internal/presets"
	"github.com/pablontiv/backscroll/internal/skillinstall"
	"github.com/spf13/cobra"
)

// embeddedBackscrollSkill is the authoritative instruction payload printed by
// the skill command and the --skill compatibility alias.
//
//go:embed backscroll_skill.md
var embeddedBackscrollSkill string

var errSkillMustBeUsedAlone = errors.New("usage: --skill must be used alone")

func validateSkillInvocation(args []string) (bool, error) {
	hasSkill := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--skill" || strings.HasPrefix(arg, "--skill=") {
			hasSkill = true
			break
		}
	}
	if !hasSkill {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--skill" {
		return true, nil
	}
	return false, errSkillMustBeUsedAlone
}

func newSkillCmd(stdout io.Writer) *cobra.Command {
	var install bool
	var restorePath string
	var approval string
	var home string
	var configHome string

	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print or install the Backscroll skill",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			restore := cmd.Flags().Changed("restore")
			if install && restore {
				return errors.New("--install and --restore are mutually exclusive")
			}
			if restore && restorePath == "" {
				return errors.New("--restore requires a receipt path")
			}
			if restore && approval == "" {
				return errors.New("--restore requires --approve")
			}
			if restore && cmd.Flags().Changed("config-home") {
				return errors.New("--config-home is only valid with --install")
			}
			if !install && !restore {
				for _, name := range []string{"approve", "home", "config-home", "install"} {
					if cmd.Flags().Changed(name) {
						return fmt.Errorf("--%s requires --install or --restore", name)
					}
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			restore := cmd.Flags().Changed("restore")
			switch {
			case install:
				resolvedHome, err := resolveSkillHome(home)
				if err != nil {
					return err
				}
				resolvedConfigHome, err := resolveConfigHome(resolvedHome, configHome)
				if err != nil {
					return err
				}
				shim, err := skillinstall.OfficialShim()
				if err != nil {
					return err
				}
				installer, err := skillinstall.New(skillinstall.Options{
					Home: resolvedHome, ConfigHome: resolvedConfigHome, Shim: shim,
				})
				if err != nil {
					return err
				}
				plan, err := installer.Plan()
				if err != nil {
					return err
				}
				if plan.RequiresApproval && approval == "" {
					return json.NewEncoder(stdout).Encode(struct {
						ApprovalRequired bool              `json:"approval_required"`
						Plan             skillinstall.Plan `json:"plan"`
					}{ApprovalRequired: true, Plan: plan})
				}
				result, err := installer.Apply(approval)
				if err != nil {
					return err
				}
				return encodeSkillInstallResult(stdout, result)
			case restore:
				resolvedHome, err := resolveSkillHome(home)
				if err != nil {
					return err
				}
				result, err := skillinstall.Restore(resolvedHome, restorePath, approval)
				if err != nil {
					return err
				}
				return json.NewEncoder(stdout).Encode(result)
			default:
				_, err := io.WriteString(stdout, embeddedBackscrollSkill)
				return err
			}
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "Install the Backscroll skill discovery shim")
	cmd.Flags().StringVar(&restorePath, "restore", "", "Restore an installation receipt")
	cmd.Flags().StringVar(&approval, "approve", "", "Approve an installation or restore digest")
	cmd.Flags().StringVar(&home, "home", "", "Use this home directory")
	cmd.Flags().StringVar(&configHome, "config-home", "", "Use this configuration home directory")
	return cmd
}

func newPresetsCmd(stdout io.Writer) *cobra.Command {
	var overwrite bool
	var configHome string
	presetsCmd := &cobra.Command{
		Use:   "presets",
		Short: "Manage Backscroll input presets",
	}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the shipped input presets",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			resolvedConfigHome, err := resolvePresetsConfigHome(configHome)
			if err != nil {
				return err
			}
			result, err := presets.Install(presets.Options{Overwrite: overwrite, ConfigHome: resolvedConfigHome})
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(result)
		},
	}
	installCmd.Flags().BoolVar(&overwrite, "overwrite", false, "Replace shipped preset names")
	installCmd.Flags().StringVar(&configHome, "config-home", "", "Use this configuration home directory")
	presetsCmd.AddCommand(installCmd)
	return presetsCmd
}

func resolveSkillHome(value string) (string, error) {
	if value == "" {
		var err error
		value, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("home must be an absolute path")
	}
	info, err := os.Stat(value)
	if err != nil {
		return "", fmt.Errorf("inspect home: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("home must be an existing directory")
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func resolvePresetsConfigHome(value string) (string, error) {
	home := ""
	if value == "" && os.Getenv("XDG_CONFIG_HOME") == "" {
		var err error
		home, err = resolveSkillHome("")
		if err != nil {
			return "", err
		}
	}
	return resolveConfigHome(home, value)
}

func resolveConfigHome(home, value string) (string, error) {
	if value == "" {
		value = os.Getenv("XDG_CONFIG_HOME")
	}
	if value == "" {
		value = filepath.Join(home, ".config")
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("config home must be an absolute path")
	}
	resolved, err := resolvePathWithMissingTail(value)
	if err != nil {
		return "", fmt.Errorf("resolve config home: %w", err)
	}
	return resolved, nil
}

func resolvePathWithMissingTail(path string) (string, error) {
	current := filepath.Clean(path)
	var tail []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for index := len(tail) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, tail[index])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

func encodeSkillInstallResult(stdout io.Writer, result skillinstall.ApplyResult) error {
	var receipt any
	var restoreApproval any
	if result.Receipt != "" {
		receipt = result.Receipt
	}
	if result.RestoreApproval != "" {
		restoreApproval = result.RestoreApproval
	}
	return json.NewEncoder(stdout).Encode(struct {
		Changed         int `json:"changed"`
		Receipt         any `json:"receipt"`
		RestoreApproval any `json:"restore_approval"`
	}{Changed: result.Changed, Receipt: receipt, RestoreApproval: restoreApproval})
}
