---
title: Run Node.js in a workspace
---

This example shows the workspace flow for a Node provider package. Replace the provider reference with the registry namespace and version you publish internally.

## Create and populate a workspace

```bash
kiox init app
kiox add core/node as node
```

If you need to pin a version or registry:

```bash
kiox add ghcr.io/acme/node-provider:v20.19.0 as node
```

## Run Node commands

```bash
kiox --workspace app -- node --version
kiox --workspace app -- node build.js
kiox --workspace app exec node --version
```

If your provider package exposes companion commands through `provides`, those show up in the same workspace too:

```bash
kiox --workspace app -- npm test
kiox --workspace app -- npx eslint .
```

## Use the interactive shell

```bash
kiox --workspace app shell
node --version
npm test
```

## Check the runtime state

```bash
kiox --workspace app ls
kiox --workspace app status --verbose
cat app/.workspace/path
```

Use this pattern when you want one reproducible Node runtime per workspace instead of relying on the host machine.
