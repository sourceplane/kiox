---
title: Environment variables
---

tinx reads a small set of environment variables from the host and writes additional variables into the workspace shell.

## Host environment variables

| Variable | Meaning |
| --- | --- |
| `TINX_HOME` | Override the default tinx home directory (`~/.tinx`) |
| `TINX_REGISTRY_USERNAME` / `TINX_REGISTRY_PASSWORD` | Registry credentials for remote pulls and pushes |
| `ORAS_USERNAME` / `ORAS_PASSWORD` | Alternative registry credentials for ORAS-backed operations |
| `GITHUB_ACTOR` / `GITHUB_TOKEN` | Credentials used for `ghcr.io` when Docker credentials are not available |
| `SHELL` | Preferred interactive shell for `tinx shell` |

CLI flags take precedence when an equivalent flag exists. For example, `--tinx-home` overrides `TINX_HOME`.

## Workspace shell variables

When tinx builds a workspace shell, it exports:

| Variable | Meaning |
| --- | --- |
| `TINX_HOME` | Workspace runtime home |
| `TINX_GLOBAL_HOME` | Global tinx home used for shared provider state |
| `TINX_WORKSPACE_ROOT` | Workspace root directory |
| `TINX_WORKSPACE_HOME` | Workspace `.workspace/` directory |
| `TINX_WORKSPACE_ENV_FILE` | Path to the generated env file |
| `TINX_WORKSPACE_PATH_FILE` | Path to the generated path file |
| `TINX_WORKSPACE_PROVIDERS` | Path to the workspace provider metadata directory |
| `TINX_PROVIDER_<ALIAS>_REF` | Provider reference for the alias |
| `TINX_PROVIDER_<ALIAS>_HOME` | Provider store root |
| `TINX_PROVIDER_<ALIAS>_BINARY` | Resolved default tool path; it may still be lazy |

These variables are available to any command run through `tinx shell`, `tinx exec`, or `tinx -- ...`.

## Script runtime install variables

When tinx installs a `script` tool, it injects:

| Variable | Meaning |
| --- | --- |
| `TINX_TOOL_INSTALL_DIR` | Tool-specific install root in the provider store |
| `TINX_TOOL_BIN` | Exact executable path the install must create |
| `TINX_TOOL_NAME` | Tool resource name |
| `TINX_TOOL_COMMAND` | Primary provided command name |
| `TINX_PROVIDER_HOME` | Provider store root |

## Managed-install target variables

When one tool installs another through `install.tool`, the installer tool also receives:

| Variable | Meaning |
| --- | --- |
| `TINX_TARGET_TOOL_NAME` | Tool being installed |
| `TINX_TARGET_TOOL_BIN` | Exact binary path the installer must create |
| `TINX_TARGET_TOOL_COMMAND` | Primary command exposed by the target tool |
| `TINX_TARGET_TOOL_INSTALL_DIR` | Install root for the target tool |

## Provider template variables

tinx expands these values inside environment variables, tool `env` and `path` entries, and legacy provider `env` and `path` entries:

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
