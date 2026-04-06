---
title: Use tinx in CI
---

Use tinx in CI when you want providers resolved the same way on developer machines and in automation.

## Use an explicit tinx home

Set `--tinx-home` or `TINX_HOME` so the CI cache path is predictable:

```bash
export TINX_HOME="$PWD/.tinx-home"
tinx --tinx-home "$TINX_HOME" install ghcr.io/acme/node-provider:v20.19.0
```

## Materialize from a workspace manifest

```bash
tinx init ci-workspace -p ghcr.io/acme/node-provider:v20.19.0 as node
tinx --workspace ci-workspace -- node --version
```

Or check in a `tinx.yaml` and initialize from that manifest:

```bash
tinx init ./tinx.yaml
tinx -- node build
```

## Cache the home directory

Cache these paths between CI jobs:

- `.tinx-home/providers/`
- `.tinx-home/store/`

That preserves both metadata and OCI store content so later jobs avoid re-downloading providers.

## Authenticate to registries

tinx reads registry credentials from standard Docker config and from environment variables such as:

- `TINX_REGISTRY_USERNAME` / `TINX_REGISTRY_PASSWORD`
- `ORAS_USERNAME` / `ORAS_PASSWORD`
- `GITHUB_ACTOR` / `GITHUB_TOKEN` for `ghcr.io`

## Non-interactive execution

Prefer `tinx exec` or `tinx --` in CI:

```bash
tinx exec lite-ci plan
tinx -- node test
```

Use `tinx shell` only for local interactive debugging.
