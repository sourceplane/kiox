# tinx

OCI-native provider runtime and packager.

`tinx` is a CLI for building, packaging, installing, and running providers distributed as OCI artifacts.
It is designed for provider-based workflows where capabilities such as `plan`, `validate`, and `release`
are executed from versioned provider binaries.

## Project Status

- **Maturity:** Active development
- **API/CLI stability:** Evolving; expect incremental improvements
- **Target users:** Platform and DevOps teams building provider-driven workflows

## Why tinx

- **OCI-native distribution** for providers and metadata
- **Lazy runtime materialization** (fetch platform binary only when needed)
- **Alias-based UX** for ergonomic command execution
- **Portable packaging** via OCI layout (`pack`) and registry push (`release --push`)

## Architecture Highlights

- `tinx install` installs provider metadata from registry or local OCI layout
- `tinx run` resolves alias/provider and executes a capability
- `tinx run gha://owner/repo@ref` treats a GitHub composite action as a cached tinx provider
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

Workspaces are the preferred UX for multi-provider flows. A workspace installs providers into a workspace-local `.tinx` home, exposes aliases on `PATH`, and lets providers invoke each other naturally.

Create a workspace from flags:

```bash
tinx init my-workspace \
  -p core/node as node \
  -p sourceplane/lite-ci as lite-ci
```

Activate it and dispatch a command through the workspace provider set:

```bash
tinx use my-workspace
tinx -- lite-ci run plan -- node deploy
```

You can also activate and run in one step:

```bash
tinx use my-workspace -- lite-ci run plan -- node deploy
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

The normalized workspace manifest is written to `tinx.yaml`, provider state is stored under `<workspace>/.tinx`, and the resolved install set is written to `tinx.lock`.

Inspect the installed inventory across workspace and standalone scopes:

```bash
tinx list workspaces
tinx list providers my-workspace
tinx list providers default
```

## Single-Provider Dispatch

For single-provider flows, `install` and `run` can expose the provider as an ephemeral command after `--`.

Install with an alias and execute immediately:

```bash
tinx install sourceplane/lite-ci as lite-ci -- lite-ci run plan
```

Run directly from a provider reference and execute immediately:

```bash
tinx run sourceplane/lite-ci -- lite-ci run plan
```

If you omit `as <alias>`, tinx still installs the provider metadata into the default home. Use `tinx run <provider-ref> <capability>` for later invocation, or use `-- <entrypoint> ...` for one-shot dispatch.

## CLI Reference

```bash
tinx init <workspace-or-config> [-p <provider-source> [as <alias>]]...
tinx use <workspace> [-- command...]
tinx list workspaces
tinx list providers [workspace|default]
tinx install <ref> [as <alias>] [--source <oci-layout>] [--tag <tag>] [--plain-http] [-- command...]
tinx install <alias> <ref> [--source <oci-layout>] [--tag <tag>] [--plain-http]
tinx run <provider-or-alias> [capability-or-args...] [--plain-http]
tinx run <provider-ref> [--plain-http] -- <command...>
tinx run gha://<owner>/<repo>@<ref> [--input name=value]
tinx install <alias> gha://<owner>/<repo>@<ref> [--input name=value]
tinx <alias> [capability-or-args...]
tinx -- <command...>
tinx pack [--manifest tinx.yaml] [--artifact-root <dir>] [--output oci] [--tag <tag>]
tinx release [--manifest tinx.yaml] [--dist dist] [--output oci] [--main <go-main-pkg>] [--push <oci-ref>]
tinx version
```

## Experimental GitHub Actions Support

`tinx` can execute GitHub Actions through the `gha://` provider source.
The current MVP supports composite and Node-based actions, and caches the action source under the normal tinx provider home.

Example:

```bash
tinx run gha://azure/setup-helm@v4 --input version=3.18.4
```

Install-as-provider example:

```bash
tinx install helm gha://azure/setup-helm@v4 --input version=3.18.4
tinx run helm version --short
```

Current scope:

- Composite actions (`runs.using: composite`)
- Node actions (`runs.using: node20`, `node24`, etc.) executed with the system `node` on `PATH`
- Alias installs (`tinx install <alias> gha://...`) create alias-scoped providers under the normal tinx provider home
- Install-time GitHub Action inputs persist on the installed provider via `--input name=value`
- Setup-style actions that expose exactly one managed executable on their action-added `PATH` are promoted to local binary providers
- Promoted providers get a generated local `tinx.yaml` and execute like a normal passthrough tool provider
- `GITHUB_ENV`, `GITHUB_PATH`, `GITHUB_OUTPUT`, and `GITHUB_STATE` file-command handling
- A stable `tinx` shim is placed on the GHA runtime `PATH`
- Cached runtime state under `~/.tinx/providers/gha/...`

Current limitations:

- No Docker actions yet
- No nested `uses:` steps inside composite actions yet
- Binary promotion is intentionally conservative; if bootstrap yields zero or multiple managed executables, the installed provider remains action-backed instead of guessing an entrypoint

Current Node runtime behavior:

- `tinx` expects a working `node` runtime to already be available on `PATH`
- A future tinx-native Node provider can replace this assumption without changing the `gha://` interface

## Configuration

- Default runtime home: `~/.tinx` (or project-provided override)
- Workspace runtime home: `<workspace>/.tinx`
- Override runtime home per command:

```bash
tinx --tinx-home /custom/path run <provider> <capability>
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
