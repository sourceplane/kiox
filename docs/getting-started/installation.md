---
title: Installation
---

Install tinx with Go when you want a local CLI quickly, or use the release installer when you want a prebuilt binary.

## Prerequisites

- macOS or Linux
- Go 1.24+ for source installs
- Registry access only when you install or push remote providers

## Install with Go

```bash
go install github.com/sourceplane/tinx/cmd/tinx@latest
```

## Install from GitHub Releases

```bash
curl -fsSL https://raw.githubusercontent.com/sourceplane/tinx/main/install.sh | bash
```

## Verify the CLI

```bash
tinx version
tinx --help
```

## Build tinx from this repository

Use this path when you are working in the repository and want `./bin/tinx` for local examples:

```bash
make build
./bin/tinx version
```

## Build the docs site locally

The documentation site lives at the repository root and uses Docusaurus:

```bash
npm install
npm run docs:start
```

## Next steps

1. Follow the [quick start](./quick-start.md) to package and run the example provider in this repository.
2. Read [workspace](../concepts/workspace.md) and [providers](../concepts/providers.md) before building your own manifests.
