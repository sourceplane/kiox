---
title: Caching
---

tinx separates provider metadata, OCI storage, and extracted binaries. That split keeps sync operations small and lets runtime extraction happen only when needed.

## Cache layers

1. **Provider metadata**: JSON metadata stored under `$TINX_HOME/providers/...`
2. **OCI store**: copied OCI layouts stored under `$TINX_HOME/store/<storeID>/oci/`
3. **Materialized binaries and assets**: extracted under the provider store root

## Metadata-first workflow

The install and sync logic can work from metadata before a platform binary is extracted. That is enough to:

- resolve provider versions
- build the workspace lock file
- inspect capabilities
- decide whether a workspace needs a refresh

## Lazy binary extraction

Binaries are extracted when the workspace shell needs them:

```bash
tinx shell
tinx exec node --version
```

If the current platform binary is missing, tinx materializes it from the cached OCI layout into:

```text
$TINX_HOME/store/<storeID>/bin/<os>/<arch>/<entrypoint>
```

## Remote hydration

If tinx has metadata and a stored remote reference but the runtime blobs are missing, tinx can hydrate the local OCI store from the registry and retry the extraction. This supports partial installs and resumed environments.

## Lock files and pinned sources

`tinx.lock` stores the resolved version and source reference that tinx used during sync. If a provider source is already pinned with a tag or digest, tinx keeps using that source directly.

## When to refresh

Use refresh commands when you want to force tinx to look for updated provider metadata:

```bash
tinx provider update
tinx provider update node
```

For local OCI layouts, tinx reuses the layout path and does not try remote hydration.
