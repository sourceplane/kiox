---
title: Configuration
---

kiox configuration is split across desired workspace state, provider package manifests, generated lock files, and kiox home state.

## Desired vs actual state

| File | Owner | Purpose |
| --- | --- | --- |
| `kiox.yaml` | User | Desired workspace name and provider declarations |
| `kiox.lock` | kiox | Resolved provider versions, digests, and store bindings |

## Workspace manifest

Workspace manifests live in the workspace root and are normally saved as `kiox.yaml`.

```yaml
apiVersion: kiox.io/v1
kind: Workspace
metadata:
  name: dev
providers:
  node:
    source: core/node
  kubectl:
    source: ghcr.io/acme/setup-kubectl:v1.31.0
    plainHTTP: false
```

### Workspace fields

| Field | Meaning |
| --- | --- |
| `apiVersion` | Must be `kiox.io/v1` |
| `kind` | Must be `Workspace` |
| `metadata.name` | Workspace name used by the CLI and lock file |
| `providers` | Alias to provider source mapping |
| `providers.<alias>.source` | OCI registry reference or local OCI layout path |
| `providers.<alias>.plainHTTP` | Allow plain HTTP registry access for that provider |

`kiox init` creates this file if it does not exist and initializes from it if it already exists. `kiox add` and `kiox remove` only rewrite `kiox.yaml` after provider reconciliation succeeds.

Manual edits are expected. Run `kiox sync` after editing `kiox.yaml`, or let `kiox status`, `kiox exec`, `kiox shell`, or `kiox -- ...` reconcile automatically on the next workspace command.

A provider entry may be a string shorthand:

```yaml
providers:
  node: core/node
```

## Provider package manifest

Provider package manifests are authored as `provider.yaml`. Legacy `kiox.yaml` provider manifests are still supported for backward compatibility. Workspace manifests stay in `kiox.yaml`.

The current built-in runtime actively uses `Tool`, `Bundle`, `Asset`, and `Environment` resources. Some additional fields are parsed into package metadata for future expansion; those are called out explicitly below.

```yaml
apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: acme
  name: node-toolchain
  version: v20.19.0
  description: Node.js runtime provider
spec:
  tools:
    - name: node
      default: true
      runtime: oci
      from: bundle.node
      provides:
        - node
        - npm
        - npx
      capabilities:
        build:
          description: Run the build
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
    - name: node-assets
      type: asset
      platforms:
        - os: any
          arch: any
          source: assets
  assets:
    - name: node-assets
      from: bundle.node-assets
      mount:
        path: assets
  environments:
    - name: default-env
      variables:
        NODE_EXTRA_CA_CERTS: ${provider_assets}/certs/root.pem
      export:
        - NODE_EXTRA_CA_CERTS
      path:
        - tools/bin
```

### Provider document fields

| Field | Meaning |
| --- | --- |
| `metadata.namespace` | Provider namespace |
| `metadata.name` | Provider name |
| `metadata.version` | Provider version |
| `spec.contents` | Optional ordered list of exported resources; synthesized when omitted |
| `spec.tools` | Inline tool resources |
| `spec.bundles` or `spec.bundle` | Inline bundle resources |
| `spec.assets` | Inline asset resources |
| `spec.environments` | Inline environment resources |
| `spec.dependencies` | Parsed into package metadata; current runtime does not auto-resolve provider-to-provider dependencies |
| `spec.secrets` | Parsed into package metadata; current runtime does not inject them automatically |
| `spec.workspaces` | Parsed into package metadata; current runtime does not create workspaces from them automatically |

### Tool fields

| Field | Meaning |
| --- | --- |
| `default` | Marks the tool as the provider default |
| `runtime.type` | Runtime plugin: `oci`, `script`, or `local` |
| `source.type` | Source type used by the selected runtime |
| `source.ref` | Bundle reference or local runtime reference |
| `source.path` | Local path for `local` tools |
| `source.script` | Install script for `script` tools |
| `from` | Shortcut for `source`, such as `bundle.node` |
| `install.strategy` | Install policy; current runtime defaults to `lazy` |
| `install.tool` | Installer tool for managed-install targets |
| `install.path` | Relative binary path to create under the tool install root |
| `provides` | Command names exported into the workspace shell |
| `dependsOn` | Other tools that must be available before this tool runs |
| `environments` | Environment resources attached to the tool |
| `capabilities` | Optional metadata about supported actions |
| `env` | Tool-specific environment variables |
| `path` | Tool-specific path additions |

The current built-in runtime and source combinations are:

- `oci` + `bundle`
- `script` + `script`
- `local` + `source.path` or `source.ref`
- `local` + `install.tool` and `install.path` for managed-install targets

### Bundle fields

| Field | Meaning |
| --- | --- |
| `type` | Optional bundle type; `asset` defaults layers to a tar media type |
| `layers` | Explicit layer list with `platform`, `mediaType`, and `source` |
| `platforms` | Shorthand layer list with `os`, `arch`, `source`, and optional `mediaType` |

### Asset and environment fields

| Field | Meaning |
| --- | --- |
| `assets[].from` | Shortcut for a bundle-backed asset source such as `bundle.node-assets` |
| `assets[].mount.path` | Mount root inside the provider store |
| `environments[].variables` | Variables exported into the workspace shell |
| `environments[].export` | Explicit allow-list of exported keys |
| `environments[].path` | Additional path entries relative to the provider store |

## Managed-install pattern

Setup-style providers model the user-facing command as a normal tool and let kiox install it through another tool:

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

In that model, `kubectl` has its own tool identity, shim, and inventory entry even though `setup-kubectl` creates the binary lazily.

## Multi-document form

Provider packages can also be authored as multiple YAML documents. The canonical runtime-active kinds are:

- `Provider`
- `Tool`
- `Bundle`
- `Asset`
- `Environment`

`Secret` and provider-local `Workspace` kinds are parsed into package metadata, but the current workspace sync and runtime do not consume them automatically.

That multi-document form is useful when a provider has many tools or when you want cleaner diffs between resources.

## Legacy compatibility manifest

kiox still accepts the legacy single-tool shorthand:

```yaml
apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: acme
  name: node
  version: v20.19.0
spec:
  runtime: binary
  entrypoint: node
  platforms:
    - os: darwin
      arch: arm64
      binary: bin/darwin/arm64/node
  env:
    NODE_EXTRA_CA_CERTS: ${provider_assets}/certs/root.pem
  path:
    - tools/bin
  layers:
    assets:
      root: assets
```

Internally, kiox normalizes that into a default `Tool`, a `Bundle`, an `Environment`, and an `Asset` mount when assets are declared.

## Lock file

Each workspace sync writes `kiox.lock`:

```yaml
apiVersion: kiox.io/v1
kind: WorkspaceLock
metadata:
  name: dev
providers:
  - alias: node
    provider: core/node
    source: core/node
    version: v20.19.0
    resolved: ghcr.io/acme/node-provider@sha256:...
    store: 4f3f...
```

Use the lock file as generated state. kiox rewrites it during sync and it should not be edited by hand.

## Global kiox home config

kiox stores shared state in `$KIOX_HOME/config.yaml`:

```yaml
aliases:
  node: core/node@v20.19.0
activeWorkspace: /abs/path/to/workspace
workspaces:
  dev: /abs/path/to/workspace
```

That file tracks aliases, the active workspace, and registered workspace roots.
