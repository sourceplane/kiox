---
title: Providers
---

A **provider** is the unit of distribution in tinx.

It is a versioned OCI artifact that packages a tool.

Think of it as a portable tool package.

## Responsibilities

A provider is responsible for:

- shipping binaries
- defining an entrypoint
- declaring supported platforms
- optionally providing assets
- defining environment configuration

## Provider structure

```yaml
kind: Provider
metadata:
  namespace: sourceplane
  name: echo-provider
  version: v0.1.0
spec:
  runtime: binary
  entrypoint: echo-provider
  platforms:
    - os: linux
      arch: amd64
      binary: bin/linux/amd64/echo-provider
  capabilities:
    plan:
      description: Generate a plan
```

## Components

### Binary layer

- platform-specific executables
- required for execution

### Assets layer

- templates
- certificates
- configuration files

```yaml
layers:
  assets:
    root: assets
```

### Metadata

- capabilities
- environment variables
- PATH extensions

## Distribution model

Providers are distributed as:

- OCI image layouts for local use
- OCI registry artifacts for remote use

That gives tinx standard registry interoperability, caching, and versioning.

## Design properties

### Immutable

A provider version does not change once published.

### Portable

A provider runs across environments using OCI distribution.

### Self-contained

It includes everything required for execution.

### Simple interface

It exposes a binary, not a custom protocol.

## What a provider does not do

- manage dependencies between providers
- decide when to execute
- control the runtime environment globally

It packages. It does not orchestrate.

## Provider reference forms

tinx accepts these source forms:

- `namespace/name`
- `namespace/name:tag`
- `namespace/name@digest`
- `ghcr.io/org/provider:tag`
- `/path/to/local/oci-layout`

Examples:

```bash
tinx install sourceplane/echo-provider --source ./oci
tinx provider add core/node as node
tinx provider add ghcr.io/acme/kubectl:v1.31.0 as kubectl
```
