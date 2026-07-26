# cipi-cli

Command-line interface for [Cipi](https://cipi.sh) — manage your servers, apps, databases, SSL certificates, and deployments from the terminal.

## Installation

### From source

```bash
git clone https://github.com/cipi-sh/cli.git
cd cli
make build
sudo make install
```

### Download binary

Download the latest release for your platform from the [Releases](https://github.com/cipi-sh/cli/releases) page, then:

```bash
chmod +x cipi-cli-*
sudo mv cipi-cli-* /usr/local/bin/cipi-cli
```

## Quick start

### Mental model: one profile = one server

A **profile** is a named connection to a Cipi server (API endpoint + token).  
Use as many profiles as you have servers. The alias `servers` works like `profiles`.

### 1. Add servers

```bash
cipi-cli api token add prod
cipi-cli api token add staging
# or: cipi-cli configure --profile prod
# or: cipi-cli profiles add staging
```

You will be prompted for:

- **Profile name** — alias for this server (e.g. `prod`, `staging`) if you omit it
- **API endpoint** — the URL of your Cipi API (e.g. `https://api.example.com`)
- **Token** — a Sanctum token created with `cipi api token create` on your server

Credentials are stored per profile in `~/.cipi/config.json` (permissions `0600`).  
Omitting the profile name no longer writes silently to `default` — you will be asked.

Non-interactive:

```bash
cipi-cli api token add prod --endpoint https://api.example.com --token "1|yourtoken..."
cipi-cli profiles add staging --endpoint https://staging.example.com --token "1|yourtoken..."
```
### 2. Manage servers

```bash
cipi-cli profiles                 # list servers (alias: cipi-cli servers)
cipi-cli profiles show prod       # inspect one server
cipi-cli profiles use prod        # set default server
cipi-cli profiles delete staging  # remove a server profile
```

### 3. Run commands

Prefix any command with the profile name to target that server:

```bash
cipi-cli prod apps list
cipi-cli staging apps show myapp
cipi-cli prod deploy myapp
cipi-cli prod ssl install myapp
```

After `cipi-cli profiles use prod`, you can omit the prefix:

```bash
cipi-cli apps list                # uses the default profile (prod)
```

## Commands

### Apps

```
cipi-cli apps list                          List all applications
cipi-cli apps show <name>                   Show application details
cipi-cli apps create [flags]                Create a new application
cipi-cli apps edit <name> [flags]           Edit an application
cipi-cli apps delete <name> [-y]            Delete an application
cipi-cli apps suspend <name>                Suspend an application (HTTP 503)
cipi-cli apps unsuspend <name>              Bring a suspended application back online
cipi-cli apps logs <name> [flags]           Read application logs
```

**Create flags:** `--user`, `--domain`, `--php`, `--repository`, `--branch`, `--custom`, `--docroot`

**Edit flags:** `--php`, `--repository`, `--branch`, `--domain` (rename primary domain; requires Cipi 4.6.2+ / API 1.9.0+)

**Logs flags:** `--type` (default `all`), `--page` (default `1`), `--per-page` (default `50`, max `1000`; requires API 1.11.9+)

### Domains

```
cipi-cli domains                            List every domain and alias across all apps
```

### Deploy

```
cipi-cli deploy <app>                       Trigger a deployment
cipi-cli deploy rollback <app> [-y]         Rollback to previous release
cipi-cli deploy unlock <app>                Unlock a stuck deployment
```

### SSL

```
cipi-cli ssl install <app>                  Install Let's Encrypt certificate
```

### Aliases

```
cipi-cli aliases list <app>                 List aliases
cipi-cli aliases add <app> <domain>         Add an alias
cipi-cli aliases remove <app> <domain> [-y] Remove an alias
```

### Databases

```
cipi-cli db list                            List all databases
cipi-cli db create <name>                   Create a database
cipi-cli db delete <name> [-y]              Delete a database
cipi-cli db backup <name>                   Create a backup
cipi-cli db restore <name> [-y]             Restore from backup
cipi-cli db password <name> [-y]            Regenerate password
```

### Status

```
cipi-cli status                             Global overview — one row per server
cipi-cli status <profile>                   Full details for one server
cipi-cli <profile> status                   Same, via profile prefix
```

Global columns: NAME, IP, CPU, RAM, HDD, APPS, SVC, CIPI.  
Requires the API token ability `status-view` (`GET /api/status`, same data as `cipi status` on the host).

### Jobs

```
cipi-cli jobs show <id>                     Show job status
cipi-cli jobs wait <id>                     Wait for a job to complete
```

### Configuration & servers (profiles)

```
cipi-cli api token add [profile]            Add/update API token for a named profile
cipi-cli configure [--profile NAME]         Add/update a server profile (endpoint + token)
cipi-cli profiles                           List configured servers
cipi-cli profiles add [name]                Add/update a server profile
cipi-cli profiles list                      List configured servers
cipi-cli profiles show [profile]            Show one or all server profiles
cipi-cli profiles use <profile>             Set the default server (alias: default)
cipi-cli profiles delete <profile> [-y]     Delete a server profile
```

Aliases: `servers` / `server` → `profiles`; `profiles use` → `profiles default`.  
Always pass a profile name (`prod`, `staging`, …) — otherwise the CLI prompts for one.

### Update

```
cipi-cli update                             Update the CLI to the latest release
cipi-cli update --force                     Reinstall even if already up to date
```

Downloads the matching binary from the latest [GitHub Release](https://github.com/cipi-sh/cli/releases), verifies its SHA-256 checksum, and replaces the running binary in place. If it is installed in a system path (e.g. `/usr/local/bin`), run it with `sudo cipi-cli update`.

### Shell completion

```
cipi-cli completion install                 Auto-detect shell and install completion
cipi-cli completion install --shell zsh     Install for a specific shell (zsh|bash|fish)
```

Writes the script under `~/.cipi/completions/` (or fish’s completions dir) and, for zsh/bash, appends a source line to your rc file. Reload the shell afterwards.

### Other

```
cipi-cli version                            Print version
cipi-cli --help                             Help
```

## Global flags

| Flag         | Description                           |
| ------------ | ------------------------------------- |
| `--json`     | Output in JSON format (for scripting) |
| `--no-color` | Disable colored output                |

## Async operations

Write operations (create, edit, delete, deploy, SSL, etc.) are asynchronous on the Cipi API. The CLI automatically polls for job completion and displays a spinner while waiting. If you prefer to handle polling manually, use `cipi-cli jobs show <id>`.

## Releases

Releases are automated via GitHub Actions. To publish a new version:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Prefer tags with a `v` prefix (`v1.0.0`). The pipeline builds binaries for Linux (amd64/arm64) and macOS (amd64/arm64), generates SHA-256 checksums, and creates a GitHub Release with all artifacts attached.

If a release is missing binaries, re-run the **Release** workflow manually from GitHub Actions (workflow dispatch) using the tag name.

### Manual cross-compilation

```bash
make release
```

## Requirements

The Cipi server must have the [API package](https://github.com/cipi-sh/api) installed and configured:

```bash
cipi api <domain>
cipi api ssl
cipi api token create
```

See the [Cipi API documentation](https://cipi.sh/docs/advanced#cipi-api) for details.

| Feature | Minimum Cipi | Minimum API |
| --- | --- | --- |
| Suspend / unsuspend | 4.5.8 | 1.8.1 |
| Rename primary domain (`apps edit --domain`) | 4.6.2 | 1.9.0 |
| App logs (`apps logs`) | — | 1.11.9 |
| Server status (`status`) | — | with `GET /api/status` + `status-view` |
| Global domain map (`domains`) | 4.5.5 | — (built from `/api/apps`) |

New app PHP versions must be **8.3**, **8.4**, or **8.5** (Cipi 4.5.4+).

## License

MIT
