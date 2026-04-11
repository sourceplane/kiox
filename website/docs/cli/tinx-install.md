---
title: tinx install
---

`tinx install` installs provider metadata into tinx home from either a registry reference or a local OCI layout. It does **not** run the provider. For execution, add the provider to a workspace and use `tinx shell`, `tinx exec`, or `tinx -- ...`.

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
tinx install sourceplane/echo-provider --source ./testdata/echo-provider/oci --tag v0.1.0
```

When you use `--source`, the reference must be `<namespace>/<name>` and tinx validates that the layout matches the requested provider.

## When to use install

Use `install` when you want to:

- inspect provider metadata in tinx home
- pre-populate a shared or cached tinx home directory
- stage providers for CI or image builds

Use `provider add` when you want the provider available in a workspace.

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
tinx provider add ghcr.io/acme/node-provider:v20.19.0 as node
tinx --workspace demo -- node --version
```
