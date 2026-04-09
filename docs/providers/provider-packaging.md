---
title: Provider packaging
---

tinx packages providers as OCI image layouts. You can keep the layout on disk, install from it locally, or push it to a registry.

## `tinx pack`

Use `pack` when the binaries and assets already exist:

```bash
tinx pack \
  --manifest tinx.yaml \
  --artifact-root dist \
  --output oci \
  --tag v1.2.3
```

`pack` reads:

- `tinx.yaml`
- the built binaries listed in `spec.platforms`
- optional assets under `spec.layers.assets.root`

and writes a directory-based OCI layout to `oci/`.

## `tinx release`

Use `release` when tinx should build first and then pack:

```bash
tinx release \
  --manifest tinx.yaml \
  --main ./cmd/my-provider \
  --dist dist \
  --output oci
```

Push to a registry in the same command:

```bash
tinx release \
  --manifest tinx.yaml \
  --main ./cmd/my-provider \
  --push ghcr.io/acme/my-provider:v1.2.3
```

## Build strategies

By default, `tinx release` uses `go build` with the platforms listed in the provider manifest.

Use GoReleaser when you already maintain a GoReleaser pipeline:

```bash
tinx release \
  --manifest tinx.yaml \
  --delegate-goreleaser \
  --goreleaser-config .goreleaser.yaml
```

If no GoReleaser config exists, tinx can generate one from the provider manifest.

## OCI layout contents

Each packaged provider layout contains:

- provider config JSON
- the original `tinx.yaml`
- provider metadata JSON
- one binary layer per platform
- an optional tarred assets layer

That layout is copied into tinx home during install and used later for binary materialization.

## Install the result

```bash
tinx install acme/my-provider --source ./oci
tinx init demo -p ./oci as my-provider
```
