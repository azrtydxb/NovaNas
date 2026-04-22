# novanasctl

Command-line interface for NovaNas. Talks to the NovaNas API (not kube-apiserver).

## Build

```sh
# The primary CLI
go build -o bin/novanasctl .

# The kubectl plugin wrapper (optional)
go build -o bin/kubectl-novanas ./cmd/kubectl-novanas
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

## As a kubectl plugin

`kubectl-novanas` is a thin wrapper around `novanasctl` that can be invoked
through `kubectl`. Install the binary anywhere on your `PATH`:

```sh
go build -o bin/kubectl-novanas ./cmd/kubectl-novanas
sudo install -m 0755 bin/kubectl-novanas /usr/local/bin/kubectl-novanas

# kubectl picks it up automatically:
kubectl novanas --help
kubectl novanas pool list
kubectl novanas whoami
```

Verify installation:

```sh
kubectl plugin list | grep novanas
```

The plugin forwards all arguments to `novanasctl`, so every flag and
subcommand documented above works identically under `kubectl novanas ...`.

## Enabling shell completion

`novanasctl` exposes a `completion` subcommand (via Cobra). Pick the
snippet for your shell.

### bash

```sh
# temporary (current shell)
source <(novanasctl completion bash)

# persistent — system-wide (Linux)
novanasctl completion bash | sudo tee /etc/bash_completion.d/novanasctl

# persistent — per-user (macOS with bash-completion@2 via Homebrew)
novanasctl completion bash > $(brew --prefix)/etc/bash_completion.d/novanasctl
```

Requires `bash-completion` (package `bash-completion` on Debian/Ubuntu or
`brew install bash-completion@2` on macOS).

### zsh

```sh
# one-time setup (first line only if compinit is not already enabled)
echo "autoload -U compinit; compinit" >> ~/.zshrc

# persistent
novanasctl completion zsh > "${fpath[1]}/_novanasctl"

# or for a user-local directory that's on $fpath
mkdir -p ~/.zsh/completions
novanasctl completion zsh > ~/.zsh/completions/_novanasctl
```

On macOS under oh-my-zsh, place the file in `~/.oh-my-zsh/completions/`.

### fish

```sh
novanasctl completion fish | source                       # current shell
novanasctl completion fish > ~/.config/fish/completions/novanasctl.fish
```

### PowerShell

```powershell
novanasctl completion powershell | Out-String | Invoke-Expression

# persistent — append to your profile
novanasctl completion powershell >> $PROFILE
```

The `kubectl-novanas` plugin inherits the same completion subcommand; if
you use the plugin, run `kubectl-novanas completion <shell>` instead.
