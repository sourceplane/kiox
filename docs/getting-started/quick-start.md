---
title: Quick start
---

This walkthrough uses the example provider shipped in `testdata/echo-provider`. It packages the provider as a local OCI layout, creates a workspace, and runs the provider through the workspace shell.

## 1. Build tinx

```bash
make build
```

## 2. Package the example provider

```bash
make release-example
```

This writes a local OCI layout to `testdata/echo-provider/oci/`.

## 3. Create a workspace that uses the local layout

```bash
./bin/tinx init demo -p testdata/echo-provider/oci as echo
```

The command writes:

- `demo/tinx.yaml`
- `demo/tinx.lock`
- `demo/.workspace/`

## 4. Inspect the workspace

```bash
./bin/tinx --workspace demo status
./bin/tinx workspace list
./bin/tinx workspace current
```

## 5. Run the provider through the workspace environment

```bash
./bin/tinx --workspace demo -- echo plan
./bin/tinx --workspace demo exec echo plan
```

The example provider prints the capability and arguments it receives. In a real provider, the alias behaves like any other command on `PATH`.

## 6. Start an interactive workspace shell

```bash
./bin/tinx --workspace demo shell
```

Inside the shell, run the provider directly:

```bash
echo plan
```

## 7. Clean up

```bash
./bin/tinx workspace delete demo
```

## What happened

1. `release-example` built multi-platform binaries and packed them into an OCI image layout.
2. `tinx init` created a workspace manifest and synced the provider into `.workspace/`.
3. The workspace shell materialized the current platform binary, wrote provider shims, and prepended `.workspace/bin` to `PATH`.

Next, read [workspace](../concepts/workspace.md) and [runtime shell](../concepts/runtime-shell.md).
