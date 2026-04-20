# kiox

OCI-native provider package runtime, lazy workspace shell, and packager.

`kiox` packages tools and toolchains as OCI artifacts, installs them into a shared cache, and exposes them through reproducible workspace shims. The current architecture normalizes both legacy single-binary manifests and newer provider packages that declare multiple tools, bundles, assets, and environments.

The main abstractions are:

- **Workspace**: the unit of execution
- **Provider package**: the unit of distribution
- **Tool**: an executable surface exposed by a provider
- **Runtime plugin**: the resolution and execution strategy for a tool (`oci`, `script`, or `local`)

Workspace state follows a simple desired-vs-actual split:

- `kiox.yaml`: user-owned desired workspace state
- `kiox.lock`: kiox-owned resolved provider state

## Documentation

- Start with the concept-first landing page: [website/docs/intro.md](website/docs/intro.md)
- Run the local docs site: `cd website && npm install && npm run docs:start`
- Build the static docs site: `cd website && npm run docs:build`
- Local provider fixture commands live in `TEST_PROVIDERS.md`

## Manual Cloudflare Pages deploy

The docs site builds into `website/docs-build/`. To publish it manually to Cloudflare Pages:

```bash
cd website
npm ci
npm run docs:build
wrangler login
wrangler pages deploy docs-build --project-name kiox
```

Replace `kiox` with your Cloudflare Pages project name if it is different.

## Install

Install from source:

```bash
go install github.com/sourceplane/kiox/cmd/kiox@latest
```

Or install a release binary:

```bash
curl -fsSL https://raw.githubusercontent.com/sourceplane/kiox/main/install.sh | bash
```

Verify the CLI:

```bash
kiox version
kiox --help
```

## Quick example

Build kiox and package the normalized multi-tool fixture:

```bash
make build
./bin/kiox release \
	--manifest testdata/multi-tool-provider/kiox.yaml \
	--dist testdata/multi-tool-provider/dist \
	--output testdata/multi-tool-provider/oci
```

Create a workspace from the local OCI layout and run both the provider alias and the provided tool command:

```bash
./bin/kiox init demo -p testdata/multi-tool-provider/oci as echo
./bin/kiox --workspace demo status
./bin/kiox --workspace demo exec echo one two
./bin/kiox --workspace demo exec echo-tool alpha beta
```

The first execution materializes the bundled `setup-echo` tool and lazy-installs the script-backed `echo-tool` into the provider store.

## Common workflows

Create and use a workspace:

```bash
kiox init demo
kiox add core/node as node
kiox sync
kiox use demo
```

Add providers and run them through the workspace shell:

```bash
kiox provider add core/node as node
kiox provider add sourceplane/lite-ci as lite-ci
kiox -- node build
kiox -- lite-ci plan
```

Package and publish a provider:

```bash
kiox release --manifest provider.yaml --main ./cmd/my-provider --push ghcr.io/acme/my-provider:v1.2.3
```

## Development

```bash
make test
go test ./...
cd website && npm run docs:build
```

For manual smoke tests against the repository fixtures, see `TEST_PROVIDERS.md`.
