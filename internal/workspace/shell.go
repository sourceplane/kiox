package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/sourceplane/kiox/internal/oci"
	kioxruntime "github.com/sourceplane/kiox/internal/runtime"
	"github.com/sourceplane/kiox/internal/runtimes"
	"github.com/sourceplane/kiox/internal/state"
)

type ShellBuildOptions struct {
	Out        io.Writer
	GlobalHome string
}

type ShellTarget struct {
	Alias string
	Tool  string
}

type ShellEnvironment struct {
	Env         map[string]string
	PathEntries []string
	EnvFile     string
	PathFile    string
	ShimDir     string
	Targets     map[string]ShellTarget
}

type shimTarget struct {
	Alias string
	Tool  string
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
		"KIOX_HOME":                home,
		"KIOX_GLOBAL_HOME":         firstNonEmpty(opts.GlobalHome, home),
		"KIOX_WORKSPACE_ROOT":      root,
		"KIOX_WORKSPACE_HOME":      home,
		"KIOX_WORKSPACE_ENV_FILE":  envFile,
		"KIOX_WORKSPACE_PATH_FILE": pathFile,
		"KIOX_WORKSPACE_PROVIDERS": filepath.Join(home, "providers"),
	}
	pathEntries := []string{shimDir}
	shimTargets := map[string]shimTarget{}
	registry, err := runtimes.NewBuiltinRegistry()
	if err != nil {
		return ShellEnvironment{}, err
	}
	kioxBinary, err := os.Executable()
	if err != nil {
		return ShellEnvironment{}, fmt.Errorf("resolve kiox binary: %w", err)
	}

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
		pkg, err := oci.LoadPackageModel(meta)
		if err != nil {
			return ShellEnvironment{}, err
		}
		defaultTool, ok := pkg.DefaultTool()
		if !ok {
			return ShellEnvironment{}, fmt.Errorf("provider %s/%s@%s has no default tool", meta.Namespace, meta.Name, meta.Version)
		}
		plugin, err := registry.MustGet(defaultTool.Spec.Runtime.Type)
		if err != nil {
			return ShellEnvironment{}, err
		}
		resolvedTool, err := plugin.Resolve(defaultTool, kioxruntime.Context{
			Home:          home,
			WorkspaceRoot: root,
			Alias:         alias,
			Metadata:      meta,
			Package:       pkg,
			GoOS:          goruntime.GOOS,
			GoArch:        goruntime.GOARCH,
			WorkingDir:    root,
			Stdout:        opts.Out,
			Stderr:        opts.Out,
		})
		if err != nil {
			return ShellEnvironment{}, err
		}
		providerEnv, providerPath, err := kioxruntime.ResolveProviderEnvironment(kioxruntime.ProviderEnvironmentSpec{
			Home:          home,
			WorkspaceRoot: root,
			Alias:         alias,
			ToolName:      defaultTool.Metadata.Name,
			BinaryPath:    resolvedTool.BinaryPath,
			Metadata:      meta,
		})
		if err != nil {
			return ShellEnvironment{}, err
		}
		if err := mergeWorkspaceEnvironment(env, providerEnv, alias); err != nil {
			return ShellEnvironment{}, err
		}
		pathEntries = appendWorkspacePaths(pathEntries, providerPath...)
		addAliasEnvironment(env, alias, meta, resolvedTool.BinaryPath, home)
		shimTargets[alias] = shimTarget{Alias: alias, Tool: defaultTool.Metadata.Name}
		for _, tool := range pkg.SortedTools() {
			for _, provided := range tool.Spec.Provides {
				if strings.TrimSpace(provided) == "" {
					continue
				}
				if _, exists := shimTargets[provided]; exists {
					continue
				}
				shimTargets[provided] = shimTarget{Alias: alias, Tool: tool.Metadata.Name}
			}
		}
	}
	shimNames := make([]string, 0, len(shimTargets))
	for name := range shimTargets {
		shimNames = append(shimNames, name)
	}
	sort.Strings(shimNames)
	for _, name := range shimNames {
		target := shimTargets[name]
		if err := writeProviderShim(filepath.Join(shimDir, name), kioxBinary, root, home, target.Alias, target.Tool); err != nil {
			return ShellEnvironment{}, err
		}
	}

	if err := writeEnvFile(envFile, env); err != nil {
		return ShellEnvironment{}, err
	}
	if err := writePathFile(pathFile, pathEntries); err != nil {
		return ShellEnvironment{}, err
	}
	targets := make(map[string]ShellTarget, len(shimTargets))
	for name, target := range shimTargets {
		targets[name] = ShellTarget{Alias: target.Alias, Tool: target.Tool}
	}
	return ShellEnvironment{
		Env:         env,
		PathEntries: pathEntries,
		EnvFile:     envFile,
		PathFile:    pathFile,
		ShimDir:     shimDir,
		Targets:     targets,
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
	env["KIOX_PROVIDER_"+aliasToken+"_REF"] = ref
	env["KIOX_PROVIDER_"+aliasToken+"_HOME"] = providerRoot
	env["KIOX_PROVIDER_"+aliasToken+"_BINARY"] = binaryPath
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

func writeProviderShim(path, kioxBinary, workspaceRoot, home, alias, tool string) error {
	content := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"KIOX_INTERNAL_CLI=1 exec " + shellQuote(kioxBinary) + " --kiox-home " + shellQuote(home) + " __shim --workspace-root " + shellQuote(workspaceRoot) + " --alias " + shellQuote(alias) + " --tool " + shellQuote(tool) + ` -- "$@"`,
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
	lines := []string{"# generated by kiox"}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
