# tinx Reimplementation Specification

**Status:** Draft  
**Scope:** Architecture, contracts, behavior, state, and externally observable semantics  
**Audience:** Implementers building a compatible tinx runtime in another language

This document specifies tinx as a command-line system for packaging provider binaries as OCI artifacts, installing provider metadata and runtime material into cacheable homes, and composing providers into workspace-local shell environments.

This specification is intentionally behavior-first. It defines externally meaningful contracts and state, not implementation internals.

## Conformance language

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirements for a compatible implementation.

## 1. System overview

tinx is a single CLI with three tightly coupled responsibilities:

1. **Provider packaging:** build and package provider binaries, manifest metadata, and optional assets into an OCI image layout, and optionally push that layout to a registry.
2. **Provider installation and caching:** ingest providers from a local OCI layout or an OCI registry reference into one or more homes, persist provider metadata, and maintain a deduplicated local store of OCI content.
3. **Workspace runtime composition:** declare provider aliases in a workspace manifest, resolve them into locked versions, materialize a workspace shell environment, and execute arbitrary commands against that environment.

The system has two distinct state scopes:

| Scope | Purpose | Typical location |
| --- | --- | --- |
| **Global home** | Registry-facing control plane, default provider inventory, shared OCI store, active workspace registry | `~/.tinx` or `TINX_HOME` / `--tinx-home` |
| **Workspace home** | Workspace-local activation state, provider metadata for that workspace, generated shell artifacts | `<workspace>/.workspace` |

The system also distinguishes between two kinds of provider presence:

| Presence | Meaning |
| --- | --- |
| **Installed metadata** | Provider metadata is available in a home and can be listed or aliased |
| **Materialized runtime** | Provider assets and the current platform binary have been extracted and are executable |

Provider execution is workspace-centric. Standalone installation is supported, but standalone execution is not part of the current contract.

## 2. Core concepts

| Concept | Definition |
| --- | --- |
| **Provider** | A binary runtime packaged as an OCI artifact with manifest, metadata, optional assets, and one or more platform-specific binary layers |
| **Provider reference** | A logical identifier of the form `<namespace>/<name>` |
| **Provider source** | Either a local OCI image layout path or a registry reference |
| **Alias** | A workspace-local or home-local command name that resolves to a concrete provider version |
| **Workspace** | A directory rooted by a workspace manifest and backed by a `.workspace` activation home |
| **Selected workspace** | The workspace chosen for a command by explicit flag, discovery, or active-workspace state |
| **Default scope** | The provider inventory stored directly in the global home, outside any workspace |
| **Activation home** | The home into which provider metadata and aliases are written for the current scope |
| **Store entry** | A content-deduplicated cached OCI layout keyed by provider identity plus manifest digest |
| **Lock entry** | A resolved provider record stored in `tinx.lock`, including source, version, digest-pinned resolution, and store identifier |
| **Shim** | A generated executable in `.workspace/bin/` that forwards argv to a materialized provider binary |
| **Capability** | Declarative metadata about a provider command surface; capabilities are informational only and do not drive dispatch |
| **Materialization** | Extracting cached assets and the current platform binary from the OCI store into executable filesystem paths |

## 3. Module responsibilities

tinx can be reimplemented as the following logical modules:

| Module | Responsibilities |
| --- | --- |
| **CLI dispatcher** | Parse global options, dispatch commands, apply compatibility shortcuts, and standardize output/error behavior |
| **Workspace registry** | Track known workspaces, active workspace, and workspace-name-to-root mappings |
| **Workspace model** | Load, normalize, validate, save, and discover workspace manifests; generate and persist lock files |
| **Provider model** | Load, normalize, validate, and interpret provider manifests |
| **Source resolver** | Distinguish short provider refs, explicit registry refs, digest refs, path-like refs, and unsupported scheme refs |
| **OCI packager** | Write OCI layouts, media-type descriptors, tags, metadata layers, asset tar layers, and binary layers |
| **Registry client** | Push and pull OCI layouts, authenticate to registries, and hydrate incomplete local stores |
| **Store manager** | Deduplicate OCI content, persist provider metadata per activation home, and maintain install source records |
| **Workspace reconciler** | Resolve declared providers into installed metadata and lock entries, including refresh semantics |
| **Shell/runtime composer** | Build environment variables, path entries, shims, env files, and path files; launch interactive shells or one-shot commands |
| **Inventory renderer** | Render workspace and provider tables, summaries, status markers, and normalized paths |
| **Build orchestrator** | Build provider binaries directly with Go or delegate to GoReleaser |

## 4. Data models

### 4.1 Provider manifest

A provider manifest is YAML and MUST conform to the following logical shape:

```yaml
apiVersion: tinx.io/v1
kind: Provider
metadata:
  namespace: <string>
  name: <string>
  version: <string>
  description: <string?>      # optional
  homepage: <string?>         # optional
  license: <string?>          # optional
spec:
  runtime: binary
  entrypoint: <string>
  platforms:
    - os: <string>
      arch: <string>
      binary: <relative-path>
  capabilities:
    <capability-name>:
      description: <string?>  # optional
  env:
    <ENV_KEY>: <template-string>
  path:
    - <path-or-template>
  layers:
    assets:
      root: <relative-path?>
      includes:
        - <string>            # accepted but currently ignored by runtime and pack flow
```

Provider manifest rules:

1. `apiVersion` defaults to `tinx.io/v1` when omitted.
2. `kind` MUST be exactly `Provider`.
3. `metadata.namespace`, `metadata.name`, and `metadata.version` are required.
4. `spec.runtime` defaults to `binary` and MUST be `binary`.
5. `spec.entrypoint` is required.
6. `spec.platforms` MUST contain at least one entry.
7. Each `(os, arch)` pair in `spec.platforms` MUST be unique.
8. Each platform entry MUST define `binary`.
9. `spec.path` entries are trimmed and slash-normalized.
10. `spec.env` keys MUST NOT be empty and MUST NOT start with `TINX_` in any case variant.
11. `spec.path` entries MUST NOT be empty after trimming.
12. Capability names are advisory metadata. tinx does not perform capability-based dispatch.

### 4.2 Workspace manifest

A workspace manifest is YAML and MAY be authored in either of the following accepted shapes:

**Normalized shape**

```yaml
apiVersion: tinx.io/v1
kind: workspace
workspace: <string>
metadata:
  name: <string?>
providers:
  <alias>: <source-string>
  <alias>:
    source: <source-string>
    plainHTTP: <bool?>
```

**Accepted alternative input shape**

```yaml
apiVersion: tinx.io/v1
kind: Workspace
metadata:
  name: <string>
spec:
  providers:
    <alias>: <source-or-mapping>
```

Workspace manifest rules:

1. The implementation MUST recognize the following manifest filenames during discovery:
   1. `tinx.yaml`
   2. `tinx.yml`
   3. `providers.tx.yaml`
   4. `providers.tx.yml`
   5. `providers.tinx.yaml`
   6. `providers.tinx.yml`
2. `kind` is case-insensitive for workspace input.
3. If `kind` is omitted and a workspace name or provider map is present, the document MUST be treated as a workspace manifest.
4. `apiVersion` defaults to `tinx.io/v1`.
5. `metadata.name` defaults to the `workspace` field.
6. If `workspace` is omitted on save, it MUST be derived from `metadata.name`.
7. Provider aliases MUST NOT be empty.
8. Provider aliases MUST NOT contain `/`.
9. Provider sources MUST NOT be empty.
10. The normalized file written by tinx MUST use the top-level `providers:` form and the literal `kind: workspace`.

### 4.3 Workspace lock file

Each workspace materialization MUST generate `<workspace-root>/tinx.lock` with the following shape:

```yaml
apiVersion: tinx.io/v1
kind: WorkspaceLock
workspace: <string>
providers:
  - alias: <alias>
    provider: <namespace>/<name>
    source: <declared-source>
    version: <resolved-version>
    resolved: <resolved-source?>
    store: <store-id?>
```

Lock semantics:

1. Entries MUST be ordered by alias.
2. `source` records the declared source after workspace-relative resolution.
3. `resolved` records the immutable or effective source used for the last successful install.
4. `store` records the store identifier backing the resolved provider content.

### 4.4 Home config

Both global and workspace homes use the same YAML config schema:

```yaml
aliases:
  <alias>: <namespace>/<name>@<version>
activeWorkspace: <absolute-workspace-root?>
workspaces:
  <workspace-name>: <absolute-workspace-root>
```

Config rules:

1. The canonical path is `<home>/config.yaml`.
2. If `config.yaml` does not exist, the implementation SHOULD read `<home>/aliases.yaml` as a backward-compatibility fallback.
3. Writes MUST target `config.yaml`.
4. Workspace homes typically populate `aliases`; global homes populate `aliases`, `activeWorkspace`, and `workspaces`.

### 4.5 Provider metadata record

Each installed provider version in an activation home MUST persist `metadata.json` with the following logical content:

```json
{
  "namespace": "<string>",
  "name": "<string>",
  "version": "<string>",
  "storeID": "<string?>",
  "storePath": "<string?>",
  "description": "<string?>",
  "homepage": "<string?>",
  "license": "<string?>",
  "runtime": "binary",
  "entrypoint": "<string>",
  "capabilities": ["<name>", "..."],
  "capabilityDescriptions": { "<name>": "<string>" },
  "platforms": [{ "os": "<string>", "arch": "<string>" }],
  "source": {
    "layoutPath": "<path>",
    "tag": "<tag-or-digest-token>",
    "ref": "<registry-ref-or-digest-ref?>",
    "plainHTTP": <bool?>
  },
  "installedAt": "<timestamp>"
}
```

Metadata rules:

1. `capabilities` MUST be lexicographically sorted.
2. `platforms` contains summaries only; it does not repeat the binary path.
3. `source.layoutPath` points to the cached OCI layout inside the store, not the original remote ref and not the original local layout path.
4. `storePath` points to the store root, not the `oci` subdirectory.

### 4.6 Install source record

Each installed provider version in an activation home MUST also persist `install.json` containing the `source` object shown above. The current runtime does not rely on this file for normal operation, but it is part of the persistent contract.

## 5. CLI contract

### 5.1 Top-level command set

tinx MUST expose the following top-level commands:

| Command | Purpose |
| --- | --- |
| `init` | Create or materialize a workspace |
| `status` | Report current workspace or default-scope status |
| `workspace` | Workspace management group |
| `provider` | Provider management group |
| `add` | Compatibility alias for workspace provider add |
| `remove` | Compatibility alias for workspace provider remove |
| `update` | Compatibility alias for workspace provider update |
| `use` | Compatibility alias for workspace use |
| `install` | Install provider metadata into the current or default scope |
| `list` | Inventory command with subcommands and compatibility behavior |
| `shell` | Launch an interactive workspace shell |
| `exec` | Run a command inside the selected workspace environment |
| `run` | Removed command retained only to emit a migration error |
| `pack` | Package a provider into a local OCI layout |
| `release` | Build, package, and optionally push a provider artifact |
| `version` | Print the tinx version |

Aliases:

| Canonical command | Aliases |
| --- | --- |
| `workspace` | `ws`, `workspaces` |
| `provider` | `providers`, `p` |
| `list` | `ls` |
| `workspace activate` | hidden deprecated alias for `workspace use` |

### 5.2 Global options

The root command MUST accept:

| Option | Meaning |
| --- | --- |
| `--tinx-home <path>` | Override the global home used for registry-facing state and shared store |
| `--workspace <workspace>` / `-w <workspace>` | Select a workspace for commands that operate on the selected workspace |

Global option rules:

1. `--tinx-home` MUST take precedence over the `TINX_HOME` environment variable.
2. If neither is provided, the default global home is `~/.tinx`.
3. `--workspace` is ephemeral. It MUST affect only the current command and MUST NOT mutate active-workspace state.
4. `--workspace` MUST support both separated and `=` syntax. `-w` MUST support both separated and `=` syntax.
5. Missing values for `--tinx-home` or `--workspace` MUST be reported as hard errors.

### 5.3 Compatibility shortcut: root `--`

The root command MUST treat a leading `--` as a compatibility shortcut:

| Form | Meaning |
| --- | --- |
| `tinx --` | Equivalent to `tinx shell` on the selected workspace |
| `tinx -- <command> [args...]` | Equivalent to `tinx exec -- <command> [args...]` |
| `tinx --workspace <ws> -- ...` | Same shortcut, but against the explicitly selected workspace |

### 5.4 Workspace selection precedence

For commands that operate on the **selected workspace**, selection MUST be resolved in this order:

1. Explicit `--workspace` / `-w`
2. Workspace discovery from the current directory upward
3. Persisted active workspace in the global home

Commands that use selected-workspace resolution:

1. `status`
2. `shell`
3. `exec`
4. Root `--`
5. `workspace current`
6. `provider add`
7. `provider remove`
8. `provider update`
9. `add`
10. `remove`
11. `update`

`list` and `provider list` do **not** use `--workspace` as an implicit scope override; they use an explicit positional scope or current/default scope rules.

### 5.5 Workspace reference forms

When a command accepts a workspace reference, the reference MAY be:

1. A registered workspace name
2. A registered workspace directory basename
3. An absolute or relative path to a workspace root
4. An absolute or relative path to a supported workspace manifest file

If a reference looks path-like and the path does not exist, the error MUST refer to a missing path rather than to a missing registered workspace.

### 5.6 Output channels and exit behavior

CLI channel rules:

1. Durable result lines, tables, and summaries MUST be written to stdout.
2. Progress events and transient fetch/materialization activity SHOULD be written to stderr.
3. Errors MUST be written as a single message to stderr.
4. Usage text MUST NOT be printed automatically on command errors.
5. Any command failure MUST terminate tinx with exit code `1`.
6. Child-process failures inside workspace execution MUST also cause tinx itself to exit with code `1`; tinx does not preserve arbitrary child exit codes as its own process exit status.

### 5.7 Inventory display conventions

Inventory rendering MUST use the following status symbol mapping:

| Status | Symbol |
| --- | --- |
| `ready` | `✓` |
| `missing` | `✗` |
| `partial` | `~` |
| `invalid` or `error` | `!` |
| any other status | `?` |

Displayed scope paths MUST follow these normalization rules:

1. If the path is within the current working directory tree, display it as `.`-relative.
2. Otherwise, if the path is within the user's home directory tree, display it as `~`-relative.
3. Otherwise, display an absolute path.
4. Long absolute paths MAY be compacted to `/first/.../last`.
5. The default scope root MUST display as `(global)`.

## 6. Provider contract

### 6.1 Accepted source forms

A provider source MUST be one of:

1. A local OCI image layout directory containing `index.json` and `oci-layout`
2. A registry reference
3. A short provider ref of the form `<namespace>/<name>` optionally followed by `:<tag>`

Short provider ref resolution rules:

1. `namespace/name` MUST resolve to `ghcr.io/namespace/name`.
2. `namespace/name:tag` MUST resolve to `ghcr.io/namespace/name:tag`.
3. Explicit registries such as `ghcr.io/org/name:tag` or `localhost/org/name:tag` MUST be left unchanged.
4. URI-style schemes such as `custom://...` MUST be rejected by install and workspace sync as unsupported provider sources.

### 6.2 Provider OCI artifact format

A packed provider artifact MUST use:

| Item | Value |
| --- | --- |
| Artifact type | `application/vnd.tinx.provider.v1` |
| Config media type | `application/vnd.tinx.provider.config.v1+json` |
| Manifest layer media type | `application/vnd.tinx.provider.manifest.v1+yaml` |
| Metadata layer media type | `application/vnd.tinx.provider.metadata.v1+json` |
| Assets layer media type | `application/vnd.tinx.provider.assets.v1+tar` |
| Binary media type pattern | `application/vnd.tinx.provider.binary.<os>.<arch>.v1` |
| OCI layout version | `1.0.0` |

The image manifest MUST contain:

1. A config descriptor carrying provider identity and high-level runtime fields
2. A manifest layer containing the original provider manifest bytes
3. A metadata layer containing derived provider metadata
4. An optional assets tar layer
5. One binary layer per declared platform

Layer annotations:

1. The top-level image manifest SHOULD annotate title as `<namespace>/<name>` and version as the provider version.
2. The manifest descriptor in `index.json` MUST annotate `org.opencontainers.image.ref.name` with the effective local tag.
3. Each binary layer MUST annotate:
   1. `org.opencontainers.image.title` with the declared platform binary path
   2. `io.tinx.platform` with `<os>/<arch>`

### 6.3 Config and metadata payload semantics

The config payload represents the immutable runtime identity needed to install the provider:

1. API version
2. kind
3. namespace
4. name
5. version
6. description
7. homepage
8. license
9. runtime
10. entrypoint

The metadata payload represents inventory-facing data:

1. namespace
2. name
3. version
4. description
5. entrypoint
6. runtime
7. sorted capability names
8. capability descriptions
9. platform summaries

### 6.4 Assets contract

If `spec.layers.assets.root` is defined and the corresponding directory exists in the artifact root at pack time:

1. The directory tree MUST be archived into a tar layer.
2. Tar entries MUST preserve relative paths under the artifact root.
3. Extraction MUST place those entries under the provider store root.
4. Tar extraction MUST reject path traversal outside the store root.

If `spec.layers.assets.root` is omitted or the directory does not exist, the provider simply has no assets layer.

### 6.5 Runtime declaration contract

Provider runtime rules:

1. Only binary runtime is supported.
2. The implementation MUST select the current OS/architecture pair when materializing a provider.
3. If the provider does not publish the current platform, runtime materialization MUST fail.
4. Capabilities do not change execution semantics. `tinx -- <alias> plan` means "execute the provider binary named by the alias with argv `plan`", not "dispatch capability `plan` internally".

### 6.6 Template variables

Provider manifest `spec.env` values and `spec.path` entries MUST support the following substitutions:

| Variable | Meaning |
| --- | --- |
| `${cwd}` | Current working directory of the tinx process at expansion time |
| `${workspace_root}` | Selected workspace root; empty outside a workspace |
| `${workspace_home}` | Selected workspace home; empty outside a workspace |
| `${provider_alias}` | Workspace alias for this provider |
| `${provider_ref}` | `<namespace>/<name>` |
| `${provider_namespace}` | Provider namespace |
| `${provider_name}` | Provider name |
| `${provider_version}` | Provider version |
| `${provider_home}` | Provider store root |
| `${provider_root}` | Alias for provider home |
| `${provider_binary}` | Materialized binary path |
| `${provider_assets}` | Provider assets root if present, otherwise provider home |

Expansion rules:

1. Unrecognized placeholders MUST remain in `${name}` form.
2. Relative `spec.path` entries after expansion MUST be resolved relative to provider home.
3. Absolute `spec.path` entries MUST remain absolute.

## 7. Workspace contract

### 7.1 Discovery

Workspace discovery MUST:

1. Start from the provided directory
2. Walk upward toward the filesystem root
3. At each level, test the supported manifest filenames in the order listed in this specification
4. Stop on the first document that parses as a valid workspace
5. Ignore provider manifests encountered during this walk

### 7.2 Registration

The global home MUST persist:

1. A map of known workspace names to absolute roots
2. The active workspace root

Registration behavior:

1. `init` and `workspace create` MUST register the created/materialized workspace.
2. `workspace use` and `use` MUST register the selected workspace if not already present.
3. Registration MUST store both the display name and, when different, the workspace directory basename.
4. `workspace delete` MUST remove all registrations pointing at the deleted workspace root.

### 7.3 Missing workspaces

If a registered workspace root no longer exists:

1. It MUST remain representable as a **missing** workspace.
2. It MAY still remain the active workspace until explicitly deleted.
3. `workspace use` or any selected-workspace command targeting it MUST fail with an error instructing the user to run `tinx workspace delete`.
4. `workspace delete` on a missing workspace MUST unregister it without trying to remove manifest or runtime files.

### 7.4 Provider declaration and locking

Workspace provider resolution rules:

1. Each declared alias points to one provider source plus an optional `plainHTTP` flag.
2. Sync MUST reconcile the declared provider map into:
   1. Workspace alias-to-version mapping in `.workspace/config.yaml`
   2. Provider metadata under `.workspace/providers/`
   3. `tinx.lock`
3. Lock entries MUST record the actual installed version and the resolved source used to install it.

### 7.5 Untagged remote pinning

For untagged remote registry sources:

1. The first successful sync MUST resolve the source to an immutable digest-backed ref.
2. That digest-backed ref MUST be stored in `tinx.lock.resolved`.
3. Subsequent syncs MUST continue using the stored resolved ref until an explicit provider update occurs.

For already pinned or tagged sources:

1. The declared source itself remains the effective install source.
2. Mutable tags such as `latest` are therefore re-resolved according to normal cache rules and are not protected by untagged-lock pinning.

### 7.6 Shell environment contract

The workspace shell environment consists of:

1. A generated env file at `.workspace/env`
2. A generated path file at `.workspace/path`
3. A shim directory at `.workspace/bin`
4. In-memory environment and PATH entries used to launch shells and commands

Base workspace environment variables:

| Variable | Value |
| --- | --- |
| `TINX_HOME` | Workspace home path |
| `TINX_WORKSPACE_ROOT` | Workspace root path |
| `TINX_WORKSPACE_HOME` | Workspace home path |
| `TINX_WORKSPACE_ENV_FILE` | Path to `.workspace/env` |
| `TINX_WORKSPACE_PATH_FILE` | Path to `.workspace/path` |
| `TINX_WORKSPACE_PROVIDERS` | Path to `.workspace/providers` |

Alias-scoped workspace variables:

| Variable pattern | Meaning |
| --- | --- |
| `TINX_PROVIDER_<SANITIZED_ALIAS>_REF` | Provider ref |
| `TINX_PROVIDER_<SANITIZED_ALIAS>_HOME` | Provider store root |
| `TINX_PROVIDER_<SANITIZED_ALIAS>_BINARY` | Materialized binary path |

Alias sanitization rules:

1. Uppercase the alias
2. Replace any non-alphanumeric character with `_`
3. Trim leading and trailing `_`

### 7.7 Environment merge semantics

Provider-contributed environment variables MUST be merged in lexicographic alias order.

Rules:

1. If two providers contribute the same key with the same value, the merge is allowed.
2. If two providers contribute the same key with different values, shell environment construction MUST fail.
3. Provider-contributed environment values MAY override ambient process environment values when the shell or command is launched.

### 7.8 PATH semantics

Workspace PATH construction MUST:

1. Start with the workspace shim directory
2. Append provider-contributed path entries in lexicographic alias order
3. Deduplicate path entries while preserving first occurrence
4. Prepend the resulting path list to the existing ambient `PATH` when launching a shell or command

### 7.9 Working directory semantics

When launching a workspace shell or command:

1. If the caller's current directory is inside the selected workspace root, that current directory MUST be preserved.
2. Otherwise, the process working directory MUST be the workspace root.

## 8. Execution lifecycle

### 8.1 Workspace creation lifecycle

`init` / `workspace create` lifecycle:

1. Parse the target as either a directory target or an existing workspace manifest file.
2. Build or load a workspace configuration.
3. Save the normalized workspace manifest to `<workspace-root>/tinx.yaml`.
4. Sync declared providers into `.workspace/` and the shared store.
5. Register the workspace in the global home.
6. Set the workspace as active.

### 8.2 Workspace provider reconciliation lifecycle

Workspace sync lifecycle:

1. Normalize and validate the workspace manifest.
2. Ensure the workspace home exists.
3. Load any prior lock file.
4. For each alias, determine the effective install source:
   1. Local OCI layout path, if applicable
   2. Locked resolved digest, for eligible untagged remote refs
   3. Declared source, otherwise
5. Install or refresh provider metadata into the workspace home.
6. Persist alias-to-provider-key mappings in `.workspace/config.yaml`.
7. Persist `tinx.lock`.

### 8.3 Workspace execution lifecycle

Workspace execution lifecycle:

1. Resolve the selected workspace.
2. Sync the workspace.
3. Build the shell environment:
   1. Reset `.workspace/bin`
   2. Materialize provider runtimes as needed
   3. Merge provider env and path contributions
   4. Write shims, `.workspace/env`, and `.workspace/path`
4. Launch either:
   1. An interactive shell, or
   2. A one-shot command

### 8.4 Standalone install lifecycle

Standalone install lifecycle:

1. Resolve the activation home:
   1. Explicit/global home if overridden
   2. Workspace home if running inside a workspace and no override is present
   3. Otherwise the global home
2. Resolve the source:
   1. Local OCI layout, or
   2. Registry reference
3. Install metadata into the activation home.
4. Cache the OCI layout into the store home.
5. Optionally record an alias in the activation home.

### 8.5 Release lifecycle

Release lifecycle:

1. Load and validate the provider manifest.
2. Build binaries unless `--skip-build` is set.
3. Stage assets into the dist tree if an assets root exists.
4. Pack the dist tree into an OCI layout.
5. Optionally push the OCI layout to a registry reference.

## 9. Filesystem layout

### 9.1 Global home

```text
<global-home>/
  config.yaml
  providers/
    <namespace>/
      <name>/
        <version>/
          metadata.json
          install.json
  store/
    <store-id>/
      tinx.yaml
      provider-metadata.json
      oci/
        index.json
        oci-layout
        blobs/
          sha256/
            <digest>
      bin/
        <os>/
          <arch>/
            <entrypoint>
```

### 9.2 Workspace root

```text
<workspace-root>/
  tinx.yaml
  tinx.lock
  .workspace/
    config.yaml
    env
    path
    bin/
      <alias>
    providers/
      <namespace>/
        <name>/
          <version>/
            metadata.json
            install.json
```

### 9.3 File semantics

| File | Meaning |
| --- | --- |
| `tinx.yaml` | Canonical workspace manifest |
| `tinx.lock` | Resolved workspace provider set |
| `.workspace/config.yaml` | Workspace alias map |
| `.workspace/env` | Shell-sourceable environment exports generated by tinx |
| `.workspace/path` | Generated path entries, one per line |
| `.workspace/bin/<alias>` | Generated shim for a provider alias |
| `providers/.../metadata.json` | Installed provider metadata for this activation home |
| `providers/.../install.json` | Install source audit record for this activation home |

## 10. Cache behavior

### 10.1 Cache tiers

tinx has two cache tiers:

| Tier | Scope | Content |
| --- | --- | --- |
| **Activation cache** | Per activation home | Provider metadata, alias maps, install source records |
| **Shared store** | Typically global home | Cached OCI layouts, cached manifest bytes, cached derived metadata, materialized binaries, extracted assets |

### 10.2 Store identity

Each store entry MUST be keyed by a deterministic store identifier derived from:

1. Provider namespace
2. Provider name
3. Provider version
4. OCI image-manifest digest

This design allows:

1. Shared store reuse across multiple workspaces
2. Shared store reuse across repeated installs of the same immutable provider artifact
3. Different versions or different manifests of the same provider to coexist

### 10.3 Local OCI layout ingestion

Installing from a local OCI layout MUST:

1. Read the layout from the source path
2. Copy it into the store
3. Cache the provider manifest and derived metadata
4. Persist activation-home metadata that points at the cached layout, not the original layout

The original local layout is therefore not required after installation completes.

### 10.4 Remote install modes

Remote install modes:

| Mode | Intended caller | Download scope |
| --- | --- | --- |
| **Metadata-only** | `install` against a registry ref | OCI manifest, config, provider manifest layer, provider metadata layer |
| **Full runtime** | Workspace sync | Metadata plus binary and asset blobs |

Consequences:

1. A metadata-only install MAY not have enough local content to materialize the runtime immediately.
2. A full runtime install MUST guarantee that the current activation home references a store capable of runtime extraction, subject to later blob corruption or deletion.

### 10.5 Remote cache hits

A remote install MAY reuse previously installed metadata in the same activation home when either:

1. The requested ref matches the stored resolved ref exactly, or
2. The requested repository matches and the requested tag equals the installed provider version

Runtime-capable reuse additionally requires the store to contain all needed runtime blobs.

### 10.6 Lazy runtime hydration

When shell construction requires a provider runtime:

1. tinx MUST first check whether the current platform binary already exists in the store.
2. If not, tinx MUST try to extract it from the cached OCI layout.
3. If the binary or assets blob is missing locally and the provider metadata still carries a remote source ref, tinx MUST hydrate the store from the remote registry and retry once.
4. If hydration is unavailable or still insufficient, runtime materialization MUST fail.

### 10.7 Cache cleanup

Current cleanup rules:

1. Removing a provider from a workspace MAY delete the provider version directory from that workspace home if no remaining alias references it there.
2. Updating a provider in a workspace deletes the workspace-home copy for the selected aliases before re-syncing.
3. Deleting a workspace removes the workspace home, manifest, and lock file.
4. Shared store entries in the global home are **not** garbage-collected automatically.
5. There is no built-in store eviction or TTL policy.

## 11. Plugin/runtime model

tinx is **not** an in-process plugin host.

The runtime model is:

1. Providers are external executables.
2. Workspaces expose providers through generated command shims placed on `PATH`.
3. Providers can invoke other providers simply by calling their aliases, because those aliases resolve through the workspace shim directory.
4. Provider capabilities are descriptive metadata only.
5. tinx does not interpret provider subcommands after command dispatch; argv is passed through unchanged.

### 11.1 Interactive shell model

`shell` and root `--` without a command MUST:

1. Use `$SHELL` if set, otherwise `/bin/sh`
2. Add `-i` for common interactive POSIX shells
3. Launch the shell with the prepared workspace environment and PATH

### 11.2 One-shot command model

`exec`, `use <workspace> -- ...`, and root `-- <command...>` MUST:

1. Resolve the first token against the prepared PATH unless it already contains a path separator
2. Execute the command directly, without wrapping it in another shell
3. Pass remaining tokens as argv verbatim

### 11.3 Environment inheritance model

Launched shells and commands MUST inherit the ambient process environment, then apply:

1. Workspace base variables
2. Provider-contributed env variables
3. Alias-scoped `TINX_PROVIDER_*` variables
4. PATH prepending based on shim and provider path entries

There is no isolation sandbox. Workspace execution inherits ambient credentials, files, and environment unless explicitly overridden by the caller.

## 12. Error handling model

### 12.1 General rules

1. Errors are explicit and fatal.
2. There are no silent success-shaped fallbacks for invalid input.
3. Usage banners are suppressed on command errors.
4. The root process emits the error string and exits with code `1`.

### 12.2 Validation errors

The implementation MUST reject:

1. Unsupported provider `apiVersion`
2. Unsupported provider `kind`
3. Unsupported provider runtime
4. Missing provider identity fields
5. Missing provider entrypoint
6. Missing provider platforms
7. Duplicate provider platform pairs
8. Reserved `TINX_` env keys in provider manifests
9. Empty workspace aliases
10. Workspace aliases containing `/`
11. Empty workspace provider sources

### 12.3 Selection and discovery errors

The implementation MUST error when:

1. A selected workspace cannot be found
2. A path-like workspace reference does not exist
3. A workspace root exists but no workspace manifest can be found beneath it
4. The selected workspace is registered but missing on disk
5. A provider mutation command is executed with no selected workspace

### 12.4 Source and registry errors

The implementation MUST error when:

1. A provider source uses an unsupported scheme
2. A local OCI layout path cannot be opened
3. A requested local OCI tag is not present
4. Registry pull or push fails
5. Registry authentication fails and anonymous access is insufficient

### 12.5 Runtime errors

The implementation MUST error when:

1. No binary is published for the current platform
2. The binary layer descriptor is missing
3. The cached assets tar attempts path traversal
4. Multiple providers contribute the same env key with different values
5. The selected command cannot be found in PATH

### 12.6 Compatibility errors

The implementation MUST preserve the following compatibility behavior:

1. `install ... -- <command...>` MUST fail with an explanation that install no longer executes commands.
2. `run ...` MUST always fail with a migration message pointing users to workspace execution.
3. Unknown top-level commands MUST surface a normal command-not-found error.

## 13. External integrations

### 13.1 OCI registry integration

The registry contract assumes ORAS-compatible OCI semantics:

1. Pulling by repository plus tag or digest-like token
2. Pushing from a local OCI layout into a remote repository
3. Content-addressed blob copying

### 13.2 Credential resolution

Registry credentials SHOULD be resolved in this order:

1. Docker credential store / Docker config
2. `TINX_REGISTRY_USERNAME` + `TINX_REGISTRY_PASSWORD`
3. `ORAS_USERNAME` + `ORAS_PASSWORD`
4. `GITHUB_ACTOR` + `GITHUB_TOKEN` for `ghcr.io`
5. Anonymous access

### 13.3 Build tool integration

The build contract assumes:

1. A Go toolchain is available for direct builds
2. `goreleaser` is available on `PATH` when delegated builds are requested

Direct build mode MUST:

1. Cross-compile for each declared provider platform
2. Set `CGO_ENABLED=0`
3. Apply version ldflags

Delegated build mode MUST:

1. Prefer an explicit GoReleaser config path when provided
2. Otherwise prefer `.goreleaser.yml` / `.goreleaser.yaml`
3. Otherwise generate `.goreleaser.tinx.generated.yaml`

### 13.4 Shell integration

The runtime assumes a POSIX-like shell model:

1. Shims are shell scripts
2. Interactive shell launching depends on `$SHELL` or `/bin/sh`
3. PATH semantics are POSIX path-list semantics

## 14. State transitions

### 14.1 Workspace state machine

| From | Event | To | Notes |
| --- | --- | --- | --- |
| `unregistered` | `init` / `workspace create` | `registered-ready-active` | Manifest saved, workspace synced, active workspace set |
| `unregistered` | `workspace use <existing>` | `registered-ready-active` | Existing workspace is registered and selected |
| `registered-ready` | `workspace use` | `registered-ready-active` | Active marker moves to this workspace |
| `registered-ready-active` | external root deletion | `registered-missing-active` | Missing state persists until explicit delete |
| `registered-ready` | external root deletion | `registered-missing` | Still listed in workspace inventory |
| `registered-missing(-active)` | `workspace delete` | `unregistered` | Registration removed; active cleared if applicable |
| `registered-ready(-active)` | `workspace delete` | `unregistered` | Manifest, lock, and workspace home removed |

### 14.2 Provider state machine per activation home

| From | Event | To | Notes |
| --- | --- | --- | --- |
| `undeclared` | add to workspace manifest | `declared` | Declaration only |
| `declared` | successful workspace sync | `installed-full` | Activation metadata written; shared store cached |
| `undeclared` | standalone `install` from registry | `installed-metadata-only` or `installed-full` | Metadata-only for remote install; full for local layout |
| `installed-metadata-only` | first workspace runtime demand with remote ref available | `installed-full` | Store is hydrated and runtime blobs become available |
| `installed-full` | first runtime extraction for current platform | `materialized` | Binary extracted to store root |
| `materialized` | later shell build | `materialized` | Reused from cache |
| `installed-*` / `materialized` | workspace provider remove | `removed-from-activation-home` | Shared store may remain |
| `installed-*` / `materialized` | workspace provider update | `refreshing` -> `installed-full/materialized` | Lock and activation metadata rewritten |

### 14.3 Untagged remote refresh transition

| Source form | Normal sync | `provider update` |
| --- | --- | --- |
| Untagged remote ref | Reuses `tinx.lock.resolved` digest | Re-resolves source and records a new digest if upstream changed |
| Version-tagged remote ref | Uses the declared tag | Reinstalls from the declared tag |
| Digest ref | Uses the declared digest | Reinstalls from the same digest |
| Local OCI layout | Uses the declared layout path | Re-reads the layout path |

## 15. Sequence diagrams (text)

### 15.1 Release and push

```text
User -> tinx: release [--manifest] [--dist] [--output] [--push]
tinx -> Provider manifest: load and validate
tinx -> Builder: build binaries (unless --skip-build)
tinx -> Dist tree: stage optional assets
tinx -> OCI packager: write layout, blobs, index, tag
alt --push provided
  tinx -> Registry: push local layout to remote reference
  Registry -> tinx: push result
end
tinx -> User: stdout "pushed ..." (optional), "released <ref>@<tag> -> <dir>"
```

### 15.2 Standalone install from registry

```text
User -> tinx: install <ref> [as <alias>]
tinx -> Activation home selector: resolve current/default scope
tinx -> Registry resolver: expand short ref if needed
tinx -> Activation home: check same-home install cache
alt cache hit
  tinx -> Activation home config: update alias if requested
else cache miss
  tinx -> Registry: pull metadata-only blobs
  tinx -> Shared store: cache OCI layout subset and manifest artifacts
  tinx -> Activation home: write metadata.json and install.json
  tinx -> Activation home config: write alias if requested
end
tinx -> User: stdout "installed <namespace>/<name>@<version>"
```

### 15.3 Workspace add and sync

```text
User -> tinx: provider add <source> [as <alias>]
tinx -> Selected workspace resolver: find workspace
tinx -> Workspace manifest: add alias declaration
tinx -> Workspace reconciler: sync workspace
loop for each alias
  Workspace reconciler -> Source resolver: derive effective install source
  Workspace reconciler -> Shared store / registry / local layout: install provider
  Workspace reconciler -> Workspace home: write provider metadata
end
Workspace reconciler -> Workspace home config: write alias map
Workspace reconciler -> Workspace root: write tinx.lock
tinx -> User: stdout "added provider ...", manifest path, workspace home
```

### 15.4 Workspace command execution

```text
User -> tinx: exec -- <command...>   (or: tinx -- <command...>)
tinx -> Selected workspace resolver: resolve workspace
tinx -> Workspace reconciler: sync workspace
tinx -> Shell composer: reset shim dir
loop for each alias in sorted order
  Shell composer -> Workspace home: load provider metadata
  Shell composer -> Shared store: materialize current-platform runtime if needed
  Shell composer -> Provider manifest: expand env/path templates
  Shell composer -> Shell environment: merge env and path
  Shell composer -> Workspace home: write alias shim
end
Shell composer -> Workspace home: write env and path artifacts
tinx -> OS process launcher: exec command with augmented env and PATH
OS process launcher -> tinx: exit status
tinx -> User: child stdout/stderr, overall exit 0 or 1
```

### 15.5 Untagged provider update

```text
User -> tinx: provider update <alias>
tinx -> Selected workspace resolver: resolve workspace
tinx -> Workspace home: remove activation-home cache for selected alias
tinx -> Workspace reconciler: sync workspace with refresh set
Workspace reconciler -> Registry: re-resolve untagged source
Registry -> Workspace reconciler: new digest-backed resolution
Workspace reconciler -> Workspace root: rewrite tinx.lock with new resolved digest
tinx -> User: stdout "updated providers: <alias>"
```

## 16. Behavior specification per command

### 16.1 `init`

**Syntax**

```text
tinx init [workspace-or-config] [-p <provider-source> [as <alias>]]...
```

Behavior:

1. If no target is provided, the target defaults to the current directory.
2. If the target is an existing YAML file and no provider flags are supplied, that file is treated as a workspace manifest input and materialized to `<dir>/tinx.yaml`.
3. If provider flags are supplied, the target MUST be treated as a directory target, not as an existing workspace config file.
4. Repeated `-p` / `--provider` flags declare initial providers.
5. If an alias is omitted, tinx derives one from the source.
6. The resulting workspace is saved, synced, registered, and set active.

Outputs:

1. `initialized workspace <name>`
2. `active workspace: <name>`
3. `manifest: <path>`
4. `home: <path>`

### 16.2 `status`

**Syntax**

```text
tinx status [--verbose | --short]
```

Behavior:

1. If a selected workspace exists, tinx syncs it and reports workspace-local status.
2. If no selected workspace exists, tinx reports the default scope.
3. `--short` renders a one-line summary.
4. `--verbose` includes generated env/path artifact locations and path entries.

Short format:

```text
<workspace-or-none> | <N> providers | shims <active|inactive>
```

### 16.3 `workspace list`

**Syntax**

```text
tinx workspace list [--short] [--ready] [--missing] [--active]
```

Behavior:

1. Lists all registered workspaces plus the `default` scope.
2. Sorts the active non-default workspace first, then other non-default workspaces alphabetically, then `default`.
3. `--ready` and `--missing` are mutually exclusive.
4. `--active` filters to the active workspace only.
5. `--short` prints one line per scope prefixed with `*` for active or a space for inactive.

Default table columns:

```text
NAME  ACTIVE  STATUS  ROOT
```

Summary lines:

1. `<N> workspaces (...)`
2. `Active workspace: <name-or-none>`

### 16.4 `workspace create`

`workspace create` is semantically identical to `init`.

### 16.5 `workspace use`

**Syntax**

```text
tinx workspace use <workspace> [-- command...]
```

Behavior:

1. Resolves the workspace reference.
2. Fails if the workspace is missing on disk.
3. Syncs the workspace.
4. Registers the workspace if needed.
5. Sets the workspace as active.
6. If no command is supplied, prints the active workspace and root.
7. If a command is supplied, executes it inside the workspace environment **after** setting the workspace active.

Side effect ordering matters:

1. Successful sync happens before the active workspace changes.
2. The active workspace changes before the optional command is executed.
3. If the optional command fails, the workspace remains active.

### 16.6 `workspace current`

**Syntax**

```text
tinx workspace current
```

Behavior:

1. Resolves the selected workspace using normal selected-workspace precedence.
2. If none exists, prints `workspace: none`.
3. If a workspace exists, prints:
   1. `workspace: <name>`
   2. `root: <path>`
   3. `status: missing` if the root is missing, otherwise `home: <workspace-home>`

### 16.7 `workspace delete`

**Syntax**

```text
tinx workspace delete <workspace>
```

Behavior:

1. Unregisters the workspace from the global home.
2. Clears active-workspace state if it points at the deleted workspace.
3. If the workspace root is missing, prints an unregister message and stops.
4. If the workspace exists, removes:
   1. `<workspace>/.workspace`
   2. `<workspace>/tinx.lock`
   3. `<workspace>/tinx.yaml`
5. If the workspace root becomes empty afterward, tinx removes the directory.
6. Shared store entries in the global home remain intact.

### 16.8 `workspace activate`

This hidden deprecated command is behaviorally equivalent to `workspace use`.

### 16.9 `provider list`

**Syntax**

```text
tinx provider list [current|default|global|<workspace-ref>]
```

Behavior:

1. With no argument or `current`, list the current workspace if one is selected; otherwise list the default scope.
2. With `default` or `global`, list the default scope.
3. With a workspace reference, list that workspace scope.
4. The command does not implicitly honor `--workspace`.

Output header:

1. `Scope: <name>`
2. `Root: <path-or-(global)>`
3. `Status: <symbol> <status>`
4. Optional `Detail: ...` for non-ready scopes

Ready-scope table columns:

```text
NAME  STATUS  PROVIDER  VERSION
```

If no providers are installed, the command prints `no providers installed`.

### 16.10 `provider add`

**Syntax**

```text
tinx provider add <provider> [as <alias>] [--plain-http]
```

Behavior:

1. Requires a selected workspace.
2. Accepts either:
   1. `<provider>`
   2. `<provider> as <alias>`
3. If the alias is omitted, tinx derives it from the source.
4. If the alias already exists with identical source and `plainHTTP`, the command succeeds with an informational message and does nothing else.
5. If the alias already exists with different settings, the command fails.
6. On success, tinx mutates the workspace manifest, syncs the workspace, and updates lock and alias state.

Outputs:

1. `added provider <alias> -> <source>`
2. `manifest: <path>`
3. `home: <path>`

### 16.11 `provider remove`

**Syntax**

```text
tinx provider remove <provider-or-alias>
```

Selection rules:

1. Exact workspace alias match wins.
2. Otherwise tinx matches against:
   1. The declared source string
   2. The normalized declared source
   3. The installed provider key `<namespace>/<name>@<version>`
   4. The installed provider ref `<namespace>/<name>`
3. If multiple aliases match, the command fails as ambiguous and lists the candidate aliases.

Behavior:

1. Removes the alias from the workspace manifest.
2. If no remaining alias in that workspace references the same installed provider key, deletes the provider version directory from the workspace home.
3. Syncs the workspace afterward.
4. Does not garbage-collect the shared store.

Outputs:

1. `removed provider <alias>`
2. `manifest: <path>`
3. `home: <path>`

### 16.12 `provider update`

**Syntax**

```text
tinx provider update [provider-or-alias...]
```

Behavior:

1. Requires a selected workspace.
2. With no selectors, refreshes all providers in the workspace.
3. With selectors, resolves them using the same matching rules as remove.
4. Deletes the workspace-home activation cache for the selected aliases.
5. Re-syncs the workspace with those aliases marked for refresh.
6. Rewrites lock entries and alias mappings as needed.

Outputs:

1. `updated providers: <comma-separated-aliases>`
2. `home: <path>`

If no providers are declared in the workspace, the command prints `workspace <name> has no providers to update`.

### 16.13 Compatibility aliases: `add`, `remove`, `update`

These top-level commands are behaviorally identical to:

1. `add` -> `provider add`
2. `remove` -> `provider remove`
3. `update` -> `provider update`

### 16.14 Compatibility alias: `use`

`tinx use <workspace> [-- command...]` is behaviorally identical to `tinx workspace use <workspace> [-- command...]`.

### 16.15 `list`

**Syntax**

```text
tinx list [scope]
tinx list workspaces
tinx list providers [scope]
```

Behavior:

1. `tinx list [scope]` is a compatibility entry point for provider inventory.
2. `tinx list workspaces` is equivalent to the unfiltered workspace inventory view.
3. `tinx list providers [scope]` is equivalent to `tinx provider list [scope]`.

### 16.16 `shell`

**Syntax**

```text
tinx shell
```

Behavior:

1. Requires a selected workspace.
2. Syncs the workspace.
3. Builds the workspace shell environment and generated artifacts.
4. Launches an interactive shell.

### 16.17 `exec`

**Syntax**

```text
tinx exec [--] <command> [args...]
```

Behavior:

1. Requires a selected workspace.
2. Syncs the workspace.
3. Builds the workspace shell environment and generated artifacts.
4. Executes the command directly with the prepared environment.
5. If the first post-parse token is `--`, it is discarded before command execution.

### 16.18 Root compatibility execution

**Syntax**

```text
tinx --                      # interactive shell
tinx -- <command> [args...]  # one-shot command
```

Behavior is identical to `shell` and `exec`, respectively.

### 16.19 `install`

**Syntax**

```text
tinx install <ref> [as <alias>] [--source <oci-layout>] [--tag <tag>] [--plain-http]
tinx install <alias> <ref> [--source <oci-layout>] [--tag <tag>] [--plain-http]
```

Behavior:

1. `install` installs metadata into the current activation scope; it does not execute providers.
2. When `--source` is omitted, the install target is treated as a registry source after short-ref resolution.
3. When `--source` is provided:
   1. The source MUST be a local OCI layout
   2. The install target MUST be a logical provider ref `<namespace>/<name>`
   3. The installed provider identity MUST match the requested ref
4. If `install` is executed inside a workspace and no home override is supplied, the activation home becomes that workspace home while the shared store remains the global home.
5. Remote installs are metadata-only by default.
6. Local-layout installs cache the full layout.

Outputs:

1. `installed <namespace>/<name>@<version>`

Compatibility error:

1. Any `-- <command...>` suffix MUST be rejected with a migration error.

### 16.20 `run`

`run` is removed.

Behavior:

1. The command MUST always fail.
2. The error MUST instruct the user to add the provider to a workspace and run it through `tinx -- ...`.
3. When arguments are present, the message SHOULD include the suggested replacement command.

### 16.21 `pack`

**Syntax**

```text
tinx pack [--manifest tinx.yaml] [--artifact-root <dir>] [--output oci] [--tag <tag>]
```

Behavior:

1. Loads the provider manifest.
2. Determines the artifact root:
   1. `--artifact-root`, if provided
   2. otherwise the manifest directory
3. Deletes any existing output directory before writing the OCI layout.
4. Packages the manifest, derived metadata, optional assets, and all declared binaries.
5. Uses the provider version as the tag unless `--tag` is supplied.

Output:

1. `packed <provider-ref>@<tag> -> <layout-dir>`

### 16.22 `release`

**Syntax**

```text
tinx release [--manifest tinx.yaml] [--dist dist] [--output oci] [--main <go-main-pkg>] [--skip-build] [--tag <tag>] [--push <oci-ref>] [--plain-http] [--delegate-goreleaser|--delegate-gorelaser] [--goreleaser-config <path>]
```

Behavior:

1. Loads the provider manifest.
2. Determines the module root as the manifest directory.
3. Unless `--skip-build` is set:
   1. Deletes the dist directory
   2. Builds binaries directly, or
   3. Delegates to GoReleaser when requested
4. Stages assets from the module root into the dist tree when an assets root exists.
5. Packs the dist tree into an OCI layout.
6. If `--push` is set, pushes the local layout to the remote reference.

Direct build main-package selection:

1. `--main`, if provided
2. `./cmd/<provider-name>`, if it exists
3. The sole subdirectory under `./cmd`, if there is exactly one
4. `.`, otherwise

Outputs:

1. Optional `pushed <remote-ref>`
2. `released <provider-ref>@<tag> -> <layout-dir>`

### 16.23 `version`

**Syntax**

```text
tinx version
```

Output:

```text
tinx <version>
```

If a commit identifier is embedded into the build, the root-version string MAY render as `tinx <version> (<commit>)`.

## 17. Non-functional requirements

1. **Platform assumptions:** The runtime model is POSIX-oriented and assumes macOS or Linux semantics for shell launching, file modes, and PATH handling.
2. **Deterministic store identity:** The same provider identity plus OCI manifest digest MUST yield the same store identifier.
3. **Deterministic capability ordering:** Capability names in derived metadata MUST be sorted.
4. **Deterministic assets archiving:** Asset tar entries SHOULD be written in sorted order to improve reproducibility.
5. **Lazy materialization:** Provider binaries and assets SHOULD be extracted only when required for runtime execution.
6. **Shared-store efficiency:** Multiple workspaces SHOULD reuse the same cached OCI store entry when they resolve to the same provider artifact.
7. **No silent conflict resolution:** Provider env conflicts within a workspace MUST fail fast.
8. **Human-oriented CLI:** Output is table- and summary-oriented; there is no JSON output mode in the current contract.
9. **Progress isolation:** Fetch/build/materialization progress SHOULD appear on stderr so stdout remains suitable for piping or inspection.
10. **No transactional multi-command state:** The file-based design does not provide cross-process locking or transactional guarantees. Concurrent writers SHOULD be treated as undefined behavior unless a reimplementation adds explicit coordination.
11. **Trust boundary:** Providers execute as ordinary child processes with inherited ambient environment and filesystem access; tinx is an orchestrator, not a sandbox.
12. **Security posture:** HTTPS registries are the safe default. Plain HTTP MUST require explicit opt-in. Digest-pinned refs and lock-backed untagged resolution SHOULD be preserved for reproducibility.
13. **Compatibility posture:** Grouped `workspace` and `provider` commands are the preferred interface, but compatibility aliases and migration errors are part of the user-visible contract.
