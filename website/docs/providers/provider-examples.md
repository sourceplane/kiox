---
title: Provider examples
---

Use the provider fixtures in this repository as working references for both the legacy and normalized manifest styles.

## Fixtures in this repository

- `testdata/echo-provider`: legacy single-binary shorthand with `runtime: binary`
- `testdata/multi-tool-provider`: multi-document normalized package with an OCI setup tool and a lazy script tool
- `testdata/inline-tool-provider`: inline normalized package with bundled assets and synthesized provider contents
- `testdata/setup-kubectl`: managed-install provider where a bundled setup tool lazily creates the default `kubectl` tool

Manual commands for all fixtures live in `TEST_PROVIDERS.md`.

## Legacy compatibility fixture

`testdata/echo-provider/kiox.yaml` keeps the old single-tool authoring model available:

```yaml
apiVersion: kiox.io/v1
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

Use it when you want the shortest possible provider manifest or need to validate backward compatibility.

## Multi-document normalized fixture

`testdata/multi-tool-provider/kiox.yaml` shows the current package model split across several YAML documents:

- `setup-echo` is an `oci` tool backed by bundle layers
- `echo-tool` is a `script` tool that depends on `setup-echo`
- `default-env` exports variables into the workspace shell
- the workspace exposes both the provider alias and the tool's `provides` command

## Inline normalized fixture

`testdata/inline-tool-provider/kiox.yaml` keeps everything in one `Provider` document:

- inline `tools`, `bundles`, `assets`, and `environments`
- synthesized `spec.contents`
- an asset tar bundle mounted into the provider store
- a lazy script tool that reads from the mounted asset path

## Managed-install fixture

`testdata/setup-kubectl/kiox.yaml` demonstrates the new setup-provider flow:

- `setup-kubectl` is a bundle-backed installer tool
- `kubectl` is a `local` tool with `install.tool` and `install.path`
- the shim manager executes `setup-kubectl` only when `kubectl` is first requested

```yaml
tools:
  - name: setup-kubectl
    runtime: oci
    from: bundle.setup-kubectl
    provides:
      - setup-kubectl
  - name: kubectl
    default: true
    runtime: local
    install:
      tool: setup-kubectl
      path: bin/kubectl
    dependsOn:
      - tool: setup-kubectl
    provides:
      - kubectl
```

## Script-installed tool pattern

Use one bundled setup tool to install a lazy script-backed command:

```yaml
tools:
  - name: setup-echo
    runtime: oci
    from: bundle.setup-echo
  - name: echo-tool
    default: true
    runtime: script
    script: setup-echo "$KIOX_TOOL_BIN"
    dependsOn:
      - tool: setup-echo
```

## Multi-command provider pattern

A provider can expose multiple command names from one default tool:

```yaml
tools:
  - name: node
    default: true
    runtime: oci
    from: bundle.node
    provides:
      - node
      - npm
      - npx
```

That pattern is useful when one packaged binary tree should show up through several familiar command names.

## Asset-heavy provider pattern

Providers that need templates, certificates, or policy bundles should package them as asset bundles and mount them into the provider store:

```yaml
bundles:
  - name: policy-assets
    type: asset
    platforms:
      - os: any
        arch: any
        source: assets/policy
assets:
  - name: policy-assets
    from: bundle.policy-assets
    mount:
      path: assets
environments:
  - name: default-env
    variables:
      POLICY_ROOT: ${provider_assets}/policy
    export:
      - POLICY_ROOT
```

The asset bundle is extracted into the provider store when the runtime first needs the provider.
