---
title: Caching
---

tinx keeps cache, source, and runtime state separate so sync stays small and execution stays lazy.

## Storage layers

| Layer | Purpose |
| --- | --- |
| **Workspace lock state** | resolved provider state in `tinx.lock` |
| **Provider metadata** | workspace activation metadata under `$TINX_HOME/providers/...` plus cached store metadata under `$TINX_HOME/store/<storeID>/provider-metadata.json` |
| **OCI store** | copied OCI layouts under `$TINX_HOME/store/<storeID>/oci/` |
| **Materialized artifacts** | extracted binaries, installed tools, and assets under the provider store root |

## tinx home vs workspace

Two storage layers matter:

- **tinx home** is the shared cache for providers, OCI content, and metadata
- **workspace** is the project-specific runtime state

That separation enables global caching, per-project isolation, and fast reuse across workspaces.

If a second workspace resolves to the same provider digest, tinx can activate the cached shared-store metadata into that workspace and skip the registry pull entirely.

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
$TINX_HOME/store/<storeID>/<bundle-layer-source>
```

Script-backed tools are installed under:

```text
$TINX_HOME/store/<storeID>/tools/<tool>/bin/<command>
```

Asset bundles are extracted into the provider store root so environment templates can reference them.

Managed-install tools that declare `install.tool` and `install.path` also end up under the tool install root. The difference is that their installer is another tool, not a shell script owned by the target tool itself.

## Tagged references and lock reuse

When a workspace references a tagged registry ref, tinx resolves that tag to a manifest digest and records the resolved digest in `tinx.lock`. Later runs can reuse the cached local store without re-checking the registry as long as the locked provider state is already available.

That is why repeated command runs can stay local and fast even when the original workspace source used a tag.

## Immutable blob reuse

The OCI store is treated as immutable content. tinx skips rewriting cached blobs when the content is unchanged and safely replaces them only when it actually differs.

That avoids unnecessary work and prevents permission issues when previously cached blob files are read-only.

## Remote hydration

Remote installs pull metadata first. tinx stores the manifest and normalized package data, then hydrates only the current host runtime blobs and shared archive layers when they are actually needed.

Platform-qualified binary layer media types let tinx skip non-host OS and architecture blobs during the pull. If tinx has metadata and a stored remote reference but the required runtime blobs are missing, it can hydrate the local OCI store from the registry and retry extraction. That supports partial installs and resumed environments.

Hydration is the exception path, not the normal repeat-run path.

## Refresh model

Use refresh commands when you want tinx to look for updated provider metadata:

```bash
tinx provider update
tinx provider update node
```

Explicit refresh and update commands bypass normal remote cache reuse so tinx re-checks the upstream provider state before updating the lock.

For local OCI layouts, tinx reuses the layout path and does not try remote hydration.

## Inventory signals

`tinx ls` and `tinx status` surface cache state indirectly through tool status:

- `ready` means the tool is already materialized or installed
- `lazy` means the provider metadata is ready but the tool has not been installed yet
- `missing` or `invalid` means tinx could not confirm the tool state
