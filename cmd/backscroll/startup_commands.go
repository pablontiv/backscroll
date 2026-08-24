package main

import "github.com/spf13/cobra"

type startupCommandClass string

const (
	startupSnapshotRead startupCommandClass = "snapshot-read"
	startupMetadataRead startupCommandClass = "metadata-read"
	startupMutation     startupCommandClass = "mutation"
	startupRemediation  startupCommandClass = "remediation"
	startupClassKey                         = "backscroll.io/startup-class"
)

func startupCommandClassFor(cmd *cobra.Command) (startupCommandClass, bool) {
	if cmd.Annotations == nil {
		return startupMutation, false
	}
	class := startupCommandClass(cmd.Annotations[startupClassKey])
	switch class {
	case startupSnapshotRead, startupMetadataRead, startupMutation, startupRemediation:
		return class, true
	default:
		return startupMutation, false
	}
}

func startupClassRetainsLease(class startupCommandClass) bool {
	return class == startupMutation || class == startupRemediation
}

func registerStartupCommand(root *cobra.Command, class startupCommandClass, cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[startupClassKey] = string(class)
	if startupClassRetainsLease(class) {
		cmd.RunE = wrapLeaseRetainingRunE(cmd.RunE)
	}
	root.AddCommand(cmd)
}

func wrapLeaseRetainingRunE(runE func(*cobra.Command, []string) error) func(*cobra.Command, []string) (retErr error) {
	return func(cmd *cobra.Command, args []string) (retErr error) {
		defer func() { retErr = startupResultFrom(cmd).release(retErr) }()
		return runE(cmd, args)
	}
}
