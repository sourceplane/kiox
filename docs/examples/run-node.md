---
title: Run Node.js in a workspace
---

This example shows the workspace flow for a Node.js provider. Replace the provider reference with the registry namespace and version you publish internally.

## Create and populate a workspace

```bash
tinx init app
tinx provider add core/node as node
```

If you need to pin a version or registry:

```bash
tinx provider add ghcr.io/acme/node-provider:v20.19.0 as node
```

## Run Node commands

```bash
tinx --workspace app -- node --version
tinx --workspace app -- node build.js
tinx --workspace app exec node --version
```

## Use the interactive shell

```bash
tinx --workspace app shell
node --version
npm test
```

## Check the runtime state

```bash
tinx --workspace app status --verbose
cat app/.workspace/path
```

Use this pattern when you want one reproducible Node runtime per workspace instead of relying on the host machine.
