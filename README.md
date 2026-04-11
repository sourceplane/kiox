# tinx

OCI-native provider runtime, workspace shell, and packager.

`tinx` is a workspace-centric runtime for tools. Providers are packaged as OCI artifacts, composed into a workspace, and executed through a reproducible shell environment.

The main abstractions are:

- **Workspace**: the unit of execution
- **Provider**: the unit of distribution
- **Runtime**: the execution layer

## Documentation

- Start with the concept-first landing page: [website/docs/intro.md](website/docs/intro.md)
- Run the local docs site: `cd website && npm install && npm run docs:start`
- Build the static docs site: `cd website && npm run docs:build`

## Manual Cloudflare Pages deploy

The docs site builds into `website/docs-build/`. To publish it manually to Cloudflare Pages:

```bash
cd website
npm ci
npm run docs:build
wrangler login
wrangler pages deploy docs-build --project-name tinx
```

Replace `tinx` with your Cloudflare Pages project name if it is different.

## Install

Install from source:

```bash
go install github.com/sourceplane/tinx/cmd/tinx@latest
```

Or install a release binary:

```bash
curl -fsSL https://raw.githubusercontent.com/sourceplane/tinx/main/install.sh | bash
```

Verify the CLI:

```bash
tinx version
tinx --help
```

## Quick example

Build tinx and package the example provider:

```bash
make build
make release-example
```

Create a workspace from the local OCI layout and run the provider:

```bash
./bin/tinx init demo -p testdata/echo-provider/oci as echo
./bin/tinx --workspace demo status
./bin/tinx --workspace demo -- echo plan
```

## Common workflows

Create and use a workspace:

```bash
tinx init demo
tinx use demo
```

Add providers and run them through the workspace shell:

```bash
tinx provider add core/node as node
tinx provider add sourceplane/lite-ci as lite-ci
tinx -- node build
tinx -- lite-ci plan
```

Package and publish a provider:

```bash
tinx release --manifest tinx.yaml --main ./cmd/my-provider --push ghcr.io/acme/my-provider:v1.2.3
```

## Development

```bash
make test
cd website && npm run docs:build
```
