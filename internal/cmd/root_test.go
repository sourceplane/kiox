package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(buf.String(), "OCI-native provider runtime") {
		t.Fatalf("unexpected help output: %s", buf.String())
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(buf.String(), "kiox") {
		t.Fatalf("unexpected version output: %s", buf.String())
	}
}
