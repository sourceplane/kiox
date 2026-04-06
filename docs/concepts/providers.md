---
title: Providers
---

A provider is a versioned artifact that tinx installs from an OCI layout or registry reference. Providers expose binaries, optional assets, environment variables, and capability metadata.

## Provider reference forms

tinx accepts several source forms:

- `namespace/name`
- `namespace/name:tag`
- `namespace/name@digest`
- `ghcr.io/org/provider:tag`
- `/path/to/local/oci-layout`

Examples:

```bash
tinx install sourceplane/echo-provider --source ./oci
tinx provider add core/node as node
tinx provider add ghcr.io/acme/kubectl:v1.31.0 as kubectl
```

## Provider manifest

```yaml
apiVersion: tinx.io/v1
kind: Provider
metadata:
  namespace: sourceplane
  name: echo-provider
  version: v0.1.0
spec:
  runtime: binary
  entrypoint: echo-provider
  platforms:
    - os: darwin
      arch: arm64
      binary: bin/darwin/arm64/echo-provider
  capabilities:
    plan:
      description: Generate a plan
```

## What tinx stores

When a provider is installed or synced, tinx stores:

- provider metadata under `$TINX_HOME/providers/<namespace>/<name>/<version>/`
- the OCI layout under `$TINX_HOME/store/<storeID>/oci/`
- source information such as layout path, tag, or pinned remote reference

## Install versus add

Use **install** when you want provider metadata in tinx home:

```bash
tinx install sourceplane/echo-provider --source ./testdata/echo-provider/oci
```

Use **provider add** when you want the provider in a workspace:

```bash
tinx provider add sourceplane/echo-provider as echo
```

`provider add` updates the workspace manifest and then syncs the workspace. That is the normal path for day-to-day execution.

## Provider environment

Providers can add environment variables and `PATH` entries through their manifest:

```yaml
spec:
  env:
    NODE_OPTIONS: "--max-old-space-size=4096"
  path:
    - tools/bin
```

tinx expands template values such as `${provider_root}`, `${provider_binary}`, and `${workspace_root}` before it writes the workspace environment.

## Capabilities

Capabilities are metadata. tinx does not turn them into subcommands. The provider binary still receives normal CLI arguments:

```bash
tinx -- echo plan
tinx -- node --version
```

Use capability names to document what the provider supports and to keep a stable contract for consumers.
