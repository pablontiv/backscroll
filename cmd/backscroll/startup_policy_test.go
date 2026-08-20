package main

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootExcludesDirectReadCommand(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	for _, cmd := range root.Commands() {
		if cmd.Name() == "read" {
			t.Fatal("public read command is registered")
		}
	}
}

func TestCommandTreeExcludesIndexedOnlyFlag(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if flag := cmd.Flags().Lookup("indexed-only"); flag != nil {
			t.Errorf("%s registers forbidden --indexed-only", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
