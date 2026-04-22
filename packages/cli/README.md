# novanasctl

Command-line interface for NovaNas. Talks to the NovaNas API (not kube-apiserver).

## Build

```sh
go build -o bin/novanasctl .
```

## Quick start

```sh
# First-time login — opens browser for OIDC device-code flow
novanasctl login --login-server https://nas.local --name default

# Inspect your identity
novanasctl whoami

# List resources
novanasctl pool list
novanasctl dataset get foo -o json
novanasctl share list -o yaml

# Manage connection contexts
novanasctl context list
novanasctl context use lab
```

## Global flags

| Flag | Description |
| --- | --- |
| `-o, --output`           | `table` (default), `json`, `yaml` |
| `--context`              | Use a specific named context |
| `--server`               | Override server URL |
| `--insecure-skip-tls-verify` | Skip TLS verification |
| `--token`                | Supply a raw API token (bypasses keyring) |
| `-v, --verbose`          | Enable debug logging |

## Resources

`pool`, `dataset`, `bucket`, `share`, `disk`, `snapshot`, `replication`,
`backup`, `app`, `vm`, `user`, `system`.

All resources support `list`, `get <name>`, `delete <name>`, `create -f file.yaml`.

## Config

Config lives at `~/.config/novanasctl/config.yaml`:

```yaml
current-context: default
contexts:
  - name: default
    server: https://nas.local
    insecure-skip-tls-verify: false
```

Refresh tokens are stored in the OS keyring under service `novanasctl` keyed by
context name. Access tokens live in memory only.

## Scripting

Use `--token <api-token>` with `--server <url>` for unattended scripts, or
supply `NOVANASCTL_TOKEN`/`--token` from your CI secret store.

When the API returns `501 Not Implemented` the CLI prints
`server not yet implemented` and exits with code `3`.
