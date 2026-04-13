---
title: tinx install
---

`tinx install` copies provider metadata and OCI content into tinx home from either a registry reference or a local OCI layout. It does **not** add the provider to a workspace and it does **not** materialize lazy tools yet.

For execution, add the provider to a workspace and use `tinx shell`, `tinx exec`, or `tinx -- ...`.

## Common examples

Install from a registry reference:

```bash
tinx install ghcr.io/acme/node-provider:v20.19.0
tinx install node ghcr.io/acme/node-provider:v20.19.0
tinx install ghcr.io/acme/node-provider:v20.19.0 as node
```

Install from a local OCI layout:

```bash
tinx install sourceplane/echo-provider --source ./testdata/echo-provider/oci
tinx install acme/setup-kubectl --source ./testdata/setup-kubectl/oci --tag v0.1.0
```

When you use `--source`, the reference must be `<namespace>/<name>` and tinx validates that the layout matches the requested provider.

## When to use install

Use `install` when you want to:

- inspect provider metadata in tinx home
- pre-populate a shared or cached tinx home directory
- stage provider packages for CI or image builds
- verify a local OCI layout without creating a workspace yet

Use `provider add` when you want the provider available in a workspace.

## What install does not do

`install` does not:

- create `.workspace/` artifacts
- register a workspace alias
- run `__shim`
- extract or install lazy tools

That last step still happens on the first real command execution inside a workspace.

## Help output

```text
Install provider metadata from an OCI layout or registry reference

Usage:
  tinx install <ref> [as <alias>] [flags]

Flags:
  -h, --help            help for install
      --plain-http      use plain HTTP for registry pull/install
      --source string   path to a local OCI image layout
      --tag string      OCI tag inside the local layout

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

## Recommended follow-up

After install, add the provider to a workspace:

```bash
tinx init demo
tinx add ghcr.io/acme/node-provider:v20.19.0 as node
tinx --workspace demo -- node --version
```
