---
title: kiox run
---

`kiox run` is deprecated and kept only to point users at the workspace model. The supported execution paths are `kiox shell`, `kiox exec`, and `kiox -- ...`.

## Migrate old commands

Replace direct provider execution with workspace execution:

```bash
# old
kiox run node build

# new
kiox add core/node as node
kiox -- node build
```

```bash
# old
kiox run lite-ci plan

# new
kiox exec lite-ci plan
```

## Why the command was removed

kiox now treats execution as a workspace concern:

- the workspace owns provider aliases
- the workspace lock records resolved versions and digests
- the runtime shell builds one merged environment for all providers
- shims resolve tool plans and lazy installs before execution

That lets provider commands call each other naturally through `PATH` and keeps setup-style tools in the same model as bundled tools.

## Help output

```text
Deprecated: execution must go through workspace shells

Usage:
  kiox run <provider-or-alias> [args...] [flags]

Flags:
  -h, --help   help for run

Global Flags:
      --kiox-home string   override the kiox home directory
  -w, --workspace string   select the workspace for workspace-shell commands
```

If you invoke `kiox run`, kiox returns an error that explains how to switch to `kiox -- <command>`.
