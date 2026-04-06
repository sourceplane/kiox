package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/oci"
	tinxruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
)

type ShellBuildOptions struct {
	Out io.Writer
}

type ShellEnvironment struct {
	Env         map[string]string
	PathEntries []string
	EnvFile     string
	PathFile    string
	ShimDir     string
}

func BuildShellEnvironment(root, home string, aliases map[string]string, opts ShellBuildOptions) (ShellEnvironment, error) {
	stateRoot := Home(root)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return ShellEnvironment{}, fmt.Errorf("create workspace state root: %w", err)
	}
	shimDir := BinPath(root)
	if err := os.RemoveAll(shimDir); err != nil {
		return ShellEnvironment{}, fmt.Errorf("reset workspace shim dir: %w", err)
	}
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return ShellEnvironment{}, fmt.Errorf("create workspace shim dir: %w", err)
	}

	envFile := EnvPath(root)
	pathFile := PathPath(root)
	env := map[string]string{
		"TINX_HOME":                home,
		"TINX_WORKSPACE_ROOT":      root,
		"TINX_WORKSPACE_HOME":      home,
		"TINX_WORKSPACE_ENV_FILE":  envFile,
		"TINX_WORKSPACE_PATH_FILE": pathFile,
		"TINX_WORKSPACE_PROVIDERS": filepath.Join(home, "providers"),
	}
	pathEntries := []string{shimDir}

	aliasNames := make([]string, 0, len(aliases))
	for alias := range aliases {
		aliasNames = append(aliasNames, alias)
	}
	sort.Strings(aliasNames)

	for _, alias := range aliasNames {
		providerRef := strings.TrimSpace(aliases[alias])
		if providerRef == "" {
			continue
		}
		meta, err := state.LoadProviderMetadataByKey(home, providerRef)
		if err != nil {
			return ShellEnvironment{}, err
		}
		binaryPath := oci.CurrentBinaryPath(meta)
		if info, err := os.Stat(binaryPath); err != nil || info.IsDir() {
			binaryPath, err = oci.MaterializeRuntime(meta, goruntime.GOOS, goruntime.GOARCH, opts.Out)
			if err != nil {
				return ShellEnvironment{}, err
			}
		}
		providerEnv, providerPath, err := tinxruntime.ResolveProviderEnvironment(tinxruntime.ProviderEnvironmentSpec{
			Home:          home,
			WorkspaceRoot: root,
			Alias:         alias,
			BinaryPath:    binaryPath,
			Metadata:      meta,
		})
		if err != nil {
			return ShellEnvironment{}, err
		}
		if err := mergeWorkspaceEnvironment(env, providerEnv, alias); err != nil {
			return ShellEnvironment{}, err
		}
		pathEntries = appendWorkspacePaths(pathEntries, providerPath...)
		addAliasEnvironment(env, alias, meta, binaryPath, home)
		if err := writeProviderShim(filepath.Join(shimDir, alias), binaryPath); err != nil {
			return ShellEnvironment{}, err
		}
	}

	if err := writeEnvFile(envFile, env); err != nil {
		return ShellEnvironment{}, err
	}
	if err := writePathFile(pathFile, pathEntries); err != nil {
		return ShellEnvironment{}, err
	}
	return ShellEnvironment{
		Env:         env,
		PathEntries: pathEntries,
		EnvFile:     envFile,
		PathFile:    pathFile,
		ShimDir:     shimDir,
	}, nil
}

func splitProviderRef(ref string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(ref), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("provider ref must be <namespace>/<name>: %q", ref)
	}
	return parts[0], parts[1], nil
}

func mergeWorkspaceEnvironment(existing, additions map[string]string, alias string) error {
	for key, value := range additions {
		if current, ok := existing[key]; ok && current != value {
			return fmt.Errorf("workspace env conflict for %s while loading provider %s", key, alias)
		}
		existing[key] = value
	}
	return nil
}

func appendWorkspacePaths(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := append([]string(nil), existing...)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func addAliasEnvironment(env map[string]string, alias string, meta state.ProviderMetadata, binaryPath, home string) {
	aliasToken := sanitizeAlias(alias)
	if aliasToken == "" {
		return
	}
	providerRoot := state.MetadataStoreRoot(meta)
	ref := strings.TrimSpace(meta.Namespace) + "/" + strings.TrimSpace(meta.Name)
	env["TINX_PROVIDER_"+aliasToken+"_REF"] = ref
	env["TINX_PROVIDER_"+aliasToken+"_HOME"] = providerRoot
	env["TINX_PROVIDER_"+aliasToken+"_BINARY"] = binaryPath
}

func sanitizeAlias(alias string) string {
	var builder strings.Builder
	for _, value := range strings.ToUpper(strings.TrimSpace(alias)) {
		switch {
		case value >= 'A' && value <= 'Z':
			builder.WriteRune(value)
		case value >= '0' && value <= '9':
			builder.WriteRune(value)
		default:
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func writeProviderShim(path, binaryPath string) error {
	content := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"exec " + shellQuote(binaryPath) + ` "$@"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write workspace shim %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeEnvFile(path string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for key := range env {
		if strings.TrimSpace(key) == "" || key == "PATH" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{"# generated by tinx"}
	for _, key := range keys {
		lines = append(lines, "export "+key+"="+shellQuote(env[key]))
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write workspace env file: %w", err)
	}
	return nil
}

func writePathFile(path string, entries []string) error {
	content := strings.Join(entries, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write workspace path file: %w", err)
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
