---
title: Workspace
---

A **workspace** is the unit of execution in tinx.

It defines:

- which providers are available
- which versions are locked
- how the runtime environment is constructed

Think of it as a reproducible tool environment for a project.

## Responsibilities

A workspace is responsible for:

- declaring providers
- resolving provider sources
- locking versions
- building runtime artifacts
- exposing commands through aliases

## Key files

```text
<workspace>/
  tinx.yaml      # desired state
  tinx.lock      # resolved state
  .workspace/    # runtime state
```

### `tinx.yaml` (desired state)

Declares what you want:

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

### `tinx.lock` (resolved state)

Records what was actually resolved:

- exact versions
- source references
- content identifiers

That keeps workspace execution reproducible.

### `.workspace/` (runtime state)

Generated artifacts used during execution:

- environment variables
- `PATH` configuration
- command shims

This directory is ephemeral and rebuildable.

## Workspace lifecycle

```text
Declare → Resolve → Lock → Build → Execute
```

1. Declare providers in `tinx.yaml`
2. Resolve sources from local or remote OCI artifacts
3. Lock versions in `tinx.lock`
4. Build the runtime environment
5. Execute commands through the workspace shell

## Design properties

### Declarative

The workspace describes what tools are needed, not how to install them.

### Reproducible

The lock file keeps the same environment stable across machines.

### Isolated

Execution happens in a controlled environment, not the host system.

### Composable

Multiple providers can coexist in one workspace.

## What a workspace does not do

- package tools
- define tool behavior
- execute logic itself

It orchestrates. It does not implement.

## Useful commands

```bash
tinx init
tinx init dev
tinx use dev
tinx workspace current
tinx workspace list --ready
```
