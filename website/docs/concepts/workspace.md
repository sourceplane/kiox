---
title: Workspace
---

A **workspace** is the execution boundary in kiox.

It decides which provider packages are available, which versions are locked, and which shell artifacts should be written for the current project.

Think of it as a reproducible tool environment for one codebase or automation context.

## What a workspace owns

A workspace is responsible for:

- declaring provider aliases and sources
- resolving those sources to installed provider metadata
- locking versions and digests in `kiox.lock`
- writing `.workspace/` shell artifacts
- exposing provider aliases and tool commands on `PATH`

## Key files

```text
<workspace>/
  kiox.yaml      # desired state
  kiox.lock      # resolved state
  .workspace/    # runtime state
```

| File | Owner | Purpose |
| --- | --- | --- |
| `kiox.yaml` | User | Desired workspace name and provider sources |
| `kiox.lock` | kiox | Resolved provider versions and digests |
| `.workspace/` | kiox | Rebuildable shell/runtime artifacts |

### `kiox.yaml`

Declares which provider packages belong to the workspace:

```yaml
apiVersion: kiox.io/v1
kind: Workspace
metadata:
  name: dev

providers:
  node:
    source: core/node
  kubectl:
    source: ghcr.io/acme/setup-kubectl:v1.29.0
```

### `kiox.lock`

Records the resolved provider state, including pinned digests for tagged registry references. That keeps workspace execution reproducible and lets repeated runs reuse cached content.

### `.workspace/`

Generated artifacts used during execution:

- `bin/` with shims for aliases and provided commands
- `env` with exported workspace environment variables
- `path` with the static path additions used by the workspace shell

This directory is rebuildable. kiox recreates it whenever the workspace is synced.

## Workspace lifecycle

```text
Edit Desired State → Reconcile → Lock → Build Shell Artifacts → Execute
```

1. Declare provider aliases in `kiox.yaml` or add them with `kiox add`.
2. Reconcile provider sources with `kiox init`, `kiox sync`, or any normal workspace execution entry point.
3. Persist the resolved provider state in `kiox.lock`.
4. Build `.workspace/bin`, `.workspace/env`, and `.workspace/path`.
5. Run commands through `kiox exec`, `kiox shell`, or `kiox -- ...`.

The actual tool binaries may still be lazy at this point. The first command run through a shim performs the final tool installation steps if needed.

`kiox add` follows the same model: kiox resolves, installs, and validates the provider first, then updates `kiox.yaml` and `kiox.lock` only after that succeeds.

## Inventory and status

The workspace surface is not only providers anymore. `kiox ls` and `kiox status` now show:

- providers installed for the workspace
- tool inventory for each ready provider
- whether tools are `ready`, `lazy`, `missing`, or `invalid`

That makes setup-style providers visible before and after the first tool run.

## Design properties

### Declarative

The workspace says which provider packages should exist, not how each tool is installed.

### Reproducible

The lock file stabilizes provider resolution across machines and CI jobs.

### Isolated

Commands execute through a workspace-specific shim and environment instead of relying on global host setup.

### Composable

Multiple providers can expose commands into the same workspace shell.

## What a workspace does not do

- package provider artifacts
- define tool behavior inside a provider
- bypass the lazy shim path

The workspace composes provider packages. The runtime resolves and executes their tools.

## Useful commands

```bash
kiox init
kiox add core/node as node
kiox sync
kiox ls
kiox status
kiox -- node --version
kiox workspace list --ready
```
