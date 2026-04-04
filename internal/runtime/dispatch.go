package runtime

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

const testHelperEnv = "TINX_CLI_HELPER_PROCESS"

type ProviderCommand struct {
	Name string
	Ref  string
}

type DispatchOptions struct {
	Home       string
	WorkingDir string
	Commands   []ProviderCommand
	Command    []string
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      *os.File
}

func Dispatch(opts DispatchOptions) error {
	if len(opts.Command) == 0 {
		return fmt.Errorf("command is required")
	}
	shimDir, cleanup, err := prepareShimDir(opts.Home, opts.Commands)
	if err != nil {
		return err
	}
	defer cleanup()

	env := envMapFromList(os.Environ())
	if opts.Home != "" {
		env["TINX_HOME"] = opts.Home
	}
	if existingPath := env["PATH"]; existingPath != "" {
		env["PATH"] = shimDir + string(os.PathListSeparator) + existingPath
	} else {
		env["PATH"] = shimDir
	}
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
		return fmt.Errorf("dispatch command: %w", err)
	}
	return nil
}

func prepareShimDir(home string, commands []ProviderCommand) (string, func(), error) {
	shimDir, err := os.MkdirTemp("", "tinx-dispatch-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create dispatch shim dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(shimDir)
	}
	for _, command := range commands {
		name := strings.TrimSpace(command.Name)
		if name == "" {
			cleanup()
			return "", func() {}, fmt.Errorf("provider command name cannot be empty")
		}
		if strings.ContainsRune(name, os.PathSeparator) {
			cleanup()
			return "", func() {}, fmt.Errorf("provider command %q must not contain path separators", name)
		}
		content, err := shimScript(home, command.Ref)
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
		path := filepath.Join(shimDir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("write provider shim %s: %w", name, err)
		}
	}
	return shimDir, cleanup, nil
}

func shimScript(home, providerRef string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve tinx executable: %w", err)
	}
	args := []string{executable}
	envAssignments := []string{}
	if strings.HasSuffix(filepath.Base(executable), ".test") {
		args = append(args, "-test.run=TestTinxCLIHelperProcess", "--")
		envAssignments = append(envAssignments, testHelperEnv+"=1")
	}
	args = append(args, "--tinx-home", home, "__dispatch-provider", providerRef)
	lines := []string{"#!/bin/sh", "set -eu"}
	for _, assignment := range envAssignments {
		parts := strings.SplitN(assignment, "=", 2)
		lines = append(lines, "export "+parts[0]+"="+shellQuote(parts[1]))
	}
	quoted := make([]string, 0, len(args)+1)
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	quoted = append(quoted, `"$@"`)
	lines = append(lines, "exec "+strings.Join(quoted, " "))
	return strings.Join(lines, "\n") + "\n", nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
	return "", fmt.Errorf("dispatch command: exec: %q: executable file not found in $PATH", command)
}
