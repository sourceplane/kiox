---
title: Execution model
---

The tinx execution model is simple: resolve a workspace, sync it, build a shell environment, and then run a process inside that environment.

## End-to-end flow

```bash
tinx --workspace demo -- node build
```

This command expands into the following steps:

1. Read the selected workspace from `--workspace`, the current directory, or the active workspace record.
2. Normalize the workspace manifest and lock file.
3. Resolve each provider alias to a provider key such as `namespace/name@version`.
4. Ensure metadata and OCI store content are available.
5. Build shims and shell files in `.workspace/`.
6. Prepend workspace and provider paths.
7. Resolve `node` from `PATH`.
8. Execute the binary with the merged environment.

## Compatibility shortcut

`tinx -- ...` is a shortcut for the workspace runtime path:

```bash
tinx -- node build
tinx -- lite-ci plan
```

Use it when you want a short command line and do not need to spell out `exec`.

## Working directory rules

If your current working directory is inside the workspace root, tinx preserves it. If you launch a workspace command from outside the workspace tree, tinx falls back to the workspace root before it runs the command.

## Failure points

tinx fails early when:

- no workspace is active and none can be discovered
- the selected workspace root is missing
- provider environment variables conflict
- the requested command is not present on the constructed `PATH`

That keeps the runtime deterministic and avoids hidden fallback behavior.
