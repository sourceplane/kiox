---
title: Workspace
---

A workspace is the main unit of execution in tinx. It defines which providers are available, which versions are locked, and which runtime artifacts tinx should write before running commands.

## Mental model

- `tinx.yaml` declares providers by alias.
- `tinx.lock` records resolved versions and source information.
- `.workspace/` holds generated runtime state such as environment files, path files, and shims.
- `tinx use`, `tinx workspace use`, or `--workspace` select which workspace tinx should run.

## Workspace manifest

```yaml
apiVersion: tinx.io/v1
kind: Workspace
workspace: dev

providers:
  node:
    source: core/node
  lite-ci:
    source: sourceplane/lite-ci
```

You can also start from a file such as `providers.tx.yaml` or `providers.tinx.yaml`. tinx normalizes those inputs to `tinx.yaml`.

## Workspace discovery and selection

tinx resolves the workspace in this order:

1. `--workspace <name-or-path>`
2. A workspace discovered by walking upward from the current directory
3. The active workspace stored in the tinx home directory

Useful commands:

```bash
tinx init
tinx init dev
tinx use dev
tinx workspace current
tinx workspace list --ready
```

## Files tinx writes

```text
<workspace>/
  tinx.yaml
  tinx.lock
  .workspace/
    env
    path
    bin/
    providers/
```

- `.workspace/env` contains exported variables for the workspace shell.
- `.workspace/path` contains newline-separated `PATH` entries.
- `.workspace/bin/<alias>` is a shim that dispatches to the provider binary.

## Adding providers

Use provider aliases that match the command name you want in the shell:

```bash
tinx provider add core/node as node
tinx provider add sourceplane/lite-ci as lite-ci
```

The workspace lock records the resolved version and source, so the same workspace can be re-synced later without guessing which provider build was used.

## Running commands

Run commands through the workspace shell, not with `tinx install`:

```bash
tinx -- node build
tinx exec node test
tinx shell
```

## Deleting workspaces

Delete the runtime state and unregister the workspace when you no longer need it:

```bash
tinx workspace delete dev
```

If the workspace root has already been removed, `workspace delete` also cleans up the stale registry entry from tinx home.
