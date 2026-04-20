---
title: kiox documentation
slug: /
---

`kiox` is a workspace-first runtime for OCI-distributed provider packages. Instead of installing tools globally, you declare provider packages in a workspace, let kiox lock and cache them, and execute commands through lazy workspace shims.

Use kiox when you want to:

- pin tool packages to a project workspace
- expose multiple commands from one provider package
- lazily materialize OCI-backed binaries and setup-installed tools
- reuse one global provider cache across many workspaces

## Core concepts

### Workspace

A **workspace** is the execution boundary. `kiox.yaml` declares the desired provider aliases and sources, `kiox.lock` records the resolved versions and digests, and `.workspace/` holds rebuildable shell artifacts.

### Provider package

A **provider package** is the distribution unit. Canonically, a provider is a normalized package of resources stored as an OCI artifact.

The active resource kinds are:

- **Tool**: a command surface with a runtime and optional dependencies
- **Bundle**: OCI-backed binaries or tarred asset payloads
- **Asset**: a mounted view of a bundle inside the provider store
- **Environment**: exported variables and path additions

Legacy manifests that declare `runtime: binary`, `entrypoint`, and `platforms` are still accepted, but kiox normalizes them into the same internal package model.

### Tool

A **tool** is the executable surface kiox resolves at runtime. One tool is usually marked as the provider default, while additional tools contribute command names through `provides`.

Tools can depend on other tools inside the same provider. That is how setup-style providers work: a bundled installer tool can materialize a second tool only when its shim is used for the first time.

### Alias and provided commands

A **workspace alias** points at the provider default tool. kiox also writes shims for every command in `provides`, so a single provider can expose several workspace commands.

### Runtime plugin

The runtime is internally split into built-in plugins:

- `oci`: materialize a tool from bundle layers in the provider artifact
- `script`: run an install script that creates the tool binary on demand
- `local`: execute an existing path, including a path created by another tool

Tool execution is still normal process spawning. The plugins only decide how a tool is resolved and installed.

### kiox home vs workspace

Two storage layers matter:

| Layer | Purpose |
| --- | --- |
| **kiox home** | global cache for providers, OCI store content, and metadata |
| **workspace** | project-specific runtime state and configuration |

This separation enables global caching, per-project isolation, and reuse across workspaces.

## Mental model

```text
Provider package (OCI artifact)
        ↓
Installed into kiox home
        ↓
Referenced in a workspace alias
        ↓
Workspace sync writes .workspace/bin shims
        ↓
Command enters hidden kiox __shim
        ↓
Tool plan is resolved and missing tools are installed
        ↓
Target process executes with the merged environment
```

That is why a workspace can feel instant after the first install. Metadata and OCI content are cached globally, while actual tool binaries are only materialized when a command needs them.

## Why kiox exists

Modern tooling problems usually look like this:

- inconsistent tool versions across machines
- setup scripts that drift over time
- global installs that conflict with each other
- CI environments that behave differently from local shells

`kiox` addresses those problems by making provider packages declarative, using OCI as a universal distribution format, isolating execution per workspace, and making lazy reproducibility the default.

## Design principles

### Workspace-first

Execution always happens inside a workspace.

### OCI-native

Provider packages are stored and transported as OCI artifacts.

### Normalized package model

Legacy shorthands and modern multi-resource manifests end up in one internal package representation.

### Lazy materialization

Only materialize bundled binaries or install setup-managed tools when a command actually needs them.

### Pluggable runtimes

Tool execution stays normal process execution, but resolution and install behavior comes from built-in runtime plugins such as `oci`, `script`, and `local`.

### Deterministic runtime

The same workspace should behave the same way everywhere.

## Typical workflow

Check in a workspace manifest:

```yaml
apiVersion: kiox.io/v1
kind: Workspace
metadata:
  name: demo
providers:
  node:
    source: core/node
```

Initialize it and run commands:

```bash
kiox init
kiox sync   # after manual edits to kiox.yaml
kiox -- node --version
kiox status
kiox ls
```

The workspace reconcile step prepares `.workspace/`, writes shims for aliases and provided commands, and records the resolved provider versions in `kiox.lock`. The first command run then materializes only the tools it needs.

## What changed in the current architecture

The current runtime is not a single provider binary launcher anymore. It is a normalized package runtime with:

- explicit `Tool`, `Bundle`, `Asset`, and `Environment` resources
- lazy shims that re-enter kiox through `__shim`
- a tool dependency planner for setup-style providers
- managed-install tools declared with `install.tool` and `install.path`
- tool inventory surfaced by `kiox ls` and `kiox status`

## What kiox is not

- Not a language-specific package manager
- Not a build system
- Not a container orchestrator

It is a runtime layer for tool packages and workspace execution.

## How to read the docs

Start with:

- [Workspace](./concepts/workspace.md)
- [Providers](./concepts/providers.md)
- [Runtime shell](./concepts/runtime-shell.md)

Then read:

- [CLI reference](./cli/kiox.md)
- [Examples](./examples/run-node.md)
- [Architecture internals](./architecture/internals.md)

## Summary

- **Workspace** defines aliases, locking, and runtime state.
- **Provider package** defines tools and the resources they need.
- **Tool** defines what command kiox resolves and how it is installed.
- **Runtime plugins** materialize or install tools and then execute them.
