package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// outputMode selects presentation, not a shared payload schema.
type outputMode uint8

const (
	outputText outputMode = iota
	outputJSON
	outputRobot
)

type outputModeKey struct{}

func resolveOutputMode(json, robot bool) (outputMode, error) {
	if json && robot {
		return outputText, fmt.Errorf("--json and --robot are mutually exclusive")
	}
	if json {
		return outputJSON, nil
	}
	if robot {
		return outputRobot, nil
	}
	return outputText, nil
}

func invocationOutputMode(cmd *cobra.Command) outputMode {
	mode, _ := cmd.Context().Value(outputModeKey{}).(outputMode)
	return mode
}

func selectInvocationOutputMode(cmd *cobra.Command) error {
	mode, err := resolveOutputMode(commandBoolFlag(cmd, "json"), commandBoolFlag(cmd, "robot"))
	if err != nil {
		return err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), outputModeKey{}, mode))
	return nil
}

func describeOutputModes(cmd *cobra.Command) {
	if cmd.Flags().Lookup("json") != nil && cmd.Flags().Lookup("robot") != nil {
		cmd.MarkFlagsMutuallyExclusive("json", "robot")
		cmd.Flags().Lookup("json").Usage += " (mutually exclusive with --robot)"
		cmd.Flags().Lookup("robot").Usage += " (mutually exclusive with --json)"
	}
}
