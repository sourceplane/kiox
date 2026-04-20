---
title: Multi-provider workspace
---

One workspace can expose several provider packages on the same `PATH`. That is the default kiox model for multi-step workflows.

## Example manifest

```yaml
apiVersion: kiox.io/v1
kind: Workspace
metadata:
  name: dev
providers:
  node:
    source: core/node
  lite-ci:
    source: sourceplane/lite-ci
  docker:
    source: kiox/docker
  kubectl:
    source: ghcr.io/acme/setup-kubectl:v1.31.0
```

Save that as `kiox.yaml` in the project root, then initialize or reconcile it:

```bash
kiox init
kiox sync   # after manual edits
```

## Run commands side by side

```bash
kiox --workspace dev -- node build.js
kiox --workspace dev -- lite-ci plan
kiox --workspace dev -- docker version
kiox --workspace dev -- kubectl version --client
```

If one process needs several tools at once, run that process inside the workspace shell:

```bash
kiox --workspace dev -- sh -lc 'node --version && kubectl version --client'
```

## Inspect the workspace

```bash
kiox --workspace dev ls
kiox --workspace dev status --verbose
kiox provider list dev
kiox workspace current
```

## Refresh selected providers

```bash
kiox provider update node
kiox provider update lite-ci kubectl
```

This layout works well for build pipelines, developer shells, and platform workflows where several toolchains should be versioned as one workspace.
