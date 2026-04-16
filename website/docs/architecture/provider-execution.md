---
title: Provider execution
---

Provider execution starts well before the process is launched. tinx must install metadata, locate or hydrate the OCI store, resolve the requested tool, materialize or install any missing dependencies, and then execute the command.

## Install and store

Provider metadata is stored under:

```text
$TINX_HOME/providers/<namespace>/<name>/<version>/
```

The OCI layout is stored under:

```text
$TINX_HOME/store/<storeID>/oci/
```

`storeID` is derived from the provider identity and manifest digest so different artifact revisions do not collide.

The provider store root also contains a cached `provider-metadata.json`, the provider manifest, normalized `package.json`, extracted asset content, and any lazily materialized tool binaries.

That store root is global to tinx home. A second workspace that references the same resolved provider can activate the cached metadata into its own workspace state without re-downloading the provider.

## Tool resolution and materialization

When the workspace shell needs a provider command, tinx:

1. resolves the workspace alias or provided command to a provider and tool name
2. loads the normalized package model from cached metadata
3. computes a dependency plan for the target tool
4. asks each runtime plugin whether its tool is already installed
5. extracts bundle-backed tools or runs script installs when needed
6. extracts asset bundles into the provider store root

Typical materialized paths:

```text
$TINX_HOME/store/<storeID>/<bundle-layer-source>
$TINX_HOME/store/<storeID>/tools/<tool>/bin/<command>
```

## Managed-install tools

Setup-style providers use the same path, but the target tool can delegate installation to another tool.

When a tool declares:

- `install.tool`
- `install.path`

the shim resolves the installer tool first, executes it, and passes target-specific environment variables such as `TINX_TARGET_TOOL_BIN`. That lets one bundled setup tool lazily create another executable such as `kubectl`.

## Remote hydration

Remote installs are metadata-first. tinx pulls the manifest, config, and provider metadata into the shared store, writes workspace activation metadata, and only hydrates runtime blobs when the current host needs them.

Platform-qualified binary layer media types and platform annotations let tinx pull only the current host runtime blobs while still preserving shared archive layers such as assets. If the OCI store metadata exists but a required runtime blob is missing, tinx hydrates the local store from the registry and retries extraction instead of discarding cached metadata.

## Command lookup

Once the workspace shell is built, tinx uses one of two equivalent dispatch paths:

1. generated shims in `.workspace/bin` resolve the command and enter the hidden `tinx __shim` command
2. CLI entrypoints such as `tinx -- kubectl` and `tinx exec kubectl` can dispatch directly to the prepared workspace target when the command already maps to a known alias or `provides` entry

Both paths use the same tool planning, lazy install, and environment merge logic. Direct dispatch just avoids a second shim round-trip during workspace command execution.
