---
title: Configuration
---

tinx configuration is split across workspace manifests, provider manifests, lock files, and tinx home state.

## Workspace manifest

Workspace manifests live in the workspace root and are normally saved as `tinx.yaml`.

```yaml
apiVersion: tinx.io/v1
kind: Workspace
workspace: dev
metadata:
  name: dev
providers:
  node:
    source: core/node
  kubectl:
    source: ghcr.io/acme/kubectl:v1.31.0
    plainHTTP: false
```

### Workspace fields

| Field | Meaning |
| --- | --- |
| `apiVersion` | Must be `tinx.io/v1` |
| `kind` | Must be `Workspace` |
| `workspace` | Workspace name written to the manifest |
| `metadata.name` | Optional explicit display name |
| `providers` | Alias to provider source mapping |
| `providers.<alias>.source` | OCI registry reference or local OCI layout path |
| `providers.<alias>.plainHTTP` | Allow plain HTTP registry access for that provider |

A provider entry may be a string shorthand:

```yaml
providers:
  node: core/node
```

## Provider manifest

Provider manifests also use `tinx.yaml`, but their `kind` is `Provider`.

```yaml
apiVersion: tinx.io/v1
kind: Provider
metadata:
  namespace: acme
  name: node
  version: v20.19.0
  description: Node.js runtime provider
spec:
  runtime: binary
  entrypoint: node
  platforms:
    - os: darwin
      arch: arm64
      binary: bin/darwin/arm64/node
  capabilities:
    build:
      description: Run the build
  env:
    NODE_EXTRA_CA_CERTS: ${provider_assets}/certs/root.pem
  path:
    - tools/bin
  layers:
    assets:
      root: assets
      includes:
        - certs/*.pem
```

### Provider fields

| Field | Meaning |
| --- | --- |
| `metadata.namespace` | Provider namespace |
| `metadata.name` | Provider name |
| `metadata.version` | Provider version |
| `spec.runtime` | Must be `binary` |
| `spec.entrypoint` | Executable name tinx should run |
| `spec.platforms` | Supported platform list and binary paths |
| `spec.capabilities` | Optional capability metadata |
| `spec.env` | Environment variables to inject into the workspace |
| `spec.path` | Additional path entries relative to the provider store |
| `spec.layers.assets` | Optional assets layer configuration |

## Lock file

Each workspace sync writes `tinx.lock`:

```yaml
apiVersion: tinx.io/v1
kind: WorkspaceLock
workspace: dev
providers:
  - alias: node
    provider: core/node
    source: core/node
    version: v20.19.0
    resolved: ghcr.io/acme/node-provider@sha256:...
    store: 4f3f...
```

Use the lock file as generated state. tinx rewrites it during sync.

## Global tinx home config

tinx stores shared state in `$TINX_HOME/config.yaml`:

```yaml
aliases:
  node: core/node@v20.19.0
activeWorkspace: /abs/path/to/workspace
workspaces:
  dev: /abs/path/to/workspace
```

That file tracks aliases, the active workspace, and registered workspace roots.
