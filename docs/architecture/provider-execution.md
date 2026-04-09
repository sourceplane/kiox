---
title: Provider execution
---

Provider execution starts well before the process is launched. tinx must install metadata, locate or hydrate the OCI store, extract the current platform binary, and then resolve the alias from `PATH`.

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

## Materialization

When the workspace shell needs a provider binary, tinx:

1. computes the expected binary path
2. checks whether the binary already exists and is executable
3. extracts the binary for the current `GOOS` and `GOARCH` if needed
4. extracts assets into the provider store root

Expected binary path:

```text
$TINX_HOME/store/<storeID>/bin/<os>/<arch>/<entrypoint>
```

## Remote hydration

If the OCI store metadata exists but the runtime blobs are missing, tinx can hydrate the local store from the registry and retry extraction. This avoids throwing away cached metadata when only the platform binary is missing.

## Command lookup

Once the workspace shell is built, execution is normal process spawning:

1. search `PATH` for the requested alias
2. resolve the shim path
3. `exec` the real provider binary
4. pass the merged workspace environment to the child process

That keeps provider invocation simple. Providers receive standard command-line arguments and environment variables rather than a custom RPC protocol.
