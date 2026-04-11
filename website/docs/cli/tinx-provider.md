---
title: tinx provider
---

`tinx provider` manages providers declared in a workspace. The shorter `tinx add`, `tinx remove`, `tinx update`, and `tinx list` commands map to the same workflows.

## Main command help

```text
Manage workspace providers and provider inventory

Usage:
  tinx provider [command]

Aliases:
  provider, providers, p

Available Commands:
  add         Add a provider to the current or selected workspace
  list        List providers for the current, named, or default scope
  remove      Remove a provider from the current or selected workspace
  update      Refresh provider metadata for the current or selected workspace

Flags:
  -h, --help   help for provider

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands

Use "tinx provider [command] --help" for more information about a command.
```

## `provider add`

Add a provider source to the workspace manifest and sync it immediately.

```text
Add a provider to the current or selected workspace

Usage:
  tinx provider add <provider> [as <alias>] [flags]

Flags:
  -h, --help         help for add
      --plain-http   use plain HTTP for registry pulls in this workspace

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx provider add core/node as node
tinx add ghcr.io/acme/kubectl:v1.31.0 as kubectl
tinx provider add ./oci as echo
```

## `provider list`

List providers for the current workspace, a named workspace, or the default scope.

```text
List providers for the current, named, or default scope

Usage:
  tinx provider list [workspace|default] [flags]

Flags:
  -h, --help   help for list

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx provider list
tinx provider list demo
tinx provider list default
tinx list providers
```

## `provider remove`

Remove a provider from the workspace manifest and refresh workspace state.

```text
Remove a provider from the current or selected workspace

Usage:
  tinx provider remove <provider-or-alias> [flags]

Flags:
  -h, --help   help for remove

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx provider remove node
tinx remove lite-ci
```

## `provider update`

Refresh provider metadata for all providers or only the named aliases.

```text
Refresh provider metadata for the current or selected workspace

Usage:
  tinx provider update [provider-or-alias...] [flags]

Flags:
  -h, --help   help for update

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx provider update
tinx provider update node lite-ci
tinx update node
```

## Related inventory commands

The top-level `list` command exposes both provider and workspace inventory:

```text
List providers or inspect workspace inventory

Usage:
  tinx list [workspace|default] [flags]
  tinx list [command]

Aliases:
  list, ls

Available Commands:
  providers   List installed providers for the current, named, or default scope
  workspaces  List workspace scopes
```
