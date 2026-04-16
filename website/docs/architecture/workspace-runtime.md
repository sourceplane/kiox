---
title: Workspace runtime
---

The workspace runtime is the project-local layer tinx builds under `.workspace/`.

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
4. activates matching provider metadata from the shared global store when available, otherwise installs or refreshes provider metadata
5. ensures the current host runtime blobs exist in the shared store when the workspace needs a full remote install
6. writes a new lock file and alias map

## Shell build phase

After sync, tinx builds workspace runtime artifacts:

```text
.workspace/
  env
  path
  bin/
    <alias>
    <provided-command>
```

Key behaviors:

- `.workspace/bin` is recreated on each build
- aliases are sorted before shell artifacts are written
- provider environment variables must agree when they share a key
- shims are written for both workspace aliases and tool `provides` entries
- generated shims still call the hidden `tinx __shim` command, not the real tool binary directly
- `tinx -- <command>` and `tinx exec <command>` can dispatch directly to a prepared workspace target instead of shelling back through a shim

The shims are what make lazy installation work for shells and exported workspace `PATH`s. Direct dispatch is an optimization for tinx CLI entrypoints, not a separate execution model.

## PATH layout

The generated `PATH` is assembled in this order:

1. `.workspace/bin`
2. environment path entries from the selected providers
3. the original host `PATH`

That ensures a provider alias wins over host binaries with the same name.

When tinx resolves a command through either a shim or direct dispatch, it can still add tool-specific binary directories for that single process.

## Working directory behavior

If you launch tinx from inside the workspace tree, tinx preserves your current directory. If you launch it from outside, tinx executes from the workspace root.

## Shell files

- `.workspace/env` is a shell-friendly export file
- `.workspace/path` is newline-separated for inspection and tooling

These files are generated output. Treat them as runtime state, not source of truth.

## Environment merging

The workspace environment contains:

- workspace-scoped variables such as `TINX_WORKSPACE_ROOT`
- provider-derived variables for each alias
- merged static path entries

If two providers export the same variable with different values, tinx fails rather than picking one silently.

Tool-scoped environment resources are merged when tinx resolves the selected tool plan.
