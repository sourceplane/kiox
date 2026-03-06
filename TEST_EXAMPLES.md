# tinx test examples

This file provides reproducible local and GHCR test flows for `tinx`.

## Local e2e (OCI layout)

From `tinx/`:

```bash
make e2e-local
```

This runs:
1. build `tinx`
2. package `testdata/echo-provider` into local OCI layout
3. install provider from layout
4. run provider capability (`plan`)

Expected output includes:
- `released sourceplane/echo-provider@v0.1.0`
- `installed sourceplane/echo-provider@v0.1.0`
- `capability=plan`

## GHCR push and pull test

### Prerequisites

- Auth to GHCR in your shell, for example:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_USER" --password-stdin
```

- Optional variables:

```bash
export GHCR_OWNER=<github-owner-lowercase>
export GHCR_REPO=ghcr.io/$GHCR_OWNER/tinx-echo-provider
export GHCR_TAG=dev-$(date +%s)
export GHCR_REF=$GHCR_REPO:$GHCR_TAG
```

### Push

```bash
make ghcr-push GHCR_REF=$GHCR_REF
```

Expected output includes:
- `pushed ghcr.io/.../tinx-echo-provider:<tag>`

### Pull/install and run

```bash
make ghcr-install-run GHCR_REF=$GHCR_REF
```

Expected output includes:
- `installed sourceplane/echo-provider@v0.1.0`
- `capability=plan`

## GitHub Actions pipelines

- `release.yml`
  - Trigger: tag push (`v*`) or manual
  - Runs tests and publishes `tinx` artifacts via GoReleaser

- `provider-ghcr-e2e.yml`
  - Trigger: `main` push or manual
  - Builds `tinx`, packages and pushes example provider to GHCR
  - Installs provider from GHCR and runs capability validation
