---
title: Run kubectl in a workspace
---

Use a setup-style kubectl provider when you want one Kubernetes client version pinned to the workspace while still installing the binary lazily.

This example uses the repository fixture in `testdata/setup-kubectl`, which models the current managed-install architecture:

- `kubectl` is the default tool exposed to the workspace
- `setup-kubectl` is the bundled installer tool
- the first `kubectl` run downloads the requested client version into the provider store

## Package the provider fixture

Run these commands from the repository root:

```bash
./bin/kiox release \
	--manifest testdata/setup-kubectl/kiox.yaml \
	--dist testdata/setup-kubectl/dist \
	--output testdata/setup-kubectl/oci
```

## Create a workspace

```bash
./bin/kiox init cluster-admin -p testdata/setup-kubectl/oci as kubectl
```

## Inspect the lazy state

Before the first command runs, the provider is installed but the `kubectl` binary is still lazy:

```bash
./bin/kiox --workspace cluster-admin ls
./bin/kiox --workspace cluster-admin status
```

## Run kubectl through kiox

```bash
KUBECTL_VERSION=1.29 ./bin/kiox --workspace cluster-admin -- kubectl version --client
./bin/kiox --workspace cluster-admin -- kubectl get pods -A
```

The first command runs the `setup-kubectl` installer tool through the shim manager and writes the requested `kubectl` binary into the provider store.

## Mix host credentials with the workspace tool

kiox does not replace your kubeconfig flow. Keep using the normal Kubernetes environment variables and config files:

```bash
export KUBECONFIG=$HOME/.kube/config
./bin/kiox --workspace cluster-admin -- kubectl config current-context
```

## Inspect the ready state

After the first successful run, the tool inventory changes from lazy to ready:

```bash
./bin/kiox --workspace cluster-admin ls
./bin/kiox --workspace cluster-admin status
```

Use this approach when you want a team-wide kubectl version without asking every workstation to manage the same binary separately.
