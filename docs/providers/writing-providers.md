---
title: Writing providers
---

Write a tinx provider when you want to package one command-line tool, ship it as an OCI artifact, and expose it inside a workspace shell.

## Provider checklist

1. Create a `tinx.yaml` manifest.
2. Build one binary per supported `os` and `arch`.
3. Keep the runtime entrypoint stable.
4. Declare optional assets, environment variables, and `PATH` additions.
5. Package with `tinx pack` or `tinx release`.

## Minimal manifest

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
    - os: linux
      arch: amd64
      binary: bin/linux/amd64/node
```

`binary` is the only supported runtime type today.

## Add capability metadata

Capabilities document what the provider does. They are published in metadata and can be surfaced in tooling, but the provider binary still receives normal CLI arguments.

```yaml
spec:
  capabilities:
    build:
      description: Compile the application
    test:
      description: Run the test suite
```

## Add environment and assets

Use `spec.env` for static or templated values and `spec.path` for extra binaries or helper scripts:

```yaml
spec:
  env:
    NODE_EXTRA_CA_CERTS: ${provider_assets}/certs/root-ca.pem
    WORKSPACE_ROOT: ${workspace_root}
  path:
    - tools/bin
  layers:
    assets:
      root: assets
```

Available template values include:

- `${cwd}`
- `${workspace_root}`
- `${workspace_home}`
- `${provider_ref}`
- `${provider_home}`
- `${provider_binary}`
- `${provider_assets}`

Do not use the reserved `TINX_` prefix in `spec.env`.

## Test locally

```bash
tinx release --manifest tinx.yaml --main ./cmd/node-provider --output oci
tinx init demo -p ./oci as node
tinx --workspace demo -- node --version
```

## Validate across platforms

List every platform you publish in `spec.platforms`. `tinx release` uses that list to build or validate the provider artifact before it packs the OCI layout.

Next, read [provider packaging](./provider-packaging.md).
