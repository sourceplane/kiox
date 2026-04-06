---
title: Runtime shell
---

The runtime shell is how tinx turns provider metadata into an execution environment. It syncs providers, writes shell artifacts, and then runs either an interactive shell or a single command.

## Commands that enter the runtime shell

```bash
tinx shell
tinx exec node build
tinx -- node build
```

- `tinx shell` starts an interactive shell.
- `tinx exec` runs one command and exits.
- `tinx -- ...` is the compatibility shortcut that routes directly into the workspace command path.

## What happens before execution

1. tinx resolves the selected workspace.
2. tinx syncs the workspace manifest and lock file.
3. tinx loads provider metadata for each alias.
4. tinx materializes the current platform binary if it is not already extracted.
5. tinx writes `.workspace/bin/<alias>` shims.
6. tinx writes `.workspace/env` and `.workspace/path`.
7. tinx prepends `.workspace/bin` and provider paths to `PATH`.

## Generated environment

The workspace shell exports variables such as:

- `TINX_HOME`
- `TINX_WORKSPACE_ROOT`
- `TINX_WORKSPACE_HOME`
- `TINX_WORKSPACE_ENV_FILE`
- `TINX_WORKSPACE_PATH_FILE`
- `TINX_PROVIDER_<ALIAS>_REF`
- `TINX_PROVIDER_<ALIAS>_HOME`
- `TINX_PROVIDER_<ALIAS>_BINARY`

These variables are available to providers and to any shell commands you run through tinx.

## Interactive shell behavior

tinx uses the current `SHELL` value when it can and falls back to `/bin/sh`. For common POSIX shells, tinx adds `-i` so the shell starts in interactive mode.

```bash
tinx shell
```

## Command execution behavior

When you run `tinx exec` or `tinx --`, tinx resolves the command from the constructed `PATH` and executes it directly:

```bash
tinx exec kubectl version --client
tinx -- lite-ci plan -- node build
```

If the command cannot be found on `PATH`, tinx returns the underlying executable lookup error.

## Environment merge rules

Provider environment variables are merged into one workspace environment. If two providers try to set the same key to different values, tinx fails the shell build instead of silently choosing one value.
