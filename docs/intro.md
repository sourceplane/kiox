---
title: tinx documentation
slug: /
---

`tinx` turns OCI-packaged providers into normal commands inside a workspace shell. You define providers in a workspace manifest, tinx resolves and caches them, and the runtime exposes each provider alias on `PATH` through generated shims.

## What to expect

- **Workspace-first workflow**: keep provider selection, lock state, and runtime artifacts next to your project.
- **OCI-native packaging**: package providers as OCI layouts, install from local layouts or registries, and push the same artifact to a registry.
- **Runtime shell model**: run `tinx shell`, `tinx exec`, or `tinx -- ...` to execute commands with provider binaries and environment wired in.
- **Lazy materialization**: metadata can be installed before platform binaries, and binaries are extracted only when a workspace actually needs them.

## Quick path

```bash
go install github.com/sourceplane/tinx/cmd/tinx@latest

tinx init demo
tinx provider add core/node as node
tinx provider add sourceplane/lite-ci as lite-ci

tinx status
tinx -- node --version
tinx -- lite-ci plan
```

## Read this next

- [Install tinx](./getting-started/installation.md)
- [Run the repository quick start](./getting-started/quick-start.md)
- [Understand the workspace model](./concepts/workspace.md)
- [Understand providers](./concepts/providers.md)
- [Browse the CLI reference](./cli/tinx.md)
- [Review internal architecture](./architecture/internals.md)

## Documentation map

| Area | Use it for |
| --- | --- |
| [Getting Started](./getting-started/installation.md) | Install tinx and run the repository example end to end |
| [Concepts](./concepts/workspace.md) | Learn the workspace, provider, caching, and shell mental model |
| [CLI](./cli/tinx.md) | Find command syntax, flags, and migration guidance |
| [Providers](./providers/writing-providers.md) | Build, package, and publish providers |
| [Examples](./examples/run-node.md) | Copy common workspace patterns into your own projects |
| [Architecture](./architecture/internals.md) | Understand sync, runtime materialization, and OCI handling |
| [Reference](./reference/configuration.md) | Check manifest fields and environment variables |
| [Contributing](./contributing/contributing.md) | Work on tinx or improve the docs site |
