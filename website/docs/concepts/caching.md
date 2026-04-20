---
title: Caching
---

kiox keeps cache, source, and runtime state separate so sync stays small and execution stays lazy.

## Storage layers

| Layer | Purpose |
| --- | --- |
| **Workspace lock state** | resolved provider state in `kiox.lock` |
| **Provider metadata** | workspace activation metadata under `$KIOX_HOME/providers/...` plus cached store metadata under `$KIOX_HOME/store/<storeID>/provider-metadata.json` |
| **OCI store** | copied OCI layouts under `$KIOX_HOME/store/<storeID>/oci/` |
| **Materialized artifacts** | extracted binaries, installed tools, and assets under the provider store root |

## kiox home vs workspace

Two storage layers matter:

- **kiox home** is the shared cache for providers, OCI content, and metadata
- **workspace** is the project-specific runtime state

That separation enables global caching, per-project isolation, and fast reuse across workspaces.

If a second workspace resolves to the same provider digest, kiox can activate the cached shared-store metadata into that workspace and skip the registry pull entirely.

When a workspace still needs multiple uncached providers, kiox can sync their metadata installs in parallel instead of serializing the whole workspace on one slow provider at a time.

## Lazy materialization

The install and sync paths can work from metadata before a tool is materialized. That is enough to:

- resolve provider versions
- write the workspace lock file
- inspect capabilities
- build command shims and shell files

Bundle-backed tools are extracted only when a workspace command needs them. Script-backed tools are installed only when their shim executes for the first time.

## Materialized paths

Bundle-backed tools are written under their declared bundle layer source inside the provider store:

```text
$KIOX_HOME/store/<storeID>/<bundle-layer-source>
```

Script-backed tools are installed under:

```text
$KIOX_HOME/store/<storeID>/tools/<tool>/bin/<command>
```

Asset bundles are extracted into the provider store root so environment templates can reference them.

Managed-install tools that declare `install.tool` and `install.path` also end up under the tool install root. The difference is that their installer is another tool, not a shell script owned by the target tool itself.

## Tagged references and lock reuse

When a workspace references a tagged registry ref, kiox resolves that tag to a manifest digest and records the resolved digest in `kiox.lock`. Later runs can reuse the cached local store without re-checking the registry as long as the locked provider state is already available.

That is why repeated command runs can stay local and fast even when the original workspace source used a tag.

## Immutable blob reuse

The OCI store is treated as immutable content. kiox skips rewriting cached blobs when the content is unchanged and safely replaces them only when it actually differs.

That avoids unnecessary work and prevents permission issues when previously cached blob files are read-only.

## Remote hydration

Remote installs pull metadata first. kiox stores the manifest and normalized package data, then hydrates only the current host runtime blobs and shared archive layers when they are actually needed.

Platform-qualified binary layer media types let kiox skip non-host OS and architecture blobs during the pull. If kiox has metadata and a stored remote reference but the required runtime blobs are missing, it can hydrate the local OCI store from the registry and retry extraction. That supports partial installs and resumed environments.

Each registry pull also uses bounded blob-copy concurrency, and workspace sync hydrates remote runtimes with a smaller fan-out than metadata resolution. That keeps the CLI from overwhelming network bandwidth when several providers all need large runtime blobs at once.

Hydration is the exception path, not the normal repeat-run path.

## Refresh model

Use refresh commands when you want kiox to look for updated provider metadata:

```bash
kiox provider update
kiox provider update node
```

Explicit refresh and update commands bypass normal remote cache reuse so kiox re-checks the upstream provider state before updating the lock.

For local OCI layouts, kiox reuses the layout path and does not try remote hydration.

## Inventory signals

`kiox ls` and `kiox status` surface cache state indirectly through tool status:

- `ready` means the tool is already materialized or installed
- `lazy` means the provider metadata is ready but the tool has not been installed yet
- `missing` or `invalid` means kiox could not confirm the tool state
