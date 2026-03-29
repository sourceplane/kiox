# Tinx Technical Specification (v1.0)

## 1. Overview

tinx is a CLI toolchain designed for the lifecycle management of OCI-native providers. It facilitates building, packaging, and executing modular extensions (providers) that expose specific capabilities (e.g., plan, validate, release).

### Core Philosophy

- **OCI-Native**: Everything is an OCI artifact.
- **Provider-First**: The CLI is a thin runtime; logic lives in providers.
- **Sub-second Installs**: Metadata-first pulling for instant discovery.
- **Language Agnostic**: Binary-based execution contract.

### Experimental Compatibility Layer

In addition to OCI-native providers, tinx now includes an experimental `gha://` compatibility layer.
This layer treats a GitHub Action repository as a provider source, clones it into the tinx cache, synthesizes provider metadata locally, and executes supported runtimes through the normal `tinx run` flow.

Initial scope is intentionally narrow:

- Composite actions
- Node actions executed with the system `node` already available on `PATH`
- Shell `run:` steps inside composite actions
- `GITHUB_ENV`, `GITHUB_PATH`, `GITHUB_OUTPUT`, and `GITHUB_STATE` support
- Cached action source and runtime state under the provider home

Install-time provider materialization:

- `tinx install <alias> gha://... --input name=value` creates an alias-scoped installed provider instead of only caching the source repository
- Install-time inputs are persisted on the installed provider and used as the default bootstrap configuration for later runs
- If the bootstrap execution adds exactly one managed executable on the action-controlled `PATH`, tinx promotes that executable into a local binary provider entrypoint
- Promoted providers are materialized under the normal provider home with a generated local `tinx.yaml` and passthrough invocation semantics
- If bootstrap does not produce a single unambiguous managed executable, tinx preserves the provider as an action-backed runtime rather than guessing
- The GHA runtime exposes a stable `tinx` shim on `PATH` so actions can invoke tinx recursively when needed

Current non-goals for the compatibility layer:

- Docker actions
- Nested `uses:` steps inside composite actions
- tinx-managed Node provisioning (a future native provider can supply the Node runtime later)

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
