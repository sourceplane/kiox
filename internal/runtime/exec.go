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
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        *os.File
}

func Execute(opts ExecOptions) error {
	cmd := osexec.Command(opts.BinaryPath, opts.Args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}
	env := envMapFromList(os.Environ())
	env["TINX_PROVIDER_HOME"] = opts.ProviderHome
	env["TINX_VERSION"] = opts.TinxVersion
	env["TINX_CONTEXT"] = "{}"
	for key, value := range opts.EnvOverrides {
		env[key] = value
	}
	cmd.Env = envListFromMap(env)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute provider: %w", err)
	}
	return nil
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
