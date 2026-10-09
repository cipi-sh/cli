# cipi-cli

Command-line interface for [Cipi](https://cipi.sh) — manage servers, apps, databases, SSL certificates, deployments, PHP, services, SMTP, and healthchecks from the terminal.

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

```bash
cipi-cli prod apps list          # explicit profile
cipi-cli apps list               # uses the default profile
```

### 1. Add servers

Create a token on the Cipi host first (`cipi api token create`), then store it locally under a profile name:

```bash
cipi-cli api token add prod
cipi-cli api token add staging
# or: cipi-cli configure --profile prod
# or: cipi-cli profiles add staging
```

You will be prompted for:

- **Profile name** — alias for this server (e.g. `prod`, `staging`) if you omit it
- **API endpoint** — the URL of your Cipi API (e.g. `https://api.example.com`)
- **Token** — the Sanctum token from the server

Credentials are stored per profile in `~/.cipi/config.json` (permissions `0600`).  
Omitting the profile name does **not** write silently to `default` — you will be asked.

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
cipi-cli profiles delete staging  # remove a local server profile (-y to skip confirm)
```

Deleting a profile only removes local credentials — nothing changes on the remote server.

### 3. Run commands

Prefix any command with the profile name to target that server:

```bash
cipi-cli prod apps list
cipi-cli staging apps show myapp
cipi-cli prod deploy myapp
cipi-cli prod ssl install myapp
cipi-cli status                   # overview of all profiles
cipi-cli status prod              # details for one server
```

After `cipi-cli profiles use prod`, you can omit the prefix:

```bash
cipi-cli apps list                # uses the default profile (prod)
```

Every command has detailed help:

```bash
cipi-cli --help
cipi-cli profiles --help
cipi-cli apps --help
cipi-cli deploy --help
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
cipi-cli apps fix-permissions <name>        Restore app home permissions (Cipi 5.2.1+)
cipi-cli apps artisan <name> [command...]   Run Artisan (async job, Cipi 5.0.3+)
cipi-cli apps run <name> [command...]       Run whitelisted command (Cipi 5.0.3+)
cipi-cli apps env show|set <name>           Read/update .env keys (Cipi 5.0.3+)
cipi-cli apps auth show|create|update|delete  Manage shared auth.json (Cipi 5.0.3+)
cipi-cli apps basicauth status|enable|disable HTTP Basic Auth
cipi-cli apps node status|restart <name>    Node app status / blue-green restart (5.4.0+)
cipi-cli apps search enable|disable <name>  Meilisearch/Scout per app (5.2.2+)
cipi-cli apps webhook recreate <name>       Recreate Git webhook [--rotate-secret] (5.0.6+)
```

Alias: `app` → `apps`.

**Create flags:** `--user`, `--domain`, `--php`, `--repository`, `--branch`, `--custom`, `--docroot`, `--engine`, `--octane`, `--node`, `--framework`, `--node-version`, `--build`, `--start`, `--output`, `--health-path` (Node apps take no `--php`)

**Edit flags:** `--php`, `--repository`, `--branch`, `--domain` (rename primary domain; requires Cipi 4.6.2+ / API 1.9.0+), plus Node flags above

**Logs flags:** `--type` (default `all`: nginx, php, worker, deploy, laravel), `--page` (default `1` = most recent), `--per-page` (default `50`, max `1000`; requires API 1.11.9+)

### Domains

```
cipi-cli domains                            List every domain and alias across all apps
```

Alias: `domain` → `domains`.  
For managing aliases on one app, see **Aliases** below.

### Deploy

```
cipi-cli deploy <app>                       Trigger a deployment
cipi-cli deploy rollback <app> [-y]         Rollback to previous release
cipi-cli deploy unlock <app>                Unlock a stuck deployment
cipi-cli deploy config show|set <app>       Structured deploy.php options (5.0.3+)
cipi-cli deploy audit <app> [--days N]      Deploy audit ledger (5.4.0+)
```

### SSL

```
cipi-cli ssl install <app>                  Install Let's Encrypt certificate
cipi-cli ssl force <app>                    Re-apply HTTP → HTTPS redirect (4.8+)
```

### WWW redirects

```
cipi-cli www status <app>                   Show www/apex redirect status
cipi-cli www add <app>                      Add www/apex counterpart alias
cipi-cli www force-to-root <app>            301 redirect www → apex
cipi-cli www force-from-root <app>          301 redirect apex → www
cipi-cli www clear <app>                    Clear www canonical redirect
```

### Redirects & proxies

```
cipi-cli redirect list <app>                List whole-app + path redirects (5.4.1+)
cipi-cli redirect set|unset|enable|disable <app>
cipi-cli redirect add|remove <app>          Path redirects (--from, --to)
cipi-cli proxies list|add|remove <app>      Prefix reverse proxies (5.4.1+)
```

Alias: `redirects` → `redirect`, `proxy` → `proxies`.

DNS for the app domain (and aliases) must already point to the server.

### Aliases

```
cipi-cli aliases list <app>                 List aliases
cipi-cli aliases add <app> <domain>         Add an alias
cipi-cli aliases remove <app> <domain> [-y] Remove an alias
```

Alias: `alias` → `aliases`.

### Databases

```
cipi-cli db engines                         List DB engines + default (4.8+)
cipi-cli db install <engine>                Install an engine: mariadb|pgsql (5.0.6+)
cipi-cli db list                            List all databases
cipi-cli db create <name> [--engine]        Create a database
cipi-cli db delete <name> [-y]              Delete a database (API < 1.19 only)
cipi-cli db backup <name>                   Create a backup
cipi-cli db restore <name> [-y]             Restore from backup
cipi-cli db password <name> [-y]            Regenerate password and update .env
cipi-cli db anonymize <app> --email <addr>  Request an anonymized dump via cipi/agent
cipi-cli db anonymize download <url> [-o F] Download the dump from the signed link
```

Aliases: `database`, `dbs` → `db`.  
API 1.19+ removed database deletion and setting the default engine: use `cipi db delete` / `cipi db default` on the host.

#### Anonymized dumps (cipi/agent)

`db anonymize` talks to the [cipi/agent](https://github.com/cipi-sh/agent) package installed in a Laravel app, not to the Cipi API: it reads the app URL (`APP_URL`, else `https://<primary domain>`) and `CIPI_ANONYMIZER_TOKEN` from the app `.env` through the API (`apps-view` + `apps-env` abilities), then calls `POST /<CIPI_ROUTE_PREFIX>/db` on the app. The agent runs the job in the app queue and emails a signed download link valid 15 minutes.

```bash
# one-time setup on the app
cipi-cli apps artisan myapp cipi:service anonymize --enable
cipi-cli apps artisan myapp cipi:generate-token anonymize
cipi-cli apps artisan myapp cipi:init-anonymize      # then edit /home/<user>/.db/anonymization.json

# request a dump, then download it from the link in the email
cipi-cli prod db anonymize myapp --email dev@example.com
cipi-cli db anonymize download "<signed url>" -o myapp.sql
cipi-cli db anonymize download "<signed url>" -o - | mysql -u root myapp_local
```

**Anonymize flags:** `--email` / `-e` (prompted if omitted), `--url` (override the app base URL, e.g. when the app redirects to `www`), `--token` (override the bearer token)  
**Download flags:** `--output` / `-o` (`-` for stdout), `--force` / `-f` (overwrite)

### PHP

```
cipi-cli php list                           Installed versions, FPM status, default (5.0.6+)
cipi-cli php install <version>              Install PHP 8.3 / 8.4 / 8.5
```

Switching the default PHP version and removing versions are host-only (`cipi php switch|remove`).

### Server services & SSH keys

```
cipi-cli services list [name]               System services and status (5.0.6+)
cipi-cli services restart <name>            Restart a service (nginx, mariadb, …)
cipi-cli ssh-keys list                      Authorized keys of the cipi user (5.0.6+)
cipi-cli ssh-keys add [key] [--file F]      Add a public key (inline or from a .pub file)
cipi-cli ssh-keys remove <id> [-y]          Remove a key by id
```

Aliases: `service` → `services`, `ssh` → `ssh-keys`.

### SMTP notifications

```
cipi-cli smtp show                          Current SMTP config (password never shown, 5.0.7+)
cipi-cli smtp set [flags]                   Configure; omitted flags keep current values
cipi-cli smtp test                          Send a test email
cipi-cli smtp enable|disable                Toggle notifications
cipi-cli smtp delete [-y]                   Remove the configuration
```

**Set flags:** `--host`, `--port` (587), `--user`, `--password` (required the first time), `--from`, `--to`, `--tls` (default on), `--enabled` (default on), `--no-test`

### Healthchecks

```
cipi-cli health list                        All configured app healthchecks (5.0.7+)
cipi-cli health show <app>                  One app's healthcheck and state
cipi-cli health set <app> [--url] [--expect] Enable/update (default https://<domain>/up, 200)
cipi-cli health unset <app>                 Disable
cipi-cli health check <app>                 Run now; exits 1 when unhealthy
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
cipi-cli run-commands                       List whitelisted app run binaries (5.0.3+)
```

Alias: `job` → `jobs`.  
Most write commands wait for jobs automatically; use these to inspect a job by ID.

### Search, Node, and host insights

```
cipi-cli search status                      Meilisearch status (5.2.2+)
cipi-cli node list                          Installed Node runtimes (5.4.0+)
cipi-cli monitor                            System monitor checks (5.3.0+)
cipi-cli packages                           Optional host packages catalog
cipi-cli zt                                 Cloudflare Zero Trust status (5.3.0+)
```

### IP whitelist

```
cipi-cli ip-whitelist show                  Show API client IP allowlist (5.0.8+)
cipi-cli ip-whitelist set <entries...>      Replace whitelist (* = allow all)
cipi-cli ip-whitelist add|remove <ip>
cipi-cli ip-whitelist allow-all
```

### Configuration & servers (profiles)

```
cipi-cli api token add [profile]            Add/update API token for a named profile
cipi-cli configure [--profile NAME]         Add/update a server profile (endpoint + token)
cipi-cli profiles                           List configured servers
cipi-cli profiles list                      Same as bare profiles
cipi-cli profiles add [name]                Add/update a server profile
cipi-cli profiles show [profile]            Show one or all server profiles
cipi-cli profiles use <profile>             Set the default server (alias: default)
cipi-cli profiles delete <profile> [-y]     Delete a local server profile
```

Aliases:

- `servers` / `server` → `profiles`
- `profiles use` → `profiles default`
- `configure list|show|delete|default` → prefer the `profiles` equivalents

Always pass a profile name (`prod`, `staging`, …) when adding credentials — otherwise the CLI prompts for one.

### Update

```
cipi-cli update                             Update the CLI to the latest release
cipi-cli update --force                     Reinstall even if already up to date (allows downgrade)
```

Aliases: `self-update`, `upgrade`.

Downloads the matching binary from GitHub Releases (highest semver, not GitHub’s “latest” flag), verifies its SHA-256 checksum, and replaces the running binary in place. If installed in a system path (e.g. `/usr/local/bin`), run `sudo cipi-cli update`.

### Shell completion

```
cipi-cli completion install                 Auto-detect shell and install completion
cipi-cli completion install --shell zsh     Install for a specific shell (zsh|bash|fish)
cipi-cli completion zsh                     Print script only (same for bash / fish)
```

Writes the script under `~/.cipi/completions/` (or fish’s completions dir) and, for zsh/bash, appends a source line to your rc file. Reload the shell afterwards.

### Other

```
cipi-cli version                            Print version and build time
cipi-cli --help                             Root help
cipi-cli <command> --help                   Command help
```

## Global flags

| Flag         | Description                           |
| ------------ | ------------------------------------- |
| `--json`     | Output in JSON format (for scripting) |
| `--no-color` | Disable colored output                |

## Async operations

Write operations (create, edit, delete, deploy, SSL, etc.) are asynchronous on the Cipi API. The CLI automatically polls for job completion and displays a spinner while waiting. If you prefer to handle polling manually, use `cipi-cli jobs show <id>` or `cipi-cli jobs wait <id>`.

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
| Multi-engine DB (`db engines`, `--engine`) | 4.8 | 1.12+ |
| WWW / SSL force | 4.8 | 1.12+ |
| Env / auth.json / artisan / run / deploy config | 5.0.3 | 1.14+ |
| PHP / SSH keys / services / `db install` / webhook recreate | 5.0.6 | 1.15+ |
| SMTP / healthchecks | 5.0.7 | 1.15+ |
| IP whitelist | 5.0.8 | 1.15+ |
| Fix permissions | 5.2.1 | 1.31+ |
| Search (Meilisearch) / packages | 5.2.2 | 1.31+ |
| Monitor / Zero Trust | 5.3.0 | 1.31+ |
| Redirects / proxies (API sudoers) | 5.4.1 | 1.31+ |
| Node apps / deploy audit | 5.4.0 | 1.31+ |
| Anonymized dumps (`db anonymize`) | 5.0.3 | 1.14+ (plus cipi/agent 1.1.1+ in the app) |
| App logs (`apps logs`) | — | 1.11.9 |
| Server status (`status`) | — | with `GET /api/status` + `status-view` |
| Global domain map (`domains`) | 4.5.5 | — (built from `/api/apps`) |
| Aliases add/remove path | — | 1.31+ (`/aliases/{domain}`) |

New app PHP versions must be **8.3**, **8.4**, or **8.5** (Cipi 4.5.4+).

## License

MIT
