package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/gha"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/resolver"
	cmdruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/pkg/version"
)

func newInstallCommand(root *rootOptions) *cobra.Command {
	var source string
	var tag string
	var plainHTTP bool
	var inputValues []string

	cmd := &cobra.Command{
		Use:   "install <ref> [as <alias>] [-- command...]",
		Short: "Install provider metadata from an OCI layout, registry reference, or GitHub Action",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			alias, installTarget, err := parseInstallTarget(filterInstallArgs(beforeDash))
			if err != nil {
				return err
			}
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			installInputs, err := parseInstallInputs(inputValues)
			if err != nil {
				return err
			}

			resolvedTarget := resolver.ResolveProviderSource(installTarget)
			if source == "" {
				var installed state.ProviderMetadata
				if gha.IsReference(resolvedTarget) {
					if len(installInputs) > 0 && alias == "" {
						return fmt.Errorf("GitHub Action install-time inputs require an alias so the configured provider can be stored")
					}
					if alias != "" {
						installed, err = gha.InstallAlias(cmd.Context(), home, resolvedTarget, gha.InstallOptions{
							Alias:       alias,
							Inputs:      installInputs,
							WorkingDir:  mustGetwd(),
							TinxVersion: version.String(),
							Stdout:      cmd.OutOrStdout(),
							Stderr:      cmd.ErrOrStderr(),
							Stdin:       os.Stdin,
						}, cmd.ErrOrStderr())
					} else {
						installed, err = gha.Install(cmd.Context(), home, resolvedTarget, cmd.ErrOrStderr())
					}
				} else {
					if len(installInputs) > 0 {
						return fmt.Errorf("--input is only supported for GitHub Action installs")
					}
					installed, err = oci.InstallRemote(cmd.Context(), home, resolvedTarget, alias, plainHTTP, cmd.ErrOrStderr())
				}
				if err != nil {
					return err
				}
				writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
				if len(afterDash) == 0 {
					return nil
				}
				commandName := alias
				if commandName == "" {
					commandName = commandNameForProvider(installed)
				}
				return cmdruntime.Dispatch(cmdruntime.DispatchOptions{
					Home:       home,
					WorkingDir: mustGetwd(),
					Commands:   []cmdruntime.ProviderCommand{{Name: commandName, Ref: installed.Namespace + "/" + installed.Name}},
					Command:    afterDash,
					Stdout:     cmd.OutOrStdout(),
					Stderr:     cmd.ErrOrStderr(),
					Stdin:      os.Stdin,
				})
			}
			if len(installInputs) > 0 {
				return fmt.Errorf("--input is only supported for GitHub Action installs")
			}

			if _, _, err := splitProviderRef(installTarget); err != nil {
				return fmt.Errorf("when using --source, ref must be <namespace>/<name>")
			}
			absSource, err := filepath.Abs(source)
			if err != nil {
				return fmt.Errorf("resolve source path: %w", err)
			}
			if _, err := os.Stat(absSource); err != nil {
				return fmt.Errorf("open source layout: %w", err)
			}
			installed, err := oci.InstallMetadata(absSource, tag, home, alias, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if installed.Namespace+"/"+installed.Name != installTarget {
				return fmt.Errorf("installed provider %s/%s does not match requested ref %s", installed.Namespace, installed.Name, installTarget)
			}
			writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
			if len(afterDash) == 0 {
				return nil
			}
			commandName := alias
			if commandName == "" {
				commandName = commandNameForProvider(installed)
			}
			return cmdruntime.Dispatch(cmdruntime.DispatchOptions{
				Home:       home,
				WorkingDir: mustGetwd(),
				Commands:   []cmdruntime.ProviderCommand{{Name: commandName, Ref: installed.Namespace + "/" + installed.Name}},
				Command:    afterDash,
				Stdout:     cmd.OutOrStdout(),
				Stderr:     cmd.ErrOrStderr(),
				Stdin:      os.Stdin,
			})
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "path to a local OCI image layout")
	cmd.Flags().StringVar(&tag, "tag", "", "OCI tag inside the local layout")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	cmd.Flags().StringArrayVar(&inputValues, "input", nil, "configure a GitHub Action input at install time (name=value)")
	return cmd
}

func filterInstallArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--source" || arg == "--tag" || arg == "--input":
			if index+1 < len(args) {
				index++
			}
		case arg == "--plain-http":
			if index+1 < len(args) {
				next := strings.ToLower(args[index+1])
				if next == "true" || next == "false" {
					index++
				}
			}
		case strings.HasPrefix(arg, "--source=") || strings.HasPrefix(arg, "--tag=") || strings.HasPrefix(arg, "--input=") || strings.HasPrefix(arg, "--plain-http="):
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func parseInstallTarget(args []string) (string, string, error) {
	if len(args) >= 3 && args[1] == "as" {
		return args[2], args[0], nil
	}
	switch len(args) {
	case 1:
		return "", args[0], nil
	case 2:
		if looksLikeProviderSource(args[1]) && !looksLikeProviderSource(args[0]) {
			return args[0], args[1], nil
		}
	}
	return "", "", fmt.Errorf("install expects <ref>, <alias> <ref>, or <ref> as <alias>")
}

func looksLikeProviderSource(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if gha.IsReference(trimmed) || isOCIReference(trimmed) {
		return true
	}
	if resolver.ResolveProviderSource(trimmed) != trimmed {
		return true
	}
	if _, err := os.Stat(trimmed); err == nil {
		return true
	}
	return false
}

func parseInstallInputs(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	inputs := make(map[string]string, len(values))
	for _, raw := range values {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("GitHub Action inputs must use name=value syntax, got %q", raw)
		}
		inputs[strings.TrimSpace(parts[0])] = parts[1]
	}
	return inputs, nil
}
