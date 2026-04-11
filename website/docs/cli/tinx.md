---
title: tinx
---

`tinx` is the root command for workspace lifecycle, provider management, provider packaging, and runtime execution.

## Common patterns

```bash
tinx init demo
tinx provider add core/node as node
tinx --workspace demo status
tinx --workspace demo -- node --version
tinx release --manifest tinx.yaml --push ghcr.io/acme/node-provider:v1.0.0
```

Use the top-level shortcuts when you want shorter commands:

- `tinx use` instead of `tinx workspace use`
- `tinx add`, `tinx remove`, `tinx update` for workspace providers
- `tinx list` for provider and workspace inventory

## Global flags

- `--tinx-home`: override the tinx home directory
- `--workspace`, `-w`: select a workspace for workspace-shell commands
- `--version`: print the tinx version

For commands that disable normal flag parsing, prefer `tinx help <command>` instead of `tinx <command> --help`.

## Help output

```text
OCI-native provider runtime and packager

Usage:
  tinx [flags]
  tinx [command]

Available Commands:
  add         Add a provider to the current or selected workspace
  completion  Generate the autocompletion script for the specified shell
  exec        Run a command inside the workspace environment
  help        Help about any command
  init        Create or materialize a provider workspace
  install     Install provider metadata from an OCI layout or registry reference
  list        List providers or inspect workspace inventory
  pack        Package a provider into an OCI image layout
  provider    Manage workspace providers and provider inventory
  release     Build, package, and optionally push a provider artifact
  remove      Remove a provider from the current or selected workspace
  shell       Launch an interactive workspace shell
  status      Show the current workspace, providers, shims, and environment
  update      Refresh provider metadata for the current or selected workspace
  use         Select a workspace and optionally run a command inside its shell
  version     Print the tinx version
  workspace   Manage workspaces

Flags:
  -h, --help               help for tinx
      --tinx-home string   override the tinx home directory
  -v, --version            version for tinx
  -w, --workspace string   select the workspace for workspace-shell commands

Use "tinx [command] --help" for more information about a command.
```

## Related commands

- [`tinx install`](./tinx-install.md)
- [`tinx workspace`](./tinx-workspace.md)
- [`tinx provider`](./tinx-provider.md)
- [`tinx run`](./tinx-run.md)
