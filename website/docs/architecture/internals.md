---
title: Internals
---

This page maps the major tinx subsystems to the responsibilities they own. Use it when you need to understand how the workspace, provider, and runtime layers fit together.

## System view

```text
Provider (OCI)
      ↓
tinx home (cache)
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
| `internal/workspace` | Workspace manifests, lock files, sync, shell artifact generation |
| `internal/manifest` | Provider manifest schema and validation |
| `internal/oci` | OCI packing, remote install, local layout reads, runtime materialization |
| `internal/state` | tinx home layout, aliases, active workspace, provider metadata |
| `internal/runtime` | Environment assembly, PATH handling, command execution |
| `internal/build` | Go and GoReleaser build pipelines for providers |

## Workspace pipeline

When you run a workspace command such as:

```bash
tinx --workspace demo -- node build
```

tinx follows this path:

1. `internal/cmd` resolves the selected workspace by flag, discovery, or active workspace record.
2. `internal/workspace` loads and normalizes the workspace manifest.
3. `internal/workspace` syncs provider sources and updates `tinx.lock`.
4. `internal/workspace` builds `.workspace/env`, `.workspace/path`, and `.workspace/bin/<alias>`.
5. `internal/runtime` resolves the command from the generated `PATH` and runs it.

## Packaging pipeline

When you package a provider:

```bash
tinx release --manifest tinx.yaml --main ./cmd/my-provider --push ghcr.io/acme/my-provider:v1.2.3
```

tinx follows this path:

1. `internal/build` compiles the binaries listed in `spec.platforms`.
2. `internal/oci` writes config, manifest, metadata, assets, and binary layers into an OCI layout.
3. `internal/oci` optionally pushes the layout to a registry through ORAS.

## Storage model

Two storage roots matter:

- **tinx home**: default `~/.tinx`, or `TINX_HOME`, or `--tinx-home`
- **workspace root**: the directory that contains `tinx.yaml`

The workspace is for project-local runtime state. tinx home is for reusable provider metadata, OCI store content, aliases, and active workspace tracking.

## Design themes

- **Workspace first**: execution always goes through a workspace shell.
- **OCI everywhere**: providers are packaged and distributed as OCI artifacts.
- **Lazy materialization**: metadata can exist before runtime binaries are extracted.
- **Explicit failures**: missing workspaces, missing commands, and environment conflicts fail early.

## Key takeaway

- workspace logic decides *what* should run
- provider packaging decides *what* gets distributed
- runtime decides *how* commands execute
