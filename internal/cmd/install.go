package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/kiox/internal/oci"
	"github.com/sourceplane/kiox/internal/resolver"
	"github.com/sourceplane/kiox/internal/state"
)

func newInstallCommand(root *rootOptions) *cobra.Command {
	var source string
	var tag string
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "install <ref> [as <alias>]",
		Short: "Install provider metadata from an OCI layout or registry reference",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(afterDash) > 0 {
				return fmt.Errorf("install no longer executes commands; add the provider to a workspace and use 'kiox -- %s' instead", strings.Join(afterDash, " "))
			}
			alias, installTarget, err := parseInstallTarget(filterInstallArgs(beforeDash))
			if err != nil {
				return err
			}
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			storeHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}

			resolvedTarget := resolver.ResolveProviderSource(installTarget)
			if source == "" {
				if resolver.HasSourceScheme(resolvedTarget) {
					return unsupportedProviderSourceError(resolvedTarget, "expected an OCI registry reference or --source <oci-layout>")
				}
				var installed state.ProviderMetadata
				installed, err = oci.InstallRemote(cmd.Context(), home, storeHome, resolvedTarget, alias, plainHTTP, cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
				return nil
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
			installed, err := oci.InstallMetadata(absSource, tag, home, storeHome, alias, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if installed.Namespace+"/"+installed.Name != installTarget {
				return fmt.Errorf("installed provider %s/%s does not match requested ref %s", installed.Namespace, installed.Name, installTarget)
			}
			writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "path to a local OCI image layout")
	cmd.Flags().StringVar(&tag, "tag", "", "OCI tag inside the local layout")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}

func filterInstallArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--source" || arg == "--tag":
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
		case strings.HasPrefix(arg, "--source=") || strings.HasPrefix(arg, "--tag=") || strings.HasPrefix(arg, "--plain-http="):
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
	if resolver.HasSourceScheme(trimmed) || isOCIReference(trimmed) {
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

func isOCIReference(ref string) bool {
	if strings.Contains(ref, "@") {
		return true
	}
	if strings.Count(ref, "/") >= 2 {
		return true
	}
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		after := ref[idx+1:]
		if !strings.Contains(after, "/") {
			return true
		}
	}
	if slash := strings.Index(ref, "/"); slash > 0 {
		return strings.Contains(ref[:slash], ".")
	}
	return false
}
