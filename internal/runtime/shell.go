package runtime

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

type ShellOptions struct {
	WorkingDir  string
	Env         map[string]string
	PathEntries []string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       *os.File
}

type ShellCommandOptions struct {
	ShellOptions
	Command []string
}

func RunCommand(opts ShellCommandOptions) error {
	if len(opts.Command) == 0 {
		return fmt.Errorf("command is required")
	}
	env := prepareShellEnv(opts.ShellOptions)
	commandPath, err := resolveCommandPath(opts.Command[0], env["PATH"])
	if err != nil {
		return err
	}
	command := osexec.Command(commandPath, opts.Command[1:]...)
	if opts.WorkingDir != "" {
		command.Dir = opts.WorkingDir
	}
	command.Env = envListFromMap(env)
	command.Stdout = opts.Stdout
	command.Stderr = opts.Stderr
	command.Stdin = opts.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("run command: %w", err)
	}
	return nil
}

func RunInteractiveShell(opts ShellOptions) error {
	shellPath := strings.TrimSpace(os.Getenv("SHELL"))
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	shellArgs := interactiveShellArgs(shellPath)
	command := osexec.Command(shellPath, shellArgs...)
	if opts.WorkingDir != "" {
		command.Dir = opts.WorkingDir
	}
	command.Env = envListFromMap(prepareShellEnv(opts))
	command.Stdout = opts.Stdout
	command.Stderr = opts.Stderr
	command.Stdin = opts.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("launch interactive shell: %w", err)
	}
	return nil
}

func prepareShellEnv(opts ShellOptions) map[string]string {
	env := envMapFromList(os.Environ())
	for key, value := range opts.Env {
		env[key] = value
	}
	applyPathEntries(env, opts.PathEntries)
	return env
}

func interactiveShellArgs(shellPath string) []string {
	base := filepath.Base(shellPath)
	switch base {
	case "sh", "bash", "zsh", "ksh", "dash", "fish":
		return []string{"-i"}
	default:
		return nil
	}
}

func resolveCommandPath(command, pathEnv string) (string, error) {
	if strings.ContainsRune(command, os.PathSeparator) {
		return command, nil
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("run command: exec: %q: executable file not found in $PATH", command)
}
