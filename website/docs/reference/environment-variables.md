---
title: Environment variables
---

kiox reads a small set of environment variables from the host and writes additional variables into the workspace shell.

## Host environment variables

| Variable | Meaning |
| --- | --- |
| `KIOX_HOME` | Override the default kiox home directory (`~/.kiox`) |
| `KIOX_REGISTRY_USERNAME` / `KIOX_REGISTRY_PASSWORD` | Registry credentials for remote pulls and pushes |
| `ORAS_USERNAME` / `ORAS_PASSWORD` | Alternative registry credentials for ORAS-backed operations |
| `GITHUB_ACTOR` / `GITHUB_TOKEN` | Credentials used for `ghcr.io` when explicit registry credentials are not set |
| `KIOX_REGISTRY_DOCKER_AUTH` | Enable Docker credential-store fallback for registry operations. Default is `1` on Linux and Windows, and `0` on macOS to avoid interactive prompts during public pulls |
| `KIOX_REGISTRY_COPY_CONCURRENCY` | Maximum concurrent blob copy tasks for a single registry pull. Default is `2` |
| `KIOX_SYNC_INSTALL_CONCURRENCY` | Maximum number of independent provider installs kiox runs in parallel during workspace sync. Default is `4` |
| `SHELL` | Preferred interactive shell for `kiox shell` |

CLI flags take precedence when an equivalent flag exists. For example, `--kiox-home` overrides `KIOX_HOME`.

## Workspace shell variables

When kiox builds a workspace shell, it exports:

| Variable | Meaning |
| --- | --- |
| `KIOX_HOME` | Workspace runtime home |
| `KIOX_GLOBAL_HOME` | Global kiox home used for shared provider state |
| `KIOX_WORKSPACE_ROOT` | Workspace root directory |
| `KIOX_WORKSPACE_HOME` | Workspace `.workspace/` directory |
| `KIOX_WORKSPACE_ENV_FILE` | Path to the generated env file |
| `KIOX_WORKSPACE_PATH_FILE` | Path to the generated path file |
| `KIOX_WORKSPACE_PROVIDERS` | Path to the workspace provider metadata directory |
| `KIOX_PROVIDER_<ALIAS>_REF` | Provider reference for the alias |
| `KIOX_PROVIDER_<ALIAS>_HOME` | Provider store root |
| `KIOX_PROVIDER_<ALIAS>_BINARY` | Resolved default tool path; it may still be lazy |

These variables are available to any command run through `kiox shell`, `kiox exec`, or `kiox -- ...`.

## Script runtime install variables

When kiox installs a `script` tool, it injects:

| Variable | Meaning |
| --- | --- |
| `KIOX_TOOL_INSTALL_DIR` | Tool-specific install root in the provider store |
| `KIOX_TOOL_BIN` | Exact executable path the install must create |
| `KIOX_TOOL_NAME` | Tool resource name |
| `KIOX_TOOL_COMMAND` | Primary provided command name |
| `KIOX_PROVIDER_HOME` | Provider store root |

## Managed-install target variables

When one tool installs another through `install.tool`, the installer tool also receives:

| Variable | Meaning |
| --- | --- |
| `KIOX_TARGET_TOOL_NAME` | Tool being installed |
| `KIOX_TARGET_TOOL_BIN` | Exact binary path the installer must create |
| `KIOX_TARGET_TOOL_COMMAND` | Primary command exposed by the target tool |
| `KIOX_TARGET_TOOL_INSTALL_DIR` | Install root for the target tool |

## Provider template variables

kiox expands these values inside environment variables, tool `env` and `path` entries, and legacy provider `env` and `path` entries:

| Template | Meaning |
| --- | --- |
| `${cwd}` | Current working directory |
| `${workspace_root}` | Workspace root directory |
| `${workspace_home}` | Workspace `.workspace/` directory |
| `${provider_alias}` | Alias currently being processed |
| `${provider_ref}` | `namespace/name` |
| `${provider_namespace}` | Provider namespace |
| `${provider_name}` | Provider name |
| `${provider_version}` | Provider version |
| `${provider_home}` / `${provider_root}` | Provider store root |
| `${provider_binary}` | Resolved default tool path; it may still be lazy |
| `${provider_assets}` | Provider assets root |

Unknown template names are left unchanged.
