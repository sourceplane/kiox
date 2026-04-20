---
title: Provider packaging
---

kiox packages provider packages as OCI image layouts. You can keep the layout on disk, install from it locally, or push it to a registry.

The packaged artifact always contains both the authoring manifest and the normalized package metadata kiox uses at runtime.

Use `provider.yaml` as the authoring manifest for provider packages. Legacy `kiox.yaml` provider manifests are still supported.

## `kiox pack`

Use `pack` when the bundle sources already exist on disk:

```bash
kiox pack \
  --manifest provider.yaml \
  --artifact-root dist \
  --output oci \
  --tag v1.2.3
```

`pack` reads:

- `provider.yaml`
- normalized package metadata derived from the manifest
- bundle layer sources for binaries or tarred assets

and writes a directory-based OCI layout to `oci/`.

## `kiox release`

Use `release` when kiox should build first and then pack:

```bash
kiox release \
  --manifest provider.yaml \
  --main ./cmd/my-provider \
  --dist dist \
  --output oci
```

Push to a registry in the same command:

```bash
kiox release \
  --manifest provider.yaml \
  --main ./cmd/my-provider \
  --push ghcr.io/acme/my-provider:v1.2.3
```

## Build strategies

By default, `kiox release` uses `go build` for the normalized bundle targets declared in the provider manifest.

If `--main` is not set, kiox infers a main package from `cmd/<binary>` for each bundle-backed binary target.

That means a provider package can publish several bundled tools without hard-coding one global entrypoint in the release step.

Use GoReleaser when you already maintain a GoReleaser pipeline:

```bash
kiox release \
  --manifest provider.yaml \
  --delegate-goreleaser \
  --goreleaser-config .goreleaser.yaml
```

If no GoReleaser config exists, kiox can generate one from the normalized bundle targets in the provider manifest.

## What gets packed

Each packaged provider layout contains:

- provider config JSON
- the original provider manifest
- normalized package metadata in `package.json`
- one OCI layer per bundle entry
- tarred asset layers for asset bundles

When a bundle entry is platform-specific, kiox writes a platform-qualified binary layer media type into the packaged OCI layout. That lets remote installs hydrate only the current host binaries instead of downloading every published platform blob.

The provider config records the default tool runtime, entrypoint, and default tool name so kiox can bootstrap installs quickly.

## Setup-style providers

Managed-install tools are not built directly unless they also have bundled binaries. Instead, the packaged artifact must include whatever installer tool they depend on.

For example, in `setup-kubectl`:

- `setup-kubectl` is bundle-backed and built into the OCI artifact
- `kubectl` is a `local` tool whose binary is created lazily by `setup-kubectl`

## Install the result

```bash
kiox install acme/my-provider --source ./oci
kiox init demo -p ./oci as my-provider
```

After install, the workspace shim path handles the final lazy tool materialization on first use.
