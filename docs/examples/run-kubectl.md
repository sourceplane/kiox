---
title: Run kubectl in a workspace
---

Use a kubectl provider when you want one Kubernetes client version pinned to the workspace.

## Add the provider

```bash
tinx init cluster-admin
tinx provider add tinx/kubectl as kubectl
```

## Run kubectl through tinx

```bash
tinx --workspace cluster-admin -- kubectl version --client
tinx --workspace cluster-admin -- kubectl get pods -A
```

## Mix host credentials with provider binaries

tinx does not replace your kubeconfig flow. Keep using the normal Kubernetes environment variables and config files:

```bash
export KUBECONFIG=$HOME/.kube/config
tinx --workspace cluster-admin -- kubectl config current-context
```

## Inspect the selected version

```bash
tinx --workspace cluster-admin status
tinx provider list cluster-admin
```

Use this approach when you want a team-wide kubectl version without asking every workstation to manage the same binary separately.
