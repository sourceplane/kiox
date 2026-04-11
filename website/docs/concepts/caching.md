---
title: Caching
---

tinx keeps cache, source, and runtime state separate so sync stays small and execution stays lazy.

## Storage layers

| Layer | Purpose |
| --- | --- |
| **Provider metadata** | JSON metadata under `$TINX_HOME/providers/...` |
| **OCI store** | copied OCI layouts under `$TINX_HOME/store/<storeID>/oci/` |
| **Materialized artifacts** | extracted binaries and assets under the provider store root |

## tinx home vs workspace

Two storage layers matter:

- **tinx home** is the shared cache for providers, OCI content, and metadata
- **workspace** is the project-specific runtime state

That separation enables global caching, per-project isolation, and fast reuse across workspaces.

## Lazy materialization

The install and sync paths can work from metadata before a platform binary is extracted. That is enough to:

- resolve provider versions
- write the workspace lock file
- inspect capabilities
- decide whether a workspace needs a refresh

Binaries are extracted only when the workspace shell needs them.

## Cache path

When tinx materializes a binary, it writes it under:

```text
$TINX_HOME/store/<storeID>/bin/<os>/<arch>/<entrypoint>
```

## Remote hydration

If tinx has metadata and a stored remote reference but the runtime blobs are missing, it can hydrate the local OCI store from the registry and retry extraction. That supports partial installs and resumed environments.

## Refresh model

Use refresh commands when you want tinx to look for updated provider metadata:

```bash
tinx provider update
tinx provider update node
```

For local OCI layouts, tinx reuses the layout path and does not try remote hydration.
