---
title: Quick start
---

This walkthrough uses the normalized multi-tool provider shipped in `testdata/multi-tool-provider`. It packages the provider as a local OCI layout, creates a workspace, and runs both the provider alias and a provided command through the workspace shell.

## 1. Build kiox

```bash
make build
```

## 2. Package the example provider

```bash
./bin/kiox release \
	--manifest testdata/multi-tool-provider/kiox.yaml \
	--dist testdata/multi-tool-provider/dist \
	--output testdata/multi-tool-provider/oci
```

This writes a local OCI layout to `testdata/multi-tool-provider/oci/`.

## 3. Create a workspace that uses the local layout

```bash
./bin/kiox init demo -p testdata/multi-tool-provider/oci as echo
```

The command writes:

- `demo/kiox.yaml`
- `demo/kiox.lock`
- `demo/.workspace/`

## 4. Inspect the workspace

```bash
./bin/kiox --workspace demo status
./bin/kiox --workspace demo ls
./bin/kiox workspace list
./bin/kiox workspace current
```

You should see the workspace home, the installed provider, and the tool inventory. Before the first command runs, the tools still show as lazy.

## 5. Run the provider through the workspace environment

```bash
./bin/kiox --workspace demo exec echo one two
./bin/kiox --workspace demo exec echo-tool alpha beta
```

The first command triggers lazy materialization of the bundled `setup-echo` tool and lazy installation of the script-backed `echo-tool`. In a real provider, both the alias and any provided commands behave like normal entries on `PATH`.

Re-run the inventory commands to see the transition from lazy to ready:

```bash
./bin/kiox --workspace demo status
./bin/kiox --workspace demo ls
```

## 6. Start an interactive workspace shell

```bash
./bin/kiox --workspace demo shell
```

Inside the shell, run the provider directly:

```bash
echo three four
echo-tool five six
```

## 7. Clean up

```bash
./bin/kiox workspace delete demo
```

## What happened

1. `kiox release` built the required bundle-backed binaries and packed them into an OCI image layout.
2. `kiox init` created a workspace manifest and synced the provider into `.workspace/`.
3. The workspace shell wrote lazy shims for `echo` and `echo-tool`, then the first execution materialized and installed the required tools on demand.

Next, read [workspace](../concepts/workspace.md) and [runtime shell](../concepts/runtime-shell.md).
