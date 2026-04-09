---
title: Multi-provider workspace
---

One workspace can expose several providers on the same `PATH`. That is the default tinx model for multi-step workflows.

## Example manifest

```yaml
apiVersion: tinx.io/v1
kind: Workspace
workspace: dev
providers:
  node:
    source: core/node
  lite-ci:
    source: sourceplane/lite-ci
  docker:
    source: tinx/docker
  kubectl:
    source: tinx/kubectl
```

Initialize from the manifest:

```bash
tinx init ./tinx.yaml
```

## Run providers side by side

```bash
tinx -- node build
tinx -- lite-ci plan
tinx -- docker version
tinx -- kubectl version --client
```

You can also pass one provider command line after another:

```bash
tinx -- lite-ci plan -- node build
```

## Inspect the workspace

```bash
tinx status --verbose
tinx provider list
tinx workspace current
```

## Refresh selected providers

```bash
tinx provider update node
tinx provider update lite-ci kubectl
```

This layout works well for build pipelines, developer shells, and platform workflows where one toolchain should be versioned as a unit.
