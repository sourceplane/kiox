---
title: kiox
---

`kiox` is the root command for workspace lifecycle, provider management, provider packaging, and runtime execution.

## Common patterns

```bash
kiox init demo
kiox add core/node as node
kiox sync
kiox --workspace demo status
kiox --workspace demo ls
kiox --workspace demo -- node --version
kiox release --manifest provider.yaml --push ghcr.io/acme/node-provider:v1.0.0
```

Use the top-level shortcuts when you want shorter commands:

- `kiox use` instead of `kiox workspace use`
- `kiox add`, `kiox remove`, `kiox update` for workspace providers
- `kiox sync` to reconcile manual edits to `kiox.yaml`
- `kiox ls` or `kiox list` for inventory

`kiox init`, `kiox add`, and `kiox sync` use a compact provider progress surface by default. Pass `--verbose` to `kiox init` or `kiox sync` when you want phase-by-phase provider updates instead of the condensed live view.

## Execution entrypoints

There are three normal ways to run workspace commands:

- `kiox shell` for an interactive shell
- `kiox exec <command> ...` for one command in the workspace environment
- `kiox -- <command> ...` as the shortest workspace shortcut

If you run `kiox --` with no command, kiox drops you into the workspace shell.

## Inventory commands

The current CLI distinguishes between provider-only and provider-plus-tool inventory:

- `kiox provider list` shows providers
- `kiox ls` shows providers and tools for a workspace or the default scope
- `kiox status` shows the current workspace plus provider, tool, shim, and environment details

## Global flags

- `--kiox-home`: override the kiox home directory
- `--workspace`, `-w`: select a workspace for workspace-shell commands
- `--version`: print the kiox version

For commands that disable normal flag parsing, prefer `kiox help <command>` instead of `kiox <command> --help`.

## Help output

```text
OCI-native provider runtime and packager

Usage:
  kiox [flags]
  kiox [command]

Available Commands:
  add         Add a provider to the current or selected workspace
  completion  Generate the autocompletion script for the specified shell
  exec        Run a command inside the workspace environment
  help        Help about any command
  init        Create or materialize a provider workspace
  install     Install provider metadata from an OCI layout or registry reference
  list        List providers and tools or inspect workspace inventory
  pack        Package a provider into an OCI image layout
  provider    Manage workspace providers and provider inventory
  release     Build, package, and optionally push a provider artifact
  remove      Remove a provider from the current or selected workspace
  shell       Launch an interactive workspace shell
  status      Show the current workspace, providers, tools, shims, and environment
  sync        Reconcile workspace state from kiox.yaml
  update      Refresh provider metadata for the current or selected workspace
  use         Select a workspace and optionally run a command inside its shell
  version     Print the kiox version
  workspace   Manage workspaces

Flags:
  -h, --help               help for kiox
      --kiox-home string   override the kiox home directory
  -v, --version            version for kiox
  -w, --workspace string   select the workspace for workspace-shell commands

Use "kiox [command] --help" for more information about a command.
```

## Related commands

- [`kiox install`](./kiox-install.md)
- [`kiox workspace`](./kiox-workspace.md)
- [`kiox provider`](./kiox-provider.md)
- [`kiox run`](./kiox-run.md)
