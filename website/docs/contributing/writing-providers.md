---
title: Contributing provider examples
---

This page is for contributors who are adding provider examples, fixtures, or provider-focused documentation to the tinx repository itself.

## Use the existing fixture pattern

The repository keeps a small provider under `testdata/echo-provider/`:

- `tinx.yaml` defines the provider contract
- `cmd/echo-provider` contains the implementation
- `make release-example` packages it
- `make install-example` and `make run-example` validate the flow

Follow that structure when you add or expand provider fixtures.

## Keep examples task-oriented

When you add provider docs:

- show the manifest
- show the packaging command
- show the workspace command that runs the provider
- avoid abstract descriptions without a CLI example

## Update reference pages with behavior changes

If you change:

- manifest fields
- runtime environment variables
- packaging flags
- install or sync behavior

then update the relevant pages in:

- `website/docs/providers/`
- `website/docs/reference/`
- `website/docs/examples/`

## Validate the fixture flow

Use the existing make targets instead of inventing a new ad hoc test path:

```bash
make release-example
make install-example
make run-example
```

That keeps the provider examples aligned with the documented workflow.
