package runtime

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
)

type ExecOptions struct {
	BinaryPath   string
	Args         []string
	WorkingDir   string
	ProviderHome string
	TinxVersion  string
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        *os.File
}

func Execute(opts ExecOptions) error {
	cmd := osexec.Command(opts.BinaryPath, opts.Args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}
	cmd.Env = append(os.Environ(),
		"TINX_PROVIDER_HOME="+opts.ProviderHome,
		"TINX_VERSION="+opts.TinxVersion,
		"TINX_CONTEXT={}",
	)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute provider: %w", err)
	}
	return nil
}
