package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: setup-echo <target>")
		os.Exit(1)
	}
	target := os.Args[1]
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create tool dir: %v\n", err)
		os.Exit(1)
	}
	content := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"printf 'normalized-env=%s\\n' \"${ECHO_GREETING:-}\"",
		"printf 'normalized-args=%s\\n' \"$*\"",
		"",
	}, "\n")
	if err := os.WriteFile(target, []byte(content), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "write tool shim: %v\n", err)
		os.Exit(1)
	}
}
