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

## CLI Reference

```bash
tinx install [alias] <ref> [--source <oci-layout>] [--tag <tag>] [--plain-http]
tinx run <provider-or-alias> [capability-or-args...] [--plain-http]
tinx run gha://<owner>/<repo>@<ref> [--input name=value]
tinx install <alias> gha://<owner>/<repo>@<ref> [--input name=value]
tinx <alias> [capability-or-args...]
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
