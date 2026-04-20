---
title: Use kiox in CI
---

Use kiox in CI when you want providers resolved the same way on developer machines and in automation.

## Use an explicit kiox home

Set `--kiox-home` or `KIOX_HOME` so the CI cache path is predictable:

```bash
export KIOX_HOME="$PWD/.kiox-home"
kiox --kiox-home "$KIOX_HOME" install ghcr.io/acme/node-provider:v20.19.0
```

`install` only pre-populates provider metadata and OCI store content. The first workspace command may still lazily materialize tools.

## Materialize from a workspace manifest

```bash
kiox init ci-workspace -p ghcr.io/acme/node-provider:v20.19.0 as node
kiox --workspace ci-workspace -- node --version
```

Or check in a `kiox.yaml` and initialize from that manifest:

```bash
kiox init
kiox sync   # after manual edits
kiox -- node build
```

## Cache the home directory

Cache these paths between CI jobs:

- `.kiox-home/providers/`
- `.kiox-home/store/`

That preserves metadata, OCI store content, extracted assets, and previously installed lazy tools so later jobs avoid re-downloading or reinstalling providers.

If you need visibility into what the cache contains, emit inventory in CI logs:

```bash
kiox --kiox-home "$KIOX_HOME" ls default
```

## Authenticate to registries

kiox prefers explicit environment credentials for non-interactive registry access:

- `KIOX_REGISTRY_USERNAME` / `KIOX_REGISTRY_PASSWORD`
- `ORAS_USERNAME` / `ORAS_PASSWORD`
- `GITHUB_ACTOR` / `GITHUB_TOKEN` for `ghcr.io`

Set `KIOX_REGISTRY_DOCKER_AUTH=1` if you want kiox to fall back to Docker credential helpers and config. On macOS that fallback is disabled by default so public pulls do not trigger the system prompt for access to other apps' data.

## Non-interactive execution

Prefer `kiox exec` or `kiox --` in CI:

```bash
kiox exec lite-ci plan
kiox -- node test
```

Use `kiox shell` only for local interactive debugging.
