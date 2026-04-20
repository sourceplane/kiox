---
title: kiox provider
---

`kiox provider` manages providers declared in a workspace. The shorter `kiox add`, `kiox remove`, and `kiox update` commands map to the same workflows.

## Main command help

```text
Manage workspace providers and provider inventory

Usage:
  kiox provider [command]

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
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands

Use "kiox provider [command] --help" for more information about a command.
```

## `provider add`

Add a provider source to the workspace manifest and sync it immediately. kiox resolves, installs, and validates the provider first; only successful adds rewrite `kiox.yaml`, update `kiox.lock`, and refresh the workspace shell artifacts.

```text
Add a provider to the current or selected workspace

Usage:
  kiox provider add <provider> [as <alias>] [flags]

Flags:
  -h, --help         help for add
      --plain-http   use plain HTTP for registry pulls in this workspace

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox provider add core/node as node
kiox add ghcr.io/acme/kubectl:v1.31.0 as kubectl
kiox provider add ./oci as echo
```

## `provider list`

List providers for the current workspace, a named workspace, or the default scope.

This is provider-only inventory. If you want tool inventory too, use `kiox ls` or `kiox status`.

```text
List providers for the current, named, or default scope

Usage:
  kiox provider list [workspace|default] [flags]

Flags:
  -h, --help   help for list

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox provider list
kiox provider list demo
kiox provider list default
kiox ls demo
```

## `provider remove`

Remove a provider from the workspace manifest and refresh workspace state. Successful removal also clears the provider from `kiox.lock` and workspace runtime state.

```text
Remove a provider from the current or selected workspace

Usage:
  kiox provider remove <provider-or-alias> [flags]

Flags:
  -h, --help   help for remove

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox provider remove node
kiox remove lite-ci
```

## `provider update`

Refresh provider metadata for all providers or only the named aliases.

```text
Refresh provider metadata for the current or selected workspace

Usage:
  kiox provider update [provider-or-alias...] [flags]

Flags:
  -h, --help   help for update

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox provider update
kiox provider update node lite-ci
kiox update node
```

If you edit `kiox.yaml` by hand, run `kiox sync` to reconcile it explicitly. Normal workspace entry points such as `kiox status`, `kiox exec`, `kiox shell`, and `kiox -- ...` also reconcile automatically.

## Related inventory commands

The top-level `list` command exposes both provider and tool inventory for workspace scopes:

```text
List providers and tools or inspect workspace inventory

Usage:
  kiox list [workspace|default] [flags]
  kiox list [command]

Aliases:
  list, ls

Available Commands:
  providers   List installed providers for the current, named, or default scope
  workspaces  List workspace scopes
```
