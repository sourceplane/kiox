---
title: tinx run
---

`tinx run` is deprecated and kept only to point users at the workspace model. The supported execution paths are `tinx shell`, `tinx exec`, and `tinx -- ...`.

## Migrate old commands

Replace direct provider execution with workspace execution:

```bash
# old
tinx run node build

# new
tinx provider add core/node as node
tinx -- node build
```

```bash
# old
tinx run lite-ci plan

# new
tinx -- lite-ci plan
```

## Why the command was removed

tinx now treats provider execution as a workspace concern:

- the workspace owns provider aliases
- the workspace lock records resolved versions
- the runtime shell builds one merged environment for all providers

That lets providers call each other naturally through `PATH`.

## Help output

```text
Deprecated: execution must go through workspace shells

Usage:
  tinx run <provider-or-alias> [args...] [flags]

Flags:
  -h, --help   help for run

Global Flags:
      --tinx-home string   override the tinx home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

If you invoke `tinx run`, tinx returns an error that explains how to switch to `tinx -- <command>`.
