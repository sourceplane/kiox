---
title: Runtime shell
---

The **runtime** is the execution layer of tinx.

It turns workspace and provider state into a working shell environment.

Think of it as where execution actually happens.

## Responsibilities

The runtime is responsible for:

- resolving workspace context
- syncing providers
- materializing binaries
- constructing the environment
- executing commands

## Runtime pipeline

```text
Workspace → Sync → Materialize → Build Env → Execute
```

### 1. Resolve workspace

- determine the active workspace
- load the manifest and lock

### 2. Sync providers

- ensure metadata is available
- validate sources

### 3. Materialize

- extract platform-specific binaries
- extract assets when needed

### 4. Build environment

- generate `PATH`
- merge environment variables
- create command shims

### 5. Execute

- resolve the command from `PATH`
- spawn the process

## Execution model

Execution is simple:

- commands are resolved via `PATH`
- providers behave like normal binaries
- environment is preconfigured

No RPC. No plugins. No extra protocol layer.

## Commands that enter the runtime

```bash
tinx shell
tinx exec node build
tinx -- node build
```

## Environment construction

The runtime builds:

- `.workspace/bin` for command entrypoints
- `.workspace/env` for environment variables
- `.workspace/path` for additional `PATH` entries

Runtime variables include:

- `TINX_HOME`
- `TINX_WORKSPACE_ROOT`
- `TINX_WORKSPACE_HOME`
- `TINX_WORKSPACE_ENV_FILE`
- `TINX_WORKSPACE_PATH_FILE`
- `TINX_PROVIDER_<ALIAS>_REF`
- `TINX_PROVIDER_<ALIAS>_HOME`
- `TINX_PROVIDER_<ALIAS>_BINARY`

## Design properties

### Deterministic

The same inputs produce the same execution behavior.

### Lazy

Binaries are extracted only when needed.

### Transparent

Execution behaves like normal shell commands.

### Fail-fast

Conflicts and missing dependencies fail early.

## What the runtime does not do

- define tools
- manage versions
- package artifacts

It executes. It does not define.
