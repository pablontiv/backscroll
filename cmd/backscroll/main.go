package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

func main() {
	if err := run(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stdout, _ io.Writer, _ []string) error {
	fmt.Fprintln(stdout, "backscroll "+version)
	return nil
}
