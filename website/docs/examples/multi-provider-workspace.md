---
title: Multi-provider workspace
---

One workspace can expose several provider packages on the same `PATH`. That is the default tinx model for multi-step workflows.

## Example manifest

```yaml
apiVersion: tinx.io/v1
kind: Workspace
metadata:
  name: dev
providers:
  node:
    source: core/node
  lite-ci:
    source: sourceplane/lite-ci
  docker:
    source: tinx/docker
  kubectl:
    source: ghcr.io/acme/setup-kubectl:v1.31.0
```

Save that as `tinx.yaml` in the project root, then initialize or reconcile it:

```bash
tinx init
tinx sync   # after manual edits
```

## Run commands side by side

```bash
tinx --workspace dev -- node build.js
tinx --workspace dev -- lite-ci plan
tinx --workspace dev -- docker version
tinx --workspace dev -- kubectl version --client
```

If one process needs several tools at once, run that process inside the workspace shell:

```bash
tinx --workspace dev -- sh -lc 'node --version && kubectl version --client'
```

## Inspect the workspace

```bash
tinx --workspace dev ls
tinx --workspace dev status --verbose
tinx provider list dev
tinx workspace current
```

## Refresh selected providers

```bash
tinx provider update node
tinx provider update lite-ci kubectl
```

This layout works well for build pipelines, developer shells, and platform workflows where several toolchains should be versioned as one workspace.
