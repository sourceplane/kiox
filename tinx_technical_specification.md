# Tinx Technical Specification (v1.0)

## 1. Overview

tinx is a CLI toolchain designed for the lifecycle management of OCI-native providers. It facilitates building, packaging, and executing modular extensions (providers) that expose specific capabilities (e.g., plan, validate, release).

### Core Philosophy

- **OCI-Native**: Everything is an OCI artifact.
- **Provider-First**: The CLI is a thin runtime; logic lives in providers.
- **Sub-second Installs**: Metadata-first pulling for instant discovery.
- **Language Agnostic**: Binary-based execution contract.

## 2. The tinx.yaml Contract

The tinx.yaml file is the source of truth for a provider. It follows a Kubernetes-style resource manifest.

```yaml
apiVersion: tinx.io/v1
kind: Provider

metadata:
    name: lite-ci
    namespace: sourceplane
    version: v0.1.0
    description: Provider-native CI planning engine
    homepage: https://github.com/sourceplane/lite-ci
    license: Apache-2.0

spec:
    runtime: binary
    entrypoint: entrypoint # The binary filename inside the artifact

    platforms:
        - os: linux
            arch: amd64
            binary: bin/linux/amd64/entrypoint
        - os: darwin
            arch: arm64
            binary: bin/darwin/arm64/entrypoint

    capabilities:
        plan:
            description: Generate an execution plan
        validate:
            description: Validate intent and component definitions

    layers:
        assets:
            root: ./assets
            includes: ["schemas/**", "templates/**"]
```

## 3. OCI Artifact Specification

tinx utilizes custom media types to allow registries to distinguish between configuration, metadata, and platform-specific binaries.

### Media Types

| Layer / Object | Media Type |
|---|---|
| Artifact Config | `application/vnd.tinx.provider.config.v1+json` |
| Provider Manifest | `application/vnd.tinx.provider.manifest.v1+yaml` |
| Platform Binary | `application/vnd.tinx.provider.binary.<os>.<arch>.v1` |
| Static Assets | `application/vnd.tinx.provider.assets.v1+tar` |

### 2-Phase Install Strategy

- **Phase 1 (Metadata Pull)**: `tinx install` fetches only the Config and Manifest layers (~50ms).
- **Phase 2 (Lazy Binary Pull)**: `tinx run` detects the local OS/Arch and pulls the specific binary layer only if not cached.

## 4. CLI Architecture

tinx acts as a command bus.

### Primary Commands

- `tinx install <namespace>/<provider>`: Resolves OCI ref and pulls metadata.
- `tinx run <provider> <capability> [args]`: Executes the provider binary.
- `tinx <alias> <capability>`: Syntactic sugar for run.
- `tinx pack`: Packages local source into an OCI layout.
- `tinx release`: Builds (via GoReleaser delegation), packs, and pushes.

## 5. Execution Model

Execution is a simple process contract. No complex RPC/gRPC is required for v1.

**Command**: `tinx ci plan --intent ./intent.yaml`

**Internal Resolution**:
1. Lookup alias `ci` → `sourceplane/lite-ci`.
2. Check local cache `~/.tinx/providers/sourceplane/lite-ci/v0.1.0/`.
3. If binary missing, pull `application/vnd.tinx.provider.binary.linux.amd64.v1` via ORAS.
4. Execute: `./entrypoint plan --intent ./intent.yaml`.

## 6. Registry & Discovery

tinx is registry-agnostic. By default, it maps `namespace/provider` to `ghcr.io/namespace/provider`. Future iterations will support tinx registry search via a centralized metadata index.
