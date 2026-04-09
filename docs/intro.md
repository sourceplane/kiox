---
title: tinx documentation
slug: /
---

`tinx` is a workspace-centric runtime for tools. Providers are packaged as OCI artifacts, composed into a workspace, and executed through a reproducible shell environment.

Instead of installing tools globally, tinx lets you:

- define tools declaratively
- resolve and lock versions
- execute them inside an isolated, reproducible workspace

## Core concepts

### Workspace

A **workspace** is the unit of execution. It defines which providers are available, which versions are locked, and how the runtime environment is built.

Think of it as a reproducible tool environment for a project.

Key properties:

- declarative (`tinx.yaml`)
- reproducible (`tinx.lock`)
- isolated runtime (`.workspace/`)

### Provider

A **provider** is the packaged tool. tinx distributes providers as OCI artifacts so tool versions stay portable and immutable.

Providers contain:

- platform-specific binaries
- optional assets such as templates or certificates
- environment configuration
- metadata such as capabilities and supported platforms

Think of a provider as a versioned, portable tool package.

### Alias

Inside a workspace, providers are mapped to **aliases**.

```yaml
providers:
  node:
    source: core/node
```

Here:

- `node` is the alias, or the command name you run
- `core/node` is the provider source

### Runtime

The runtime turns workspace state into an execution environment. It resolves providers, builds `PATH`, writes shims, and executes commands.

Execution always happens through a workspace:

```bash
tinx -- node --version
```

Think of it as a deterministic shell built from providers.

### tinx home vs workspace

Two storage layers matter:

| Layer | Purpose |
| --- | --- |
| **tinx home** | global cache for providers, OCI store content, and metadata |
| **workspace** | project-specific runtime state and configuration |

This separation enables global caching, per-project isolation, and reuse across workspaces.

## Mental model

```text
Provider (OCI artifact)
        ↓
Installed into tinx home
        ↓
Referenced in workspace (alias)
        ↓
Resolved + locked
        ↓
Runtime environment built
        ↓
Command executed via PATH
```

## Why tinx exists

Modern tooling problems:

- inconsistent tool versions across machines
- complex setup scripts
- global installations that conflict
- CI environments that drift from local machines

`tinx` addresses those problems by making tools declarative, using OCI as a universal distribution format, isolating execution per workspace, and making reproducibility the default.

## Design principles

### Workspace-first

Execution always happens inside a workspace.

### OCI-native

Providers are OCI artifacts, so distribution stays standard.

### Lazy materialization

Only extract binaries when a workspace needs them.

### Deterministic runtime

The same workspace should behave the same way everywhere.

### Simple execution model

No RPC. No plugin framework. Just binaries on `PATH`.

## Typical workflow

### 1. Define a workspace

```yaml
providers:
  node: core/node
  kubectl: tinx/kubectl
```

### 2. Resolve and lock

- tinx resolves versions
- tinx writes `tinx.lock`

### 3. Build runtime

- tinx creates `.workspace/`
- tinx generates `PATH` and environment files
- tinx creates shims

### 4. Execute commands

```bash
tinx -- node build
```

## What tinx is not

- Not a package manager
- Not a build system
- Not a plugin framework

It is a runtime and distribution model for tools.

## How to read the docs

Start with:

- [Workspace](./concepts/workspace.md)
- [Provider](./concepts/providers.md)
- [Runtime shell](./concepts/runtime-shell.md)

Then read:

- [CLI reference](./cli/tinx.md)
- [Examples](./examples/run-node.md)
- [Architecture internals](./architecture/internals.md)

## Summary

- **Workspace** defines the environment
- **Provider** defines the tool
- **Alias** defines the command
- **Runtime** executes everything

That separation is what makes tinx scalable for CNCF-style workflows.
