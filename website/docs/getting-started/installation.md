---
title: Installation
---

Install kiox with Go when you want a local CLI quickly, or use the release installer when you want a prebuilt binary.

## Prerequisites

- macOS or Linux
- Go 1.24+ for source installs
- Registry access only when you install or push remote providers

## Install with Go

```bash
go install github.com/sourceplane/kiox/cmd/kiox@latest
```

## Install from GitHub Releases

```bash
curl -fsSL https://raw.githubusercontent.com/sourceplane/kiox/main/install.sh | bash
```

## Verify the CLI

```bash
kiox version
kiox --help
```

## Build kiox from this repository

Use this path when you are working in the repository and want `./bin/kiox` for local examples:

```bash
make build
./bin/kiox version
```

## Build the docs site locally

The documentation site lives in `website/` and uses Docusaurus:

```bash
cd website
npm install
npm run docs:start
```

## Next steps

1. Follow the [quick start](./quick-start.md) to package and run the example provider in this repository.
2. Read [workspace](../concepts/workspace.md) and [providers](../concepts/providers.md) before building your own manifests.
