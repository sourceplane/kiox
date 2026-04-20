---
title: Execution model
---

The execution model is the relationship between the three core abstractions:

- **Provider package** = packaged tool graph
- **Workspace** = selected composition of providers
- **Runtime** = lazy shim and plugin-driven execution layer

## Mental model

```text
Provider package → kiox home → Workspace → Lazy shim → Runtime plugin → Command
```

```bash
kiox -- node build
```

The command above is not special. It follows the same path every time.

## End-to-end flow

1. Resolve the workspace from `--workspace`, discovery, or the active workspace record.
2. Normalize the workspace manifest and lock file.
3. Resolve each provider alias to a provider key like `namespace/name@version`.
4. Ensure provider metadata and OCI store content are available.
5. Build `.workspace/` shims and shell files.
6. Prepend workspace and provider paths.
7. Resolve the command from `PATH`.
8. Let the shim map the command to a provider alias and tool name.
9. Compute the tool dependency plan.
10. Ask the selected runtime plugins whether each tool is already installed.
11. Materialize or install only the missing tools.
12. Execute the target command with the merged environment.

## Typical workflow

```yaml
providers:
  node: core/node
  kubectl: ghcr.io/acme/setup-kubectl:v1.31.0
```

```bash
kiox init dev
kiox use dev
kiox -- node build
```

## Design properties

- **Workspace-first**: execution always happens in a workspace
- **OCI-native**: provider distribution is standard and portable
- **Lazy**: bundles and script installs happen only when needed
- **Tool-aware**: execution is planned per tool, not per provider binary
- **Deterministic**: the same workspace behaves the same way everywhere
- **Normal process execution**: tools run as child processes, even though runtime resolution is plugin-driven internally

## Failure points

kiox fails early when:

- no workspace is active and none can be discovered
- the selected workspace root is missing
- provider environment variables conflict
- a tool dependency plan cannot be resolved
- a runtime plugin cannot resolve the requested tool on the current platform
- the requested command is not present on the constructed `PATH`

That keeps the runtime predictable and avoids hidden fallback behavior.
