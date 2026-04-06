# tinx

OCI-native provider runtime, workspace shell, and packager.

`tinx` packages providers as OCI artifacts and exposes them as normal commands inside a workspace-local shell. A workspace keeps provider aliases, lock state, and generated runtime artifacts together so the same toolchain can run on developer machines and in CI.

## Documentation

- Start with the documentation landing page: [docs/intro.md](docs/intro.md)
- Run the local docs site: `npm install && npm run docs:start`
- Build the static docs site: `npm run docs:build`

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
npm run docs:build
```
