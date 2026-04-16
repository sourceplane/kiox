package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

type initProviderSpec struct {
	Source string
	Alias  string
}

type initCommandInput struct {
	Target    string
	Providers []initProviderSpec
	ShowHelp  bool
	Verbose   bool
}

func newInitCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "init <workspace-or-config> [-p <provider-source> [as <alias>]]...",
		Short:              "Create or materialize a provider workspace",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitCommand(cmd, root, args, ".")
		},
	}
	return cmd
}

func runInitCommand(cmd *cobra.Command, root *rootOptions, args []string, defaultTarget string) error {
	input, err := parseInitCommandArgs(args, defaultTarget)
	if err != nil {
		return err
	}
	if input.ShowHelp {
		return cmd.Help()
	}
	target, err := buildInitWorkspaceTarget(input)
	if err != nil {
		return err
	}
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	_, manifestPath, err := applyWorkspaceConfigChange(cmd.Context(), cmd.ErrOrStderr(), globalHome, target, target.Config, input.Verbose)
	if err != nil {
		return err
	}
	if err := rememberWorkspaceTarget(globalHome, target); err != nil {
		return err
	}
	if err := state.SaveActiveWorkspace(globalHome, target.Root); err != nil {
		return err
	}
	writeLine(cmd.OutOrStdout(), "initialized workspace %s", target.Config.Name())
	writeLine(cmd.OutOrStdout(), "active workspace: %s", target.Config.Name())
	writeLine(cmd.OutOrStdout(), "manifest: %s", displayWorkspaceSummaryFilePath(manifestPath))
	writeLine(cmd.OutOrStdout(), "home: %s", displayWorkspaceSummaryDirPath(target.Root))
	return nil
}

func parseInitCommandArgs(args []string, defaultTarget string) (initCommandInput, error) {
	input := initCommandInput{Target: defaultTarget}
	if len(args) == 0 {
		return input, nil
	}
	targetSet := false
	for index := 0; index < len(args); {
		arg := args[index]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			input.ShowHelp = true
			return input, nil
		case arg == "-v" || arg == "--verbose":
			input.Verbose = true
			index++
		case arg == "-p" || arg == "--provider":
			if index+1 >= len(args) {
				return initCommandInput{}, fmt.Errorf("missing provider source after %s", arg)
			}
			spec := initProviderSpec{Source: args[index+1]}
			index += 2
			if index < len(args) && args[index] == "as" {
				if index+1 >= len(args) {
					return initCommandInput{}, fmt.Errorf("missing alias after as")
				}
				spec.Alias = args[index+1]
				index += 2
			}
			input.Providers = append(input.Providers, spec)
		case strings.HasPrefix(arg, "--provider="):
			input.Providers = append(input.Providers, initProviderSpec{Source: strings.TrimPrefix(arg, "--provider=")})
			index++
		case strings.HasPrefix(arg, "-p="):
			input.Providers = append(input.Providers, initProviderSpec{Source: strings.TrimPrefix(arg, "-p=")})
			index++
		case !targetSet:
			input.Target = arg
			targetSet = true
			index++
		default:
			return initCommandInput{}, fmt.Errorf("unexpected argument %q", arg)
		}
	}
	return input, nil
}

func buildInitWorkspaceTarget(input initCommandInput) (*workspaceTarget, error) {
	target := strings.TrimSpace(input.Target)
	if target == "" {
		return nil, fmt.Errorf("workspace target is required")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace target: %w", err)
	}
	if (strings.HasSuffix(strings.ToLower(absTarget), ".yaml") || strings.HasSuffix(strings.ToLower(absTarget), ".yml")) && len(input.Providers) == 0 {
		if _, err := os.Stat(absTarget); os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace config file %s does not exist", absTarget)
		}
	}
	if info, err := os.Stat(absTarget); err == nil && !info.IsDir() {
		if len(input.Providers) > 0 {
			return nil, fmt.Errorf("provider flags cannot be combined with an existing workspace config file")
		}
		config, loadErr := workspace.Load(absTarget)
		if loadErr != nil {
			return nil, loadErr
		}
		return &workspaceTarget{Root: filepath.Dir(absTarget), ConfigPath: absTarget, Config: config}, nil
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat workspace target: %w", err)
	}
	if existingConfig, configPath, ok, err := loadWorkspaceConfigAtRootIfPresent(absTarget); err != nil {
		return nil, err
	} else if ok {
		if len(input.Providers) > 0 {
			return nil, fmt.Errorf("provider flags cannot be combined with an existing workspace manifest")
		}
		return &workspaceTarget{Root: absTarget, ConfigPath: configPath, Config: existingConfig}, nil
	}
	if len(input.Providers) == 0 {
		lowerTarget := strings.ToLower(absTarget)
		if strings.HasSuffix(lowerTarget, ".yaml") || strings.HasSuffix(lowerTarget, ".yml") {
			return nil, fmt.Errorf("workspace init requires a directory target when no providers are supplied")
		}
	}
	config := workspace.Config{
		APIVersion: workspace.APIVersionV1,
		Kind:       workspace.KindWorkspace,
		Metadata: workspace.Metadata{
			Name: filepath.Base(absTarget),
		},
		Providers: map[string]workspace.Provider{},
	}
	for _, provider := range input.Providers {
		alias := strings.TrimSpace(provider.Alias)
		if alias == "" {
			alias = defaultAliasForSource(provider.Source)
		}
		if alias == "" {
			return nil, fmt.Errorf("could not derive alias for provider %q", provider.Source)
		}
		config.Providers[alias] = workspace.Provider{Source: normalizeInitSource(provider.Source)}
	}
	if err := config.Normalize(); err != nil {
		return nil, err
	}
	return &workspaceTarget{Root: absTarget, ConfigPath: workspace.ManifestPath(absTarget), Config: config}, nil
}

func normalizeInitSource(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	if info, err := os.Stat(trimmed); err == nil && info.IsDir() {
		if absPath, absErr := filepath.Abs(trimmed); absErr == nil {
			return absPath
		}
	}
	return trimmed
}

func defaultAliasForSource(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	if at := strings.Index(trimmed, "@"); at >= 0 {
		trimmed = trimmed[:at]
	}
	if colon := strings.LastIndex(trimmed, ":"); colon >= 0 {
		after := trimmed[colon+1:]
		if !strings.Contains(after, "/") {
			trimmed = trimmed[:colon]
		}
	}
	trimmed = filepath.Base(trimmed)
	trimmed = strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
	return strings.TrimSpace(trimmed)
}
