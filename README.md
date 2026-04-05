# tinx

OCI-native provider runtime, workspace shell, and packager.

`tinx` is a CLI for building, packaging, installing, and running providers distributed as OCI artifacts.
It is designed for provider-based workflows where versioned provider binaries can be composed into a
workspace-local shell environment and invoked like normal commands.

## Project Status

- **Maturity:** Active development
- **API/CLI stability:** Evolving; expect incremental improvements
- **Target users:** Platform and DevOps teams building provider-driven workflows

## Why tinx

- **OCI-native distribution** for providers and metadata
- **Lazy runtime materialization** (fetch platform binary only when needed)
- **Workspace-local shell UX** with provider shims on `PATH`
- **Portable packaging** via OCI layout (`pack`) and registry push (`release --push`)

## Architecture Highlights

- `tinx init` creates a workspace in the current or target directory and selects it immediately
- `tinx use` or `tinx workspace use` selects the current workspace scope
- `tinx provider add` mutates a workspace manifest and syncs providers into `.workspace/`
- `tinx status` shows the current workspace, providers, shims, and generated environment artifacts
- `tinx --` rebuilds a workspace shell environment and runs any command, or launches an interactive shell
- `tinx install` installs provider metadata from registry or local OCI layout
- `tinx pack` packages a provider into an OCI image layout
- `tinx release` builds, packages, and optionally pushes artifacts

See `tinx_technical_specification.md` for deeper contract and media-type details.

## Prerequisites

- macOS or Linux
- One of:
  - downloaded `tinx` binary
  - Go 1.24+ (for source install)
- OCI registry access (only when installing/pushing remote providers)

## Installation

### Option 1: Install from source

```bash
go install github.com/sourceplane/tinx/cmd/tinx@latest
```

### Option 2: Install from GitHub Releases (script)

```bash
curl -fsSL https://raw.githubusercontent.com/sourceplane/tinx/main/install.sh | bash
```

### Verify

```bash
tinx version
```

## Quick Start

### 1) Build tinx locally

```bash
make build
```

### 2) Package example provider (local OCI layout)

```bash
make release-example
```

### 3) Install example provider into local tinx home

```bash
make install-example
```

### 4) Run provider capability

```bash
make run-example
```

## Workspaces

Workspaces are the preferred UX for multi-provider flows. A workspace installs providers into a workspace-local `.workspace` runtime, writes shell artifacts on each `tinx -- ...` invocation, exposes provider shims on `PATH`, and lets providers invoke each other naturally.

Create a workspace in the current directory and switch tinx to it immediately:

```bash
tinx init
```

Create a named workspace elsewhere:

```bash
tinx init my-workspace
```

Switch to a workspace later:

```bash
tinx use my-workspace
```

Add providers to the active workspace:

```bash
tinx provider add core/node as node
tinx provider add sourceplane/lite-ci as lite-ci
```

Short aliases are available for a shorter flow:

```bash
tinx ws use my-workspace
tinx p add core/node as node
tinx p add sourceplane/lite-ci as lite-ci
```

Run any shell command through the workspace environment:

```bash
tinx -- node build
tinx -- lite-ci plan
tinx -- lite-ci plan -- node build
```

Launch an interactive shell with the workspace environment loaded:

```bash
tinx --
```

Target a workspace explicitly without selecting it globally:

```bash
tinx --workspace my-workspace -- node build
```

Materialize a workspace from a manifest file:

```bash
tinx init providers.tx.yaml
```

Example workspace manifest:

```yaml
kind: workspace
workspace: dev

providers:
  node: core/node
  lite-ci: sourceplane/lite-ci
  docker: tinx/docker
  kubectl: tinx/kubectl
```

The normalized workspace manifest is written to `tinx.yaml`, the resolved install set is written to `tinx.lock`, and runtime state is stored under:

- `<workspace>/.workspace/env`
- `<workspace>/.workspace/path`
- `<workspace>/.workspace/providers/`

Inspect the current runtime state and installed inventory:

```bash
tinx status
tinx status --short
tinx status --verbose
tinx ws current
tinx ws list
tinx ws list --short
tinx ws list --ready
tinx ws list --missing
tinx ws list --active
tinx p list
tinx p list default
```

Workspace inventory uses a compact table with activity markers, status symbols, and shortened roots:

```text
NAME             ACTIVE   STATUS     ROOT
-----------------------------------------
my-space         *        ✓ ready    ./my-space
my-workspace              ✓ ready    ./my-workspace
dev                       ✗ missing  /tmp/.../dev
default                   ✓ ready    (global)

4 workspaces (3 ready, 1 missing)
Active workspace: my-space
```

For quick checks or scripting:

```text
* my-space
  my-workspace
  default
```

Provider inventory is similarly compact:

```text
Scope: my-space
Root: ./my-space
Status: ✓ ready

NAME      STATUS     PROVIDER             VERSION
-----------------------------------------------
lite-ci   ✓ ready    sourceplane/lite-ci  v0.2.25

1 provider (1 ready)
```

Default status output is intentionally short:

```text
tinx workspace: my-space
path: ./my-space
shims: active

providers:
  lite-ci  sourceplane/lite-ci  v0.2.25  ready
```

For a one-line summary:

```text
my-space | 1 providers | shims active
```

Provider manifests can also contribute workspace session variables and extra PATH entries:

```yaml
spec:
  env:
    LITECI_CONFIG: ${provider_assets}/config
  path:
    - tools/bin
```

Supported interpolation keys include `${provider_ref}`, `${provider_home}`, `${provider_assets}`, `${provider_binary}`, `${workspace_root}`, and `${cwd}`.

## Standalone Install

`install` remains a low-level metadata and cache command. It does not execute providers.

Install a provider into the default tinx home:

```bash
tinx install sourceplane/lite-ci as lite-ci
```

Execution still happens through a workspace shell:

```bash
tinx use my-workspace
tinx p add sourceplane/lite-ci as lite-ci
tinx -- lite-ci plan
```

Preferred modern UX:

```bash
tinx init
tinx use dev
tinx status

tinx ws list
tinx ws list --short
tinx ws list --ready
tinx ws list --missing
tinx ws list --active
tinx ws create dev
tinx ws use dev
tinx ws current
tinx ws delete dev

tinx p list
tinx p add core/node
tinx p remove node
tinx p update
```

Legacy compatibility commands like `tinx add` still work, but the grouped `workspace` and `provider` commands are the preferred UX.

## CLI Reference

```bash
tinx init [workspace-or-config] [-p <provider-source> [as <alias>]]...
tinx use <workspace> [-- command...]
tinx status
tinx workspace list [--short] [--ready] [--missing] [--active]
tinx workspace create [workspace-or-config] [-p <provider-source> [as <alias>]]...
tinx workspace use <workspace> [-- command...]
tinx workspace current
tinx workspace delete <workspace>
tinx ws <subcommand> ...
tinx workspaces <subcommand> ...
tinx providers <subcommand> ...
tinx provider list [workspace|default]
tinx provider add <provider> [as <alias>] [--plain-http]
tinx provider remove <provider-or-alias>
tinx provider update [provider-or-alias...]
tinx p <subcommand> ...
tinx add <provider> [as <alias>] [--plain-http]
tinx [--workspace <workspace>] -- <command...>
tinx [--workspace <workspace>] --
tinx install <ref> [as <alias>] [--source <oci-layout>] [--tag <tag>] [--plain-http]
tinx install <alias> <ref> [--source <oci-layout>] [--tag <tag>] [--plain-http]
tinx -- <command...>
tinx pack [--manifest tinx.yaml] [--artifact-root <dir>] [--output oci] [--tag <tag>]
tinx release [--manifest tinx.yaml] [--dist dist] [--output oci] [--main <go-main-pkg>] [--push <oci-ref>]
tinx version
```

## Configuration

- Default runtime home: `~/.tinx` (or project-provided override)
- Workspace runtime home: `<workspace>/.workspace`
- Override runtime home per command:

```bash
tinx --tinx-home /custom/path --workspace my-workspace -- node build
```

## Security

- Prefer HTTPS registries (avoid `--plain-http` except trusted local/dev setups)
- Pin immutable references (digests) for reproducible installs
- Restrict provider trust to known namespaces/registries
- Review provider manifests and capabilities before execution

## Contributing

```bash
make tidy
make test
make test-core
```

Contribution and development details are documented in:

- `implimentaion_context_spec.md`
- `tinx_technical_specification.md`
- `TEST_EXAMPLES.md`

## CNCF Alignment Notes

`tinx` follows common cloud-native tooling conventions:

- OCI artifact model and registry interoperability
- Explicit command surface and automation-friendly CLI
- Platform-aware binary packaging for reproducible distribution
- Security-conscious defaults and clear trust boundaries

## Roadmap (High-level)

- Improved provider discovery/index integrations
- Stronger supply-chain verification workflows
- Expanded provider authoring ergonomics
