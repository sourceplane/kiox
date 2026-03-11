package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
)

func newRunCommand(root *rootOptions) *cobra.Command {
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "run <provider-or-alias> <capability> [args...]",
		Short: "Execute a provider capability from an alias, installed provider, or OCI reference",
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plainHTTP, filteredArgs, err := parseRunArgs(args, plainHTTP)
			if err != nil {
				return err
			}
			if len(filteredArgs) == 0 {
				return fmt.Errorf("provider or alias is required")
			}
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			input := filteredArgs[0]
			aliasName := ""
			ref := input
			if aliases, err := state.LoadAliases(home); err == nil {
				if aliased, ok := aliases[input]; ok {
					aliasName = input
					ref = aliased
				}
			}
			providerMeta, err := resolveProviderForRun(home, ref, plainHTTP, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			helpAlias := aliasName
			if helpAlias == "" {
				helpAlias = input
			}
			if len(filteredArgs) == 1 || isHelpToken(filteredArgs[1]) {
				writeProviderHelp(cmd.OutOrStdout(), helpAlias, ref, providerMeta)
				return nil
			}
			if len(filteredArgs) >= 3 && isHelpToken(filteredArgs[2]) {
				writeCapabilityHelp(cmd.OutOrStdout(), helpAlias, filteredArgs[1], providerMeta)
				return nil
			}

			return executeProviderCapability(cmd, home, ref, providerMeta, filteredArgs[1:])
		},
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}

func parseRunArgs(args []string, plainHTTP bool) (bool, []string, error) {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--plain-http" {
			plainHTTP = true
			if index+1 < len(args) {
				next := strings.ToLower(args[index+1])
				if next == "true" || next == "false" {
					parsed, err := strconv.ParseBool(next)
					if err != nil {
						return false, nil, fmt.Errorf("invalid value for --plain-http: %q", args[index+1])
					}
					plainHTTP = parsed
					index++
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "--plain-http=") {
			value := strings.TrimPrefix(arg, "--plain-http=")
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return false, nil, fmt.Errorf("invalid value for --plain-http: %q", value)
			}
			plainHTTP = parsed
			continue
		}
		filtered = append(filtered, arg)
	}
	return plainHTTP, filtered, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
