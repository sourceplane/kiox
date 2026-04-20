---
title: Writing providers
---

Write a kiox provider package when you want to package a command or toolchain as an OCI artifact and expose one or more commands inside a workspace shell.

Provider packages use `provider.yaml` as the preferred authoring manifest. Workspace manifests remain `kiox.yaml`. Legacy `kiox.yaml` provider manifests still work, but new provider packages should use the split naming.

## Provider checklist

1. Create a `provider.yaml` manifest.
2. Choose inline or multi-document authoring.
3. Define one or more tools and mark one as default.
4. Declare bundle layers for binaries or assets.
5. Choose a runtime per tool: `oci`, `script`, or `local`.
6. Declare environments, paths, and optional assets.
7. Package with `kiox pack` or `kiox release`.
8. Test with `kiox init`, `kiox ls`, `kiox status`, and a real command run.

## Canonical inline manifest

```yaml
apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: acme
  name: node-toolchain
  version: v20.19.0
  description: Node.js toolchain provider
spec:
  tools:
    - name: node
      default: true
      runtime: oci
      source:
        type: bundle
        ref: node
      provides:
        - node
        - npm
        - npx
      environments:
        - default-env
  bundles:
    - name: node
      layers:
        - platform:
            os: darwin
            arch: arm64
          mediaType: application/vnd.kiox.tool.binary
          source: bin/darwin/arm64/node
        - platform:
            os: linux
            arch: amd64
          mediaType: application/vnd.kiox.tool.binary
          source: bin/linux/amd64/node
  environments:
    - name: default-env
      variables:
        NODE_HOME: ${provider_home}
      export:
        - NODE_HOME
```

The canonical provider model is resource-based. kiox still accepts the legacy `runtime: binary` manifest shorthand, but it now normalizes that into the same internal model.

## Multi-document authoring

Use multiple YAML documents when a provider grows beyond a simple inline file:

```yaml
apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: acme
  name: multi-doc
  version: v0.1.0
spec:
  contents:
    - Tool: setup-tool
    - Tool: default-tool
    - Bundle: setup-tool
---
apiVersion: kiox.io/v1
kind: Bundle
metadata:
  name: setup-tool
spec:
  layers:
    - platform:
        os: linux
        arch: amd64
      mediaType: application/vnd.kiox.tool.binary
      source: bin/linux/amd64/setup-tool
---
apiVersion: kiox.io/v1
kind: Tool
metadata:
  name: default-tool
spec:
  default: true
  runtime:
    type: script
  source:
    type: script
    script: setup-tool "$KIOX_TOOL_BIN"
```

Both forms normalize to the same package model.

## Choose a runtime

### `oci`

Use `oci` for tools that should be materialized from bundle layers published in the provider artifact.

### `script`

Use `script` when the tool should lazily install itself on first use.

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
    provides:
      - echo-tool
```

The install script must write an executable to `KIOX_TOOL_BIN`.

### `local`

Use `local` when the tool should run from an existing path or from a path created by another tool.

That is the current model for setup-style providers:

```yaml
tools:
  - name: setup-kubectl
    runtime: oci
    from: bundle.setup-kubectl
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

## Install-time environment variables

When kiox runs a `script` tool install, it injects:

| Variable | Meaning |
| --- | --- |
| `KIOX_TOOL_INSTALL_DIR` | Tool-specific install root in the provider store |
| `KIOX_TOOL_BIN` | Exact executable path the install must create |
| `KIOX_TOOL_NAME` | Tool resource name |
| `KIOX_TOOL_COMMAND` | Primary command exposed by the tool |
| `KIOX_PROVIDER_HOME` | Provider store root |

When a tool delegates installation to another tool through `install.tool`, the installer also receives:

| Variable | Meaning |
| --- | --- |
| `KIOX_TARGET_TOOL_NAME` | Tool being installed |
| `KIOX_TARGET_TOOL_BIN` | Exact binary path the installer must create |
| `KIOX_TARGET_TOOL_COMMAND` | Primary command exposed by the target tool |
| `KIOX_TARGET_TOOL_INSTALL_DIR` | Install root for the target tool |

## Add capability metadata

Capabilities document what a tool does. They are published in metadata and can be surfaced in tooling, but the tool still receives normal CLI arguments.

```yaml
tools:
  - name: node
    capabilities:
      build:
        description: Compile the application
      test:
        description: Run the test suite
```

## Add environment and assets

Use environment resources for exported variables and shared paths. Use asset bundles when you need templates, certificates, or policy content.

```yaml
bundles:
  - name: templates
    type: asset
    platforms:
      - os: any
        arch: any
        source: assets/templates
assets:
  - name: templates
    from: bundle.templates
    mount:
      path: assets
environments:
  - name: default-env
    variables:
      TEMPLATE_ROOT: ${provider_assets}/templates
      WORKSPACE_ROOT: ${workspace_root}
    export:
      - TEMPLATE_ROOT
      - WORKSPACE_ROOT
```

Available template values include:

- `${cwd}`
- `${workspace_root}`
- `${workspace_home}`
- `${provider_ref}`
- `${provider_home}`
- `${provider_binary}`
- `${provider_assets}`

Do not use the reserved `KIOX_` prefix in provider environment variables.

## Legacy compatibility

This still works:

```yaml
spec:
  runtime: binary
  entrypoint: node
  platforms:
    - os: darwin
      arch: arm64
      binary: bin/darwin/arm64/node
```

Use it when you need the old shorthand, but prefer the normalized package model for new providers.

## Test locally

```bash
kiox release --manifest provider.yaml --dist dist --output oci
kiox init demo -p ./oci as node
kiox sync
kiox --workspace demo ls
kiox --workspace demo exec node --version
```

## Validate across platforms

List every platform you publish in the relevant bundle layers. `kiox release` uses those layer targets to build or validate the provider artifact before it packs the OCI layout.

Next, read [provider packaging](./provider-packaging.md).
