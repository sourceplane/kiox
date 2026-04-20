---
title: Providers
---

A **provider package** is the unit of distribution in kiox.

It is a versioned OCI artifact that can expose one or more tools plus the bundles, assets, and environment data those tools need at runtime.

Think of it as a portable tool package with an internal dependency graph.

## What a provider package owns

A provider package is responsible for:

- publishing tool resources and marking one as the default tool
- shipping bundle layers for platform binaries or asset payloads
- mounting assets into the provider store when needed
- exporting environment variables and path entries
- declaring tool-to-tool dependencies inside the same provider

## Canonical structure

```yaml
apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: acme
  name: setup-kubectl
  version: v0.1.0
spec:
  tools:
    - name: setup-kubectl
      runtime: oci
      from: bundle.setup-kubectl
      provides:
        - setup-kubectl
    - name: kubectl
      default: true
      runtime: local
      install:
        tool: setup-kubectl
        path: bin/kubectl
      dependsOn:
        - tool: setup-kubectl
      provides:
        - kubectl
  bundles:
    - name: setup-kubectl
      layers:
        - platform:
            os: linux
            arch: amd64
          mediaType: application/vnd.kiox.tool.binary
          source: bin/linux/amd64/setup-kubectl
  environments:
    - name: default-env
      variables:
        KUBECTL_PROVIDER_REF: ${provider_ref}
      export:
        - KUBECTL_PROVIDER_REF
```

In that model:

- `setup-kubectl` is a bundled installer tool
- `kubectl` is the user-facing tool
- the workspace alias points at the default tool
- the first `kubectl` execution can trigger the installer tool lazily

The canonical provider model is resource-based, but kiox also accepts the legacy single-tool manifest shorthand and normalizes it into the same internal representation.

## Resource kinds

### Tool

Tools define:

- the runtime plugin to use
- where the tool comes from
- which command names should appear in the workspace
- which other tools must exist before this tool runs

### Bundle

Bundles hold immutable OCI-backed payloads:

- platform-specific binary layers for `oci` tools
- tar layers for assets
- any other packaged content the provider needs at runtime

```yaml
bundles:
  - name: policy-assets
    type: asset
    platforms:
      - os: any
        arch: any
        source: assets/policy
```

### Asset

Assets mount bundle content into the provider store so tools and environments can reference it through `${provider_assets}`.

### Environment

Environment resources export variables and path entries into workspace execution.

## Alias versus provided commands

Every workspace provider entry creates an alias that resolves to the provider default tool. kiox also creates shims for every command in `provides`.

That means one provider package can surface commands such as:

- a default alias like `kubectl`
- an installer command like `setup-kubectl`
- additional companion commands such as `npm` or `npx`

## Authoring modes

You can author the same provider package in two ways:

- **Inline Provider document**: `tools`, `bundles`, `assets`, and `environments` live under one `Provider` document.
- **Multi-document package**: separate `Tool`, `Bundle`, `Asset`, and `Environment` documents share one file.

kiox normalizes both styles into the same internal package model.

## Legacy compatibility

kiox still accepts the legacy single-binary shorthand with `runtime: binary`, `entrypoint`, and `platforms`. That shorthand is normalized into:

- one default `Tool`
- one `Bundle`
- one `Environment`
- one `Asset` mount when assets are declared

## Distribution model

Provider packages are distributed as:

- OCI image layouts for local use
- OCI registry artifacts for remote use

That gives kiox standard registry interoperability, caching, and versioning.

## What a provider package does not do

- choose the active workspace
- resolve provider-to-provider orchestration across a workspace
- decide when commands run
- own global host state

Provider packages define tools and runtime inputs. Workspaces and the runtime orchestrate them.

## Provider reference forms

kiox accepts these source forms:

- `namespace/name`
- `namespace/name:tag`
- `namespace/name@digest`
- `ghcr.io/org/provider:tag`
- `/path/to/local/oci-layout`

Examples:

```bash
kiox install sourceplane/echo-provider --source ./oci
kiox provider add core/node as node
kiox provider add ghcr.io/acme/kubectl:v1.31.0 as kubectl
```
