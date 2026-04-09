---
title: Execution model
---

The execution model is the relationship between the three core abstractions:

- **Provider** = packaged tool
- **Workspace** = selected composition of providers
- **Runtime** = execution layer

## Mental model

```text
Provider → tinx home → Workspace → Runtime → Command
```

```bash
tinx -- node build
```

The command above is not special. It follows the same path every time.

## End-to-end flow

1. Resolve the workspace from `--workspace`, discovery, or the active workspace record.
2. Normalize the workspace manifest and lock file.
3. Resolve each provider alias to a provider key like `namespace/name@version`.
4. Ensure metadata and OCI store content are available.
5. Build shims and shell files in `.workspace/`.
6. Prepend workspace and provider paths.
7. Resolve the command from `PATH`.
8. Execute the binary with the merged environment.

## Typical workflow

```yaml
providers:
  node: core/node
  kubectl: tinx/kubectl
```

```bash
tinx init dev
tinx use dev
tinx -- node build
```

## Design properties

- **Workspace-first**: execution always happens in a workspace
- **OCI-native**: provider distribution is standard and portable
- **Lazy**: binaries are extracted only when needed
- **Deterministic**: the same workspace behaves the same way everywhere
- **Simple**: no RPC, no plugin framework, just binaries on `PATH`

## Failure points

tinx fails early when:

- no workspace is active and none can be discovered
- the selected workspace root is missing
- provider environment variables conflict
- the requested command is not present on the constructed `PATH`

That keeps the runtime predictable and avoids hidden fallback behavior.
