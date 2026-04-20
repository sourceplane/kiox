---
title: kiox install
---

`kiox install` copies provider metadata and OCI content into kiox home from either a registry reference or a local OCI layout. It does **not** add the provider to a workspace and it does **not** materialize lazy tools yet.

For execution, add the provider to a workspace and use `kiox shell`, `kiox exec`, or `kiox -- ...`.

## Common examples

Install from a registry reference:

```bash
kiox install ghcr.io/acme/node-provider:v20.19.0
kiox install node ghcr.io/acme/node-provider:v20.19.0
kiox install ghcr.io/acme/node-provider:v20.19.0 as node
```

Install from a local OCI layout:

```bash
kiox install sourceplane/echo-provider --source ./testdata/echo-provider/oci
kiox install acme/setup-kubectl --source ./testdata/setup-kubectl/oci --tag v0.1.0
```

When you use `--source`, the reference must be `<namespace>/<name>` and kiox validates that the layout matches the requested provider.

## When to use install

Use `install` when you want to:

- inspect provider metadata in kiox home
- pre-populate a shared or cached kiox home directory
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
  kiox install <ref> [as <alias>] [flags]

Flags:
  -h, --help            help for install
      --plain-http      use plain HTTP for registry pull/install
      --source string   path to a local OCI image layout
      --tag string      OCI tag inside the local layout

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

## Recommended follow-up

After install, add the provider to a workspace:

```bash
kiox init demo
kiox add ghcr.io/acme/node-provider:v20.19.0 as node
kiox --workspace demo -- node --version
```
