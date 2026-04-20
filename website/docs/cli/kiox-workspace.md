---
title: kiox workspace
---

`kiox workspace` manages workspace selection, registration, and discovery. The top-level `kiox use` shortcut shares the same execution path as `kiox workspace use`.

## Typical workflow

```bash
kiox init demo
kiox add core/node as node
kiox workspace list
kiox workspace current
kiox workspace use demo -- node --version
kiox workspace delete demo
```

You can target a workspace by name or by path:

```bash
kiox workspace use demo
kiox workspace use ./demo
kiox --workspace demo -- node build
```

`kiox --`, `kiox exec`, and `kiox shell` all resolve the target workspace the same way: explicit flag first, discovery second, active workspace record third.

## Main command help

```text
Manage workspaces

Usage:
  kiox workspace [command]

Aliases:
  workspace, ws, workspaces

Available Commands:
  create      Create or materialize a workspace
  current     Show the current workspace
  delete      Delete workspace runtime state and unregister it
  list        List known workspaces
  use         Select a workspace and optionally run a command inside its shell

Flags:
  -h, --help   help for workspace

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands

Use "kiox workspace [command] --help" for more information about a command.
```

## `workspace use`

Select a workspace, store it as the active workspace, and optionally run a command in one step.

```text
Select a workspace and optionally run a command inside its shell

Usage:
  kiox workspace use <workspace> [-- command...] [flags]

Flags:
  -h, --help   help for use

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox workspace use demo
kiox workspace use demo -- node build
kiox use demo -- kubectl version --client
```

## `workspace list`

List known workspaces with filters for active, ready, and missing entries.

```text
List known workspaces

Usage:
  kiox workspace list [flags]

Flags:
      --active    show only the active workspace
  -h, --help      help for list
      --missing   show only missing workspaces
      --ready     show only ready workspaces
  -s, --short     show only workspace names

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
kiox workspace list
kiox workspace list --short
kiox workspace list --ready
kiox workspace list --missing
```

## `workspace current`

Show the current active workspace or the workspace discovered from the current directory.

```text
Show the current workspace

Usage:
  kiox workspace current [flags]

Flags:
  -h, --help   help for current

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

## `workspace delete`

Delete runtime state and unregister the workspace from kiox home.

```text
Delete workspace runtime state and unregister it

Usage:
  kiox workspace delete <workspace> [flags]

Flags:
  -h, --help   help for delete

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

If the registered workspace path no longer exists, delete still removes the stale record from kiox home.
