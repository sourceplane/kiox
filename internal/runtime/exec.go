package runtime

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"sort"
	"strings"
)

type ExecOptions struct {
	BinaryPath   string
	Args         []string
	WorkingDir   string
	ProviderHome string
	TinxVersion  string
	EnvOverrides map[string]string
	PathEntries  []string
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        *os.File
}

func Execute(opts ExecOptions) error {
	cmd := osexec.Command(opts.BinaryPath, opts.Args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}
	cmd.Env = CommandEnvironment(os.Environ(), withProviderEnvironment(opts), opts.PathEntries)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute provider: %w", err)
	}
	return nil
}

func CommandEnvironment(base []string, overrides map[string]string, pathEntries []string) []string {
	env := envMapFromList(base)
	for key, value := range overrides {
		env[key] = value
	}
	applyPathEntries(env, pathEntries)
	return envListFromMap(env)
}

func withProviderEnvironment(opts ExecOptions) map[string]string {
	env := make(map[string]string, len(opts.EnvOverrides)+3)
	for key, value := range opts.EnvOverrides {
		env[key] = value
	}
	env["TINX_PROVIDER_HOME"] = opts.ProviderHome
	env["TINX_VERSION"] = opts.TinxVersion
	env["TINX_CONTEXT"] = "{}"
	env["TINX_INTERNAL_CLI"] = ""
	return env
}

func applyPathEntries(env map[string]string, pathEntries []string) {
	filtered := make([]string, 0, len(pathEntries))
	seen := make(map[string]struct{}, len(pathEntries))
	for _, entry := range pathEntries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return
	}
	existingPath := strings.TrimSpace(env["PATH"])
	if existingPath != "" {
		filtered = append(filtered, existingPath)
	}
	env["PATH"] = strings.Join(filtered, string(os.PathListSeparator))
}

func envMapFromList(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result
}

func envListFromMap(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
