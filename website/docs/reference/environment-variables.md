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
| `TINX_WORKSPACE_ROOT` | Workspace root directory |
| `TINX_WORKSPACE_HOME` | Workspace `.workspace/` directory |
| `TINX_WORKSPACE_ENV_FILE` | Path to the generated env file |
| `TINX_WORKSPACE_PATH_FILE` | Path to the generated path file |
| `TINX_WORKSPACE_PROVIDERS` | Path to the workspace provider metadata directory |
| `TINX_PROVIDER_<ALIAS>_REF` | Provider reference for the alias |
| `TINX_PROVIDER_<ALIAS>_HOME` | Provider store root |
| `TINX_PROVIDER_<ALIAS>_BINARY` | Materialized binary path |

These variables are available to any command run through `tinx shell`, `tinx exec`, or `tinx -- ...`.

## Provider manifest template variables

tinx expands these values inside `spec.env` and `spec.path` entries:

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
| `${provider_binary}` | Materialized provider binary path |
| `${provider_assets}` | Provider assets root |

Unknown template names are left unchanged.
