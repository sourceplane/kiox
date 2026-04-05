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

- `tinx workspace activate` selects the current workspace scope
- `tinx add` mutates a workspace manifest and syncs providers into `.workspace/`
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

Create an empty workspace:

```bash
tinx init my-workspace
```

Activate it:

```bash
tinx workspace activate my-workspace
```

Add providers to the active workspace:

```bash
tinx add core/node as node
tinx add sourceplane/lite-ci as lite-ci
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

Target a workspace explicitly without activating it globally:

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

Inspect the installed inventory across workspace and standalone scopes:

```bash
tinx list workspaces
tinx list providers my-workspace
tinx list providers default
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
tinx workspace activate my-workspace
tinx add sourceplane/lite-ci as lite-ci
tinx -- lite-ci plan
```

## CLI Reference

```bash
tinx init <workspace-or-config> [-p <provider-source> [as <alias>]]...
tinx workspace activate <workspace> [-- command...]
tinx add <provider> [as <alias>] [--plain-http]
tinx list workspaces
tinx list providers [workspace|default]
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
