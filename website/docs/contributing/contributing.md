---
title: Contributing
---

Use this repository workflow when you change the tinx CLI, provider packaging logic, runtime behavior, or the docs site.

## Writing style

When you update docs:

- prefer concepts before commands
- keep the layers clear: workspace, provider package, runtime
- explain why first, how second
- keep the mental model consistent across pages
- document implemented behavior, not just parsed schema

## Prerequisites

- Go 1.24+
- Node.js and npm for the docs site

## Common commands

```bash
make build
go test ./...
```

For the docs site:

```bash
cd website
npm install
npm run docs:start
npm run docs:build
```

Legacy echo-provider helper targets still exist:

```bash
make release-example
make install-example
make run-example
make e2e-local
```

For the current fixture matrix, use the manual commands in `TEST_PROVIDERS.md`.

## Repository layout

- `cmd/tinx`: tinx entrypoint
- `internal/cmd`: Cobra command definitions
- `internal/core`: normalized package model and tool dependency planning
- `internal/parser`: manifest normalization and multi-document parsing
- `internal/workspace`: workspace manifests, sync, and shell files
- `internal/oci`: OCI packaging and installation
- `internal/runtime`: environment assembly and process execution helpers
- `internal/runtimes`: built-in `oci`, `script`, and `local` runtime plugins
- `internal/state`: tinx home state
- `website/docs/`: Docusaurus source docs
- `testdata/`: provider fixtures, including legacy, multi-tool, inline, and managed-install examples

## Updating documentation

When the CLI or runtime behavior changes:

1. update the affected doc pages under `website/docs/`
2. update `website/sidebars.js` if navigation changes
3. update `README.md` if the landing flow changes
4. update `TEST_PROVIDERS.md` if fixture workflows change
5. re-run `tinx --help` and subcommand help for CLI reference pages
6. build the docs site before opening a PR

If you add a new concept page, make sure it maps cleanly to one of:

- workspace
- provider package
- runtime

## Testing changes

Run the existing Go tests and the Docusaurus build before you send a change for review.

If you touch provider packaging or workspace execution, also run the relevant fixture commands from `TEST_PROVIDERS.md`. Use `make e2e-local` when you need to keep the legacy echo-provider flow covered.
