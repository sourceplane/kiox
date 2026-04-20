---
title: Internals
---

This page maps the major kiox subsystems to the responsibilities they own. Use it when you need to understand how the workspace, provider, and runtime layers fit together.

## System view

```text
Provider (OCI)
      ↓
kiox home (cache)
      ↓
Workspace (definition + lock)
      ↓
Runtime (execution)
      ↓
Command
```

## Subsystems

| Package | Responsibility |
| --- | --- |
| `internal/cmd` | Cobra commands, workspace targeting, CLI behavior |
| `internal/core` | Normalized provider package model and tool dependency resolution |
| `internal/parser` | Manifest loading, legacy normalization, and multi-document parsing |
| `internal/workspace` | Workspace manifests, lock files, sync, shell artifact generation |
| `internal/oci` | OCI packing, remote install, local layout reads, runtime materialization |
| `internal/state` | kiox home layout, aliases, active workspace, provider metadata |
| `internal/runtime` | Environment assembly, PATH handling, process execution helpers |
| `internal/runtimes` | Built-in runtime plugins for `oci`, `script`, and `local` tools |
| `internal/build` | Go and GoReleaser build pipelines for providers |

## Workspace pipeline

When you run a workspace command such as:

```bash
kiox --workspace demo -- node build
```

kiox follows this path:

1. `internal/cmd` resolves the selected workspace by flag, discovery, or active workspace record.
2. `internal/workspace` loads and normalizes the workspace manifest.
3. `internal/workspace` syncs provider sources and updates `kiox.lock`.
4. `internal/workspace` builds `.workspace/env`, `.workspace/path`, and lazy shims under `.workspace/bin/`.
5. The selected shim re-enters `internal/cmd` through the hidden `__shim` command.
6. `internal/core` resolves the tool dependency plan.
7. `internal/runtimes` installs missing tools and `internal/runtime` executes the target process.

The important architectural shift is that execution is now planned per tool, not per provider binary.

## Packaging pipeline

When you package a provider:

```bash
kiox release --manifest provider.yaml --main ./cmd/my-provider --push ghcr.io/acme/my-provider:v1.2.3
```

kiox follows this path:

1. `internal/parser` normalizes the provider manifest into a package model.
2. `internal/build` infers build targets from normalized bundle layer sources.
3. `internal/build` compiles the required bundle-backed binaries.
4. `internal/oci` stages bundle sources, writes config, manifest, normalized package metadata, and bundle layers into an OCI layout.
5. `internal/oci` optionally pushes the layout to a registry through ORAS.

## Storage model

Two storage roots matter:

- **kiox home**: default `~/.kiox`, or `KIOX_HOME`, or `--kiox-home`
- **workspace root**: the directory that contains `kiox.yaml`

The workspace is for project-local runtime state. kiox home is for reusable provider metadata, OCI store content, aliases, and active workspace tracking.

## Design themes

- **Workspace first**: execution always goes through a workspace shell.
- **OCI everywhere**: providers are packaged and distributed as OCI artifacts.
- **Lazy materialization**: metadata can exist before tools or assets are materialized.
- **Normalized packages**: legacy manifests and new resource-based manifests share one internal model.
- **Plugin-driven execution**: runtime behavior lives in built-in plugins behind a shared interface.
- **Explicit failures**: missing workspaces, missing commands, and environment conflicts fail early.

## Key takeaway

- workspace logic decides *what* should run
- provider packaging decides *what* gets distributed
- runtime decides *how* commands execute
