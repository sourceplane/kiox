package cmd

import (
	"context"
	"fmt"
	"os"
)

func init() {
	if os.Getenv("TINX_INTERNAL_CLI") != "1" {
		return
	}
	if err := executeCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
