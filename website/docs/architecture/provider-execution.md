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

The provider store root also contains the cached provider manifest, normalized `package.json`, extracted asset content, and any lazily materialized tool binaries.

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

If the OCI store metadata exists but the runtime blobs are missing, tinx can hydrate the local store from the registry and retry extraction. This avoids throwing away cached metadata when only the platform binary is missing.

## Command lookup

Once the workspace shell is built, execution is normal process spawning:

1. search `PATH` for the requested alias
2. resolve the shim path
3. enter the hidden `tinx __shim` command
4. resolve and install the tool plan if needed
5. pass the merged workspace environment to the child process

That keeps provider invocation simple. Tools receive standard command-line arguments and environment variables rather than a custom RPC protocol.
