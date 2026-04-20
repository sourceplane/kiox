---
title: Runtime shell
---

The **runtime** is the execution layer of kiox.

It turns workspace and provider state into a working shell environment.

Think of it as where execution actually happens.

## Responsibilities

The runtime is responsible for:

- resolving workspace context
- syncing providers
- writing lazy shims
- materializing or installing tools
- constructing the environment
- executing commands

## Runtime pipeline

```text
Workspace → Sync → Build Env → Resolve Tool Plan → Materialize/Install → Execute
```

### 1. Resolve workspace

- determine the active workspace
- load the manifest and lock

### 2. Sync providers

- ensure metadata and cached OCI content are available
- validate sources

### 3. Build environment

- create shims for aliases and provided commands
- resolve default tool paths for environment construction
- generate `PATH`
- merge environment variables
- write `.workspace/env` and `.workspace/path`

### 4. Resolve tool plan

- enter the shim for the requested command
- resolve the target tool and any `dependsOn` tools
- attach tool-scoped environments and path entries

### 5. Materialize and execute

- install missing tools via the appropriate runtime plugin
- spawn the target process

## Execution model

Execution is simple:

- commands are resolved via `PATH`
- providers behave like normal commands
- environment is preconfigured

There is no provider RPC protocol. Runtime plugins are an internal execution detail.

## Static versus dynamic PATH

The workspace path file contains the static shell path order:

1. `.workspace/bin`
2. provider and environment path entries
3. host `PATH`

When a shim resolves a tool plan, kiox can add extra tool-specific directories for that launched process. That is how lazily installed tools become executable without rewriting the workspace path file on every run.

## Commands that enter the runtime

```bash
kiox shell
kiox exec node build
kiox -- node build
```

## Environment construction

The runtime builds:

- `.workspace/bin` for aliases and provided commands
- `.workspace/env` for environment variables
- `.workspace/path` for additional `PATH` entries

It rebuilds those artifacts whenever the workspace is synced.

Runtime variables include:

- `KIOX_HOME`
- `KIOX_WORKSPACE_ROOT`
- `KIOX_WORKSPACE_HOME`
- `KIOX_WORKSPACE_ENV_FILE`
- `KIOX_WORKSPACE_PATH_FILE`
- `KIOX_PROVIDER_<ALIAS>_REF`
- `KIOX_PROVIDER_<ALIAS>_HOME`
- `KIOX_PROVIDER_<ALIAS>_BINARY`

`KIOX_PROVIDER_<ALIAS>_BINARY` points at the resolved default tool path and may not exist yet for lazily materialized tools.

## Built-in runtime plugins

The current built-in plugins are:

- `oci` for bundle-backed binaries
- `script` for tools installed by an on-demand shell script
- `local` for tools executed from an existing path, including managed-install targets

## Design properties

### Deterministic

The same inputs produce the same execution behavior.

### Lazy

Bundle-backed binaries and script-backed tool installs happen only when needed.

### Transparent

Execution behaves like normal shell commands.

### Fail-fast

Conflicts and missing dependencies fail early.

## What the runtime does not do

- define tools
- manage versions
- package artifacts

It executes. It does not define.
