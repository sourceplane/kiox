---
title: Provider examples
---

Use the example provider in this repository as the smallest working reference, then layer on assets and richer commands as needed.

## Example from this repository

`testdata/echo-provider/tinx.yaml` declares a binary provider with one documented capability:

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
    - os: linux
      arch: amd64
      binary: bin/linux/amd64/echo-provider
  capabilities:
    plan:
      description: Generate a plan
  layers:
    assets:
      root: assets
```

Build and run it locally:

```bash
make release-example
./bin/tinx init demo -p testdata/echo-provider/oci as echo
./bin/tinx --workspace demo -- echo plan
```

## Toolchain provider pattern

A provider that wraps a toolchain usually exposes one entrypoint and keeps helper tools in `spec.path`:

```yaml
spec:
  entrypoint: node
  path:
    - tools/bin
```

That pattern is useful when the provider needs bundled helpers such as wrappers, plugins, or companion binaries.

## Asset-heavy provider pattern

Providers that need templates, certificates, or policy bundles should ship them as an assets layer:

```yaml
spec:
  env:
    POLICY_ROOT: ${provider_assets}/policy
  layers:
    assets:
      root: assets
      includes:
        - policy/**
        - certs/*.pem
```

The assets are extracted into the provider store when the runtime is materialized.
