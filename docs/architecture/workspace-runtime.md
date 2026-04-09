---
title: Workspace runtime
---

The workspace runtime is the layer that turns a manifest into a shell environment.

## Workspace resolution

The command layer resolves a workspace target from:

1. `--workspace`
2. upward discovery from the current directory
3. the active workspace stored in tinx home

If a registered workspace root no longer exists, tinx marks it as missing and asks you to delete the stale entry.

## Sync phase

During sync, tinx:

1. loads the current `tinx.lock`
2. resolves each provider source to a local OCI layout or registry reference
3. decides whether to reuse the locked source or refresh it
4. installs or refreshes provider metadata
5. writes a new lock file and alias map

## Shell build phase

After sync, tinx builds workspace runtime artifacts:

```text
.workspace/
  env
  path
  bin/
    <alias>
```

Key behaviors:

- `.workspace/bin` is recreated on each build
- aliases are sorted before shell artifacts are written
- provider environment variables must agree when they share a key
- workspace shims call the real provider binary with `exec`

## PATH layout

The generated `PATH` is assembled in this order:

1. `.workspace/bin`
2. provider `spec.path` entries
3. the original host `PATH`

That ensures a provider alias wins over host binaries with the same name.

## Working directory behavior

If you launch tinx from inside the workspace tree, tinx preserves your current directory. If you launch it from outside, tinx executes from the workspace root.

## Shell files

- `.workspace/env` is a shell-friendly export file
- `.workspace/path` is newline-separated for inspection and tooling

These files are generated output. Treat them as runtime state, not source of truth.
