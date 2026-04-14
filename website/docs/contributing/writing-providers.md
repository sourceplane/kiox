---
title: Contributing provider examples
---

This page is for contributors who are adding provider examples, fixtures, or provider-focused documentation to the tinx repository itself.

## Use the fixture matrix

The repository now uses several provider fixtures, each covering a distinct part of the architecture:

- `testdata/echo-provider`: legacy single-binary shorthand
- `testdata/multi-tool-provider`: multi-document normalized package with a script-installed tool
- `testdata/inline-tool-provider`: inline normalized package with bundled assets
- `testdata/setup-kubectl`: managed-install provider where one tool installs another lazily

When you add or expand provider fixtures, decide which part of the architecture you are exercising and keep that fixture focused.

If you add a new fixture, also add manual commands for it to `TEST_PROVIDERS.md` and update the relevant provider docs under `website/docs/providers/`.

## Keep examples task-oriented

When you add provider docs:

- show the manifest or the relevant resource snippet
- show the packaging command
- show the workspace command that runs the provider
- show `tinx ls` or `tinx status` when lazy tool behavior matters
- avoid abstract descriptions without a CLI example

## Update reference pages with behavior changes

If you change:

- manifest fields
- runtime environment variables
- packaging flags
- install or sync behavior
- inventory or status output

then update the relevant pages in:

- `website/docs/providers/`
- `website/docs/reference/`
- `website/docs/examples/`
- `website/docs/architecture/` when the execution model changes

## Validate the fixture flow

Prefer the manual commands in `TEST_PROVIDERS.md` instead of inventing a new ad hoc test path.

For focused validation, the most useful packages are:

```bash
go test ./internal/parser ./internal/cmd ./internal/oci ./internal/workspace
```

The legacy echo-provider path still has Makefile helpers if you need it:

```bash
make e2e-local
```

That keeps the provider examples aligned with the documented workflow.
