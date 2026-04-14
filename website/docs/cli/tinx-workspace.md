---
title: tinx workspace
---

`tinx workspace` manages workspace selection, registration, and discovery. The top-level `tinx use` shortcut shares the same execution path as `tinx workspace use`.

## Typical workflow

```bash
tinx init demo
tinx add core/node as node
tinx workspace list
tinx workspace current
tinx workspace use demo -- node --version
tinx workspace delete demo
```

You can target a workspace by name or by path:

```bash
tinx workspace use demo
tinx workspace use ./demo
tinx --workspace demo -- node build
```

`tinx --`, `tinx exec`, and `tinx shell` all resolve the target workspace the same way: explicit flag first, discovery second, active workspace record third.

## Main command help

```text
Manage workspaces

Usage:
  tinx workspace [command]

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
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands

Use "tinx workspace [command] --help" for more information about a command.
```

## `workspace use`

Select a workspace, store it as the active workspace, and optionally run a command in one step.

```text
Select a workspace and optionally run a command inside its shell

Usage:
  tinx workspace use <workspace> [-- command...] [flags]

Flags:
  -h, --help   help for use

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx workspace use demo
tinx workspace use demo -- node build
tinx use demo -- kubectl version --client
```

## `workspace list`

List known workspaces with filters for active, ready, and missing entries.

```text
List known workspaces

Usage:
  tinx workspace list [flags]

Flags:
      --active    show only the active workspace
  -h, --help      help for list
      --missing   show only missing workspaces
      --ready     show only ready workspaces
  -s, --short     show only workspace names

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

Examples:

```bash
tinx workspace list
tinx workspace list --short
tinx workspace list --ready
tinx workspace list --missing
```

## `workspace current`

Show the current active workspace or the workspace discovered from the current directory.

```text
Show the current workspace

Usage:
  tinx workspace current [flags]

Flags:
  -h, --help   help for current

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

## `workspace delete`

Delete runtime state and unregister the workspace from tinx home.

```text
Delete workspace runtime state and unregister it

Usage:
  tinx workspace delete <workspace> [flags]

Flags:
  -h, --help   help for delete

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

If the registered workspace path no longer exists, delete still removes the stale record from tinx home.
