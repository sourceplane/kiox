package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/resolver"
	cmdruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
)

func newRunCommand(root *rootOptions) *cobra.Command {
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "run <provider-or-alias> [capability] [args...]",
		Short: "Execute a provider capability from an alias, installed provider, or remote reference",
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitRunCommandArgs(args, plainHTTP)
			plainHTTP, filteredArgs, err := parseRunArgs(beforeDash, plainHTTP)
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
			resolvedRef := ref
			if aliasName == "" {
				resolvedRef = resolver.ResolveProviderSource(ref)
			}
			providerMeta, err := resolveProviderForRun(home, resolvedRef, plainHTTP, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if len(afterDash) > 0 {
				commandName := aliasName
				if commandName == "" {
					commandName = commandNameForProvider(providerMeta)
				}
				return cmdruntime.Dispatch(cmdruntime.DispatchOptions{
					Home:       home,
					WorkingDir: mustGetwd(),
					Commands:   []cmdruntime.ProviderCommand{{Name: commandName, Ref: providerMeta.Namespace + "/" + providerMeta.Name}},
					Command:    afterDash,
					Stdout:     cmd.OutOrStdout(),
					Stderr:     cmd.ErrOrStderr(),
					Stdin:      os.Stdin,
				})
			}
			helpAlias := aliasName
			if helpAlias == "" {
				helpAlias = input
			}
			invocation := planProviderInvocation(providerMeta, filteredArgs[1:])
			if invocation.showProviderHelp {
				writeProviderHelp(cmd.OutOrStdout(), helpAlias, input, providerMeta)
				return nil
			}
			if invocation.showCapabilityHelp {
				writeCapabilityHelp(cmd.OutOrStdout(), helpAlias, input, invocation.capability, providerMeta)
				return nil
			}

			return executeProviderCapability(cmd, home, input, providerMeta, invocation.args)
		},
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}

func splitRunCommandArgs(args []string, plainHTTP bool) ([]string, []string) {
	for index, arg := range args {
		if arg != "--" {
			continue
		}
		_, filteredBefore, err := parseRunArgs(args[:index], plainHTTP)
		if err != nil {
			return args, nil
		}
		if len(filteredBefore) == 1 {
			return args[:index], args[index+1:]
		}
		break
	}
	return args, nil
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
