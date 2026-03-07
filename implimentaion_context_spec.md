# Tinx Implementation Details Specification (v1.0)

## 1. Introduction

This document serves as the internal implementation reference for tinx, the OCI-native provider toolchain. It codifies the architectural decisions discussed during the design phase, specifically focusing on how the CLI orchestrates builds, manages the local provider cache, and interacts with OCI registries using the ORAS (OCI Registry as Storage) protocol.

## 2. Core Architecture: The "Command Bus" Model

tinx is designed as a "Thin Runtime." It does not contain domain-specific logic (e.g., CI planning, Docker building). Instead, it acts as a dispatcher.

### 2.1 Component Diagram

```
[ User CLI ] <---> [ tinx Core ] <---> [ Local Cache (~/.tinx) ]
                         |
                 [ Plugin Resolver ] <---> [ OCI Registry (GHCR/ECR) ]
                         |
                 [ Runtime Selector ]
                         |
          +--------------+--------------+
          |              |              |
   [ Native Binary ]  [ Docker ]   [ WASM (v2) ]
```

### 2.2 Execution Pipeline

1. **Command Resolution**: User runs `tinx ci plan`. tinx looks up the ci alias in the local configuration.
2. **Metadata Verification**: tinx checks `~/.tinx/providers/sourceplane/lite-ci/metadata.json` to verify the plan capability exists.
3. **Binary Readiness**: If the binary for the current GOOS/GOARCH is missing, it initiates a "Phase 2" lazy pull.
4. **Environment Preparation**: tinx sets up a temporary workspace and injects the `{{.ProviderHome}}` variable.
5. **Execution**: The entrypoint is invoked: `./entrypoint plan [args]`.

## 3. The tinx.yaml Schema Deep Dive

The tinx.yaml is a versioned API. v1 focuses on `Kind: Provider`.

### 3.1 Field Specifications

- **apiVersion**: `tinx.io/v1`
- **kind**: `Provider`
- **metadata.namespace**: Used to create the OCI repository path (e.g., `ghcr.io/namespace/name`).
- **spec.runtime**:
  - `binary`: Default. Executes a local file.
  - `container`: Future. Executes via docker run or podman.
- **spec.entrypoint**: The filename of the primary executable. This file must exist in every platform-specific binary layer.
- **spec.capabilities**: A map of strings. These are used to generate shell auto-completion and help menus.

## 4. OCI Artifact Strategy

tinx treats the OCI registry as a structured filesystem.

### 4.1 Media Type Definitions

We use strongly-typed media types to allow the tinx client to selectively pull layers.

| Media Type | Content | Purpose |
|---|---|---|
| `application/vnd.tinx.provider.config.v1+json` | OCI Config Object | Fast inspection for tinx search. |
| `application/vnd.tinx.provider.manifest.v1+yaml` | tinx.yaml | The source of truth for the runtime. |
| `application/vnd.tinx.provider.metadata.v1+json` | Capability Index | Used for CLI help/completion. |
| `application/vnd.tinx.provider.binary.linux.amd64.v1` | Binary Blob | Compressed executable for Linux x64. |
| `application/vnd.tinx.provider.binary.darwin.arm64.v1` | Binary Blob | Compressed executable for macOS (M-series). |
| `application/vnd.tinx.provider.assets.v1+tar` | Assets Blob | Tarball of static resources. |

### 4.2 Layer Indexing

The OCI Manifest contains annotations to help tinx identify the platform without downloading the blob:

```json
{
  "mediaType": "application/vnd.tinx.provider.binary.linux.amd64.v1",
  "annotations": {
    "org.opencontainers.image.title": "bin/linux/amd64/entrypoint",
    "io.tinx.platform": "linux/amd64"
  }
}
```

## 5. Sub-second Install Engine (Metadata-First)

To avoid the "Heavy Installer" problem found in Terraform or NPM, tinx uses a split-fetch approach.

### 5.1 Phase 1: Registration

```bash
tinx install sourceplane/lite-ci
```

1. Fetch the OCI Manifest.
2. Identify the `vnd.tinx.provider.manifest.v1+yaml` layer and the `vnd.tinx.provider.metadata.v1+json` layer.
3. Download and cache these two small files (< 20KB total).
4. Register the alias in `~/.tinx/aliases.yaml`.

### 5.2 Phase 2: Execution (Lazy Pull)

```bash
tinx ci plan
```

1. tinx checks `~/.tinx/providers/sourceplane/lite-ci/v0.1.0/bin/[OS]/[ARCH]/entrypoint`.
2. If the file is absent:
   - tinx looks at the cached manifest to find the layer matching the host's OS/Arch.
   - It pulls only that layer.
   - It applies `chmod +x` to the resulting binary.
   - It proceeds to execution.

## 6. The tinx release Orchestrator

The `tinx release` command is a wrapper that manages the "Artifact Assembly" process.

### 6.1 Delegation Logic

1. **Build Detection**: If go.mod is present, tinx looks for goreleaser.yaml.
2. **Execution**: tinx invokes `goreleaser build --clean --skip=validate`.
3. **Artifact Harvesting**: tinx scans the dist/ directory for binaries matching the platforms defined in tinx.yaml.
4. **Packaging**:
   - Creates a temporary OCI layout.
   - Generates the OCI Config JSON.
   - Bundles the ./assets directory into a tarball.
5. **ORAS Push**:
   - Connects to the registry (defaulting to GHCR).
   - Pushes the manifest, config, and layers.

## 7. Local Cache Management

The `~/.tinx` directory is structured to support multi-versioning and multi-architecture.

### 7.1 Directory Layout

```
~/.tinx/config.yaml                           # Global settings (default registry, aliases)
~/.tinx/aliases.yaml                          # Maps short commands (e.g., ci) to full provider refs
~/.tinx/providers/
├── [namespace]/[name]/metadata.json          # The latest metadata
├── [namespace]/[name]/[version]/tinx.yaml    # The manifest for this version
├── [namespace]/[name]/[version]/bin/
│   └── [os]/[arch]/                          # The executable
└── [namespace]/[name]/[version]/assets/      # Shared assets
```

## 8. Provider Process Contract (The "ABI")

Providers do not use a complex SDK. They follow standard UNIX principles.

### 8.1 Entrypoint Protocol

**Arguments**: `[capability] [user-args...]`

**Standard Variables**:
- `TINX_PROVIDER_HOME`: Path to the provider's cached directory.
- `TINX_VERSION`: Version of the tinx CLI.
- `TINX_CONTEXT`: A JSON blob containing contextual info (if applicable).

**Exit Codes**:
- `0`: Success.
- `1`: Generic error.
- `126`: Command not found (Capability not implemented).

**Communication**: Capabilities should output results to stdout. Logging should go to stderr.

## 9. Future-Proofing: Resource Kinds

While v1 focus is Provider, the tinx parser is built to handle multiple Kind values to prevent "monolith manifest" syndrome.

### 9.1 Planned Kinds

- **Kind: Workflow**: A YAML DAG that executes multiple providers in sequence.
- **Kind: Bundle**: An OCI artifact containing an index of other providers (e.g., a "DevOps Starter Pack").
- **Kind: Intent**: A high-level declarative state (e.g., "I want a production-ready API") that providers translate into plans.

## 10. Security & Provenance

As a CNCF-standard tool, tinx is designed to support the software supply chain.

### 10.1 Signing (v1.1+)

- tinx will integrate with Sigstore/Cosign.
- Providers will be signed after the `tinx release push`.
- `tinx install` will verify signatures using the `--verify` flag or global config.

### 10.2 SBOM Generation

- `tinx release` will generate an SBOM (Software Bill of Materials) in SPDX/CycloneDX format.
- The SBOM will be pushed as an OCI Referrer to the provider artifact.

## 11. Summary of Internal Packages (Go)

- **internal/config**: Logic for loading/parsing tinx.yaml.
- **internal/oci**: Wrapper around oras-go/v2 for registry operations.
- **internal/resolver**: Maps namespace/name to OCI endpoints.
- **internal/runtime**: Handles OS-level process execution and chmod.
- **internal/ui**: Minimalist logger for CLI feedback.

## 12. Conclusion

The implementation of tinx represents a shift toward "Infrastructure as Artifacts." By leveraging OCI as the distribution layer and a metadata-first installation strategy, tinx provides a sub-second developer experience while maintaining strict compatibility with the cloud-native ecosystem.
