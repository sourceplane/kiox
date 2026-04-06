---
title: Contributing
---

Use this repository workflow when you change the tinx CLI, provider packaging logic, or the docs site.

## Prerequisites

- Go 1.24+
- Node.js and npm for the docs site

## Common commands

```bash
make build
make test
make release-example
make install-example
make run-example
```

For the docs site:

```bash
npm install
npm run docs:start
npm run docs:build
```

## Repository layout

- `cmd/tinx`: tinx entrypoint
- `internal/cmd`: Cobra command definitions
- `internal/workspace`: workspace manifests, sync, and shell files
- `internal/oci`: OCI packaging and installation
- `internal/runtime`: shell and command execution
- `internal/state`: tinx home state
- `docs/`: Docusaurus source docs
- `testdata/echo-provider`: example provider fixture

## Updating documentation

When the CLI or runtime behavior changes:

1. update the affected doc pages under `docs/`
2. update `sidebars.js` if navigation changes
3. update `README.md` if the landing flow changes
4. re-run `tinx --help` and subcommand help for CLI reference pages
5. build the docs site before opening a PR

## Testing changes

Run the existing Go tests and the Docusaurus build before you send a change for review.

If you touch provider packaging or workspace execution, also run the example provider flow:

```bash
make e2e-local
```
