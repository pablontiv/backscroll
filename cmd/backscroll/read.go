package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/reader"
)

func newReadCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read <path>",
		Short: "Read a specific session or plan file",
		Long:  `Read displays the contents of a session file or plan in human-readable format.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRead(stdout, stderr, args[0])
		},
	}

	return cmd
}

func runRead(stdout, stderr io.Writer, path string) error {
	// Read the session file
	messages, err := reader.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Format and print
	for i, msg := range messages {
		_, _ = fmt.Fprintf(stdout, "=== Message %d ===\n", i+1)
		_, _ = fmt.Fprintf(stdout, "Role: %s\n", msg.Role)
		_, _ = fmt.Fprintf(stdout, "ContentType: %s\n", msg.ContentType)
		_, _ = fmt.Fprintf(stdout, "Timestamp: %s\n", msg.Timestamp.Format("2006-01-02 15:04:05 MST"))
		_, _ = fmt.Fprintf(stdout, "\n%s\n\n", msg.Content)
	}

	_, _ = fmt.Fprintf(stdout, "Total messages: %d\n", len(messages))

	return nil
}
