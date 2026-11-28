<p align="center">
  <a href="https://cipi.sh">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://cipi.sh/brand/cipi-logo-dark.svg">
      <img src="https://cipi.sh/brand/cipi-logo.svg" alt="Cipi" height="56">
    </picture>
  </a>
</p>

# cipi-cli

Command-line interface for [Cipi](https://cipi.sh) — manage servers, apps, databases, backups, SSL certificates, deployments, PHP and Node runtimes, services, SMTP, healthchecks, disk usage, and an app's `cipi.yml` from the terminal.

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
cipi-cli apps list                          List all applications (ENV column: environment label)
cipi-cli apps show <name>                   Show application details
cipi-cli apps create [flags]                Create a new application
                                            --custom / --node: --db NAME creates a database with the app and links it
                                            (5.18.0+; --engine picks its engine). A Laravel app has its own.
cipi-cli apps edit <name> [flags]           Edit an application
cipi-cli apps delete <name> [-y]            Delete an application
cipi-cli apps suspend <name>                Suspend an application (HTTP 503)
cipi-cli apps unsuspend <name>              Bring a suspended application back online
cipi-cli apps logs <name> [flags]           Read application logs
cipi-cli apps fix-permissions <name>        Restore app home permissions (Cipi 5.2.1+)
cipi-cli apps artisan <name> [command...]   Run Artisan (async job, Cipi 5.0.3+)
cipi-cli apps run <name> [command...]       Run whitelisted command (Cipi 5.0.3+)
cipi-cli apps env show|set <name>           Read/update .env keys (Cipi 5.0.3+)
cipi-cli apps env order <name> KEY... [--after|--before KEY]  Move .env keys (5.13.0+)
cipi-cli apps auth show|create|update|delete  Manage shared auth.json (Cipi 5.0.3+)
cipi-cli apps basicauth status|enable|disable HTTP Basic Auth
cipi-cli apps node status|restart <name>    Node app status / blue-green restart (5.4.0+)
cipi-cli apps search enable|disable <name>  Meilisearch/Scout per app (5.2.2+)
cipi-cli apps webhook recreate <name>       Recreate Git webhook [--rotate-secret] (5.0.6+)
```

Alias: `app` → `apps`.

**Create flags:** `--user`, `--domain`, `--php`, `--repository`, `--branch`, `--custom`, `--docroot`, `--engine`, `--octane`, `--node`, `--framework`, `--node-version`, `--build`, `--start`, `--output`, `--health-path` (Node apps take no `--php`), `--node-build` (Laravel apps only: the asset build run inside every deploy before go-live, e.g. `--node-build "npm ci && npm run build"`; must start with npm/npx/yarn/pnpm/bun/node, `&&` and `;` allowed, no pipes/redirects/backticks/`$( )`, max 200 chars; requires Cipi 5.6.0+ / API 1.34+). The interactive flow offers an optional build command prompt for Laravel apps.

**Environments:** `--env` labels the environment of an app — `production` (the default), `staging` or any label of 1 to 32 lowercase letters, digits, `-` or `_` (`apps create --env staging`, `apps edit <name> --env staging`; the CLI rejects anything else before calling the server). `apps list` shows it in the ENV column and `apps show` as Environment (`production` when the server sends none), and it selects the file the `yml` commands read: `cipi-<env>.yml` first, then `cipi.yml`. Requires Cipi 5.8.0+ / API 1.36+; older servers drop the field, so after create/edit the CLI reads the app back and warns when the label did not stick.

When the create job completes the CLI prints the app credentials (SSH, database), deploy key, webhook and build. With Cipi 5.6.0+ / API 1.34+, `apps create` and `apps edit --repository/--branch` also report whether the deploy key can read the repository: reachable (with the branch), reachable but the branch is missing, or not readable (the first deploy will fail; git's error and the fix command to run on the server are shown). Older servers report nothing. With `--json`, the job result is printed as `{"data": …}`, `repository_access` and `build` included.

**Edit flags:** `--php`, `--repository`, `--branch`, `--domain` (rename primary domain; requires Cipi 4.6.2+ / API 1.9.0+), `--env` (environment label; requires Cipi 5.8.0+ / API 1.36+), plus Node flags above

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
cipi-cli deploy status <app> [--tail N]     Running, queued or interrupted + the last run (5.8.4+)
cipi-cli deploy history <app> [--limit N]   Every recorded run of one app, newest first (5.9.0+)
cipi-cli deploy last                        The last run of every app (5.9.0+)
cipi-cli deploy rollback <app> [-y]         Rollback to previous release
cipi-cli deploy unlock <app>                Unlock a stuck deployment, resume a paused commit
cipi-cli deploy config show|set <app>       Structured deploy.php options (5.0.3+)
                                            [--keep-releases N] [--build-timeout S] [--step-timeout S] [--migrate] [--node-build CMD] …
                                            --build-timeout / --step-timeout (5.18.0+): seconds the frontend build may run (60-3600,
                                            1800 unless set) and every other command of a deploy (60-1800, 300 unless set).
                                            A Node app takes --keep-releases, --predeploy-snapshot and these two.
cipi-cli deploy audit <app> [--days N]      Deploy audit ledger (5.4.0+)
```

`deploy status` says whether a deploy is running (with its phase, trigger, branch and how long ago it started), queued behind another one, or interrupted — a run that never ended with nothing running it, in which case the CLI points at `deploy unlock` — and prints the last finished run (result, release, duration, trigger, branch). With Cipi 5.13.0+ it also shows **Paused**: the commit whose automatic deploy failed, which the webhook does not deploy again by itself; the next push does, and so do a deploy that succeeds and `deploy unlock`. `--tail N` adds the last N lines of the deploy log (max 500). Requires Cipi 5.8.4+ / API 1.37+ and the `deploy-manage` ability.

`deploy history` lists the recorded runs of one app (`--limit`, default 30, max 500) and `deploy last` the last run of every app, `never deployed` for an app without one. Both require Cipi 5.9.0+ / API 1.39+ and the `apps-view` or `deploy-manage` ability.

### cipi.yml

```
cipi-cli yml <app>                          What applying the app's cipi.yml would change
cipi-cli yml plan <app>                     Same (aliases: show, status, diff)
cipi-cli yml apply <app> [-y]               Apply it (async job: preview + confirmation unless -y)
cipi-cli yml auto <app> [on|off|status]     Apply after every successful deploy (default: status)
```

The `cipi.yml` an app ships in its repository describes the server state it expects (aliases, www redirect, basic auth, redirects, proxies, PHP version and settings, limits, extra databases, workers, healthcheck, deploy options, crons, backups, post-deploy steps); these commands mirror `cipi yml` on the host and read the file of the live release, so commit it and deploy first. An app with an environment label (`apps create --env staging`) reads `cipi-<env>.yml` first and falls back to the shared `cipi.yml`, so one repository can ship a file per environment (Cipi 5.8.0+ / API 1.36+); `deploy.branch` in the file is the git branch the app deploys.

`yml plan` prints the environment label and the file in use (and says when a non-production app is on the shared `cipi.yml`), whether auto-apply is on, validation errors and warnings, the changes, the blockers (while any is listed, apply changes nothing) and what happens after the next deploy; it exits 1 when there is no file, it is not valid or something blocks the apply (`--json` prints the data and exits 0). `yml apply` previews the plan and asks before dispatching the `app-yml-apply` job (`-y` skips both), then prints the outcome — applied, unchanged, partial (with the failed changes), blocked, invalid or no-file — and exits 1 unless it is applied or unchanged; `--json` prints the job result as `{"data": …}`. `yml auto <app> on` covers CLI, panel and Git webhook deploys: anyone who can commit to the repository can then change the app's aliases, PHP settings, limits, workers, deploy branch and recipe, scheduled commands, databases and backup schedule (for a Node app also its mode, Node version, build and start commands), so the CLI says so and asks for confirmation unless `-y` is passed.

Requires Cipi 5.6.4+, API 1.35+ and the `yml-view` (plan, auto status) / `yml-manage` (apply, auto on|off) token abilities — existing tokens do not get them: create a token with them (`cipi api token create`). Cipi 5.6.4 is also the release that applies `cipi.yml` on Git webhook deploys when auto-apply is on.

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

### Nginx directives

```
cipi-cli nginx show <app>                   Print the app's own nginx directives
cipi-cli nginx edit <app>                   Edit them in $VISUAL / $EDITOR, on this computer
cipi-cli nginx set <app> --file <path>      Replace them with a file's content (- for stdin)
cipi-cli nginx reset <app>                  Remove them
```

Headers, caching, limits, rewrites, `location` blocks of your own: Cipi keeps them and includes them in the app's server block, so they survive every change of the vhost (Cipi 5.14.0+, cipi/api 1.44+, abilities `nginx-view` / `nginx-manage`). The server tests every change with `nginx -t` before the reload; a refused text changes nothing and the reasons are printed line by line — `edit` then offers to open the editor again on your text.

From the API a **subset of nginx's language** is taken: no `access_log` or `error_log` with a path, `include`, `listen`, `server_name`, `*_pass`, certificates; `root` and `alias` only under the app's home. Reverse proxies and redirects are the commands above; the whole language is `cipi nginx edit <app>` on the server.

### Page cache

```
cipi-cli cache                              Every app: cache on or off, pages kept, space taken
cipi-cli cache show <app>                   One app: on or off, time kept, pages and bytes now
cipi-cli cache on <app> [--ttl 10m] [--size 256m] [--exclude '/cart*,/feed'] [--deploy-clear=false]
cipi-cli cache off <app>                    Every request runs PHP again; the cached pages are deleted
cipi-cli cache ttl <app> <time>             How long a page is kept: 30s, 10m, 1h, 7d
cipi-cli cache exclude <app>                The paths of its own the app never caches
cipi-cli cache exclude <app> add '/cart*' /feed
cipi-cli cache exclude <app> remove /feed   --all for none of its own
cipi-cli cache exclude <app> set '/cart*,/feed'
cipi-cli cache clear <app>                  Delete the cached pages now
cipi-cli cache purge <app> <page>...        Delete some pages only: /blog/post/, '/blog/*', a URL of the app (5.18.0+)
```

`cache purge` leaves the rest of the cache warm. A path is that page on every domain and alias of the app, http and https, with its `?utm_…` variants; a trailing `*` is every page under it (quote it); a URL is that page on that domain only. Up to 500 at once (cipi/api 1.48+). From the app's own code: the WordPress plugin [cipi-sh/cache-wp](https://github.com/cipi-sh/cache-wp) and the PHP package [cipi-sh/cache-php](https://github.com/cipi-sh/cache-php).

The page cache is nginx's: it keeps the page PHP made and answers the next visitors with it, without running PHP at all — `fastcgi_cache` for an app on PHP-FPM (Laravel, WordPress, any PHP app), `proxy_cache` for one on Octane (Cipi 5.17.0+, cipi/api 1.47+, abilities `nginx-view` / `nginx-manage`).

It is **not OPcache**. OPcache is on already on every Cipi server and keeps PHP's compiled code in memory: PHP no longer parses its files at every request, but it still runs — WordPress boots, loads its plugins, asks the database and builds the same page again for every visitor. With the page cache none of that happens for a page that is kept: it comes back in about a millisecond, and a small server holds a traffic peak that would have filled its PHP pool.

Never cached: anything but GET and HEAD, signed-in visitors (WordPress, WooCommerce carts, PHP and Laravel sessions), an `Authorization` header, `/wp-admin`, `wp-login.php`, `/wp-json`, a query string (tracking parameters apart), and whatever the app marks `private` / `no-cache` or gives a cookie. A Laravel app marks every response `no-cache, private` by default, so it caches only the routes it marks public itself. `X-Cache-Status` in the response says `HIT`, `MISS`, `BYPASS`, `EXPIRED` or `STALE`.

Cipi empties the cache after every successful deploy and rollback. In a deploy script, a cron or a plugin of your own — on the server, as the app's user:

```
sudo /usr/local/bin/cipi-cache-clear <app>
```

### Aliases

```
cipi-cli aliases list <app>                 List aliases
cipi-cli aliases add <app> <domain>         Add an alias
cipi-cli aliases remove <app> <domain> [-y] Remove an alias
```

Alias: `alias` → `aliases`.

### Databases

```
cipi-cli db engines                         List DB engines + default (4.8+); SERVER says MySQL where `cipi db switch mysql` ran (5.13.0+)
cipi-cli db install <engine>                Install an engine: mariadb|pgsql (5.0.6+)
cipi-cli db list                            List all databases (engine, size, app)
cipi-cli db create <name> [--engine] [--app] Create a database, optionally shown as an app's
cipi-cli db link <database> <app>           Tie a database to an app (5.9.0+)
cipi-cli db unlink <database>               Remove that tie (5.9.0+)
cipi-cli db delete <name> [-y]              Delete a database (API < 1.19 only)
cipi-cli db backup <name>                   Create a backup; prints the dump file on the server
cipi-cli db restore <name> --file <path>    Restore from a .sql.gz on the server [-y]
cipi-cli db restore <name> --upload <file>  Upload a local .sql / .sql.gz and restore it [-y] (5.9.0+)
cipi-cli db upload-limits                   Largest dump, chunk size and extensions the server takes (5.9.0+)
cipi-cli db password <name> [-y]            Regenerate password and update .env
cipi-cli db anonymize <app> --email <addr>  Request an anonymized dump via cipi/agent
cipi-cli db anonymize download <url> [-o F] Download the dump from the signed link
```

Aliases: `database`, `dbs` → `db`.  
API 1.19+ removed database deletion and setting the default engine: use `cipi db delete` / `cipi db default` on the host.

**App link:** `db create --app <app>` and `db link <database> <app>` show a database as an app's — `db list` gets an APP column (`shop (own)` for the app's own database, `shop (linked)` for a linked one) and `apps show` a Databases line. It is a label: the database keeps its own user and password and stays when the app is deleted; a database is linked to one app at most, and `db unlink` removes the tie. `--engine` picks the engine when the name exists on both. Requires Cipi 5.9.0+ / API 1.39+ and `dbs-manage`; on an older server `--app` is dropped, so after `db create --app` the CLI reads the list back and warns when the label did not stick.

**Restore:** `db restore` needs exactly one of `--file` (a `.sql.gz` already on the server, e.g. the file `db backup` printed) or `--upload` (a local `.sql` or `.sql.gz`). An upload is sent in chunks with a progress indicator, restored, and deleted on the server when the restore ends; a chunk that fails on a network or server error is sent again up to three times, and Ctrl-C cancels the upload on the server. The size limit is the server's (`db upload-limits`, 2 GB unless configured otherwise) — copy a larger dump to the server and use `--file`. The content must match the name: a `.sql.gz` must be gzip-compressed, a `.sql` must not. `--upload` requires Cipi 5.9.0+ / API 1.39+.

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

### Backups

```
cipi-cli backup                             Status (same as backup status)
cipi-cli backup status                      Destinations, profiles, last runs, what no profile covers
cipi-cli backup runs [--profile P]          The runs each destination holds, per profile
cipi-cli backup run [profile]               Start a run now, in the background
cipi-cli backup verify [profile]            Check that the newest run opens (async job)
cipi-cli backup profile add <name> --dest local,s3 [flags]
cipi-cli backup profile edit <name> [flags]
cipi-cli backup profile enable|disable <name>
cipi-cli backup profile remove <name> [-y]
cipi-cli backup destination test [name]     Can the server reach each destination; exits 1 when one fails
cipi-cli backup configure                   Set up backups on the server (5.19.0+)
cipi-cli backup destination add|edit|key|remove …        S3 buckets (5.19.0+) and SFTP servers (5.18.0+), on a par
cipi-cli backup key show|rotate             The encryption key (5.19.0+)
cipi-cli backup prune [--profile P] [--dry-run]          Apply the retention now (async job, 5.19.0+)
```

Aliases: `backups` → `backup`, `profiles` → `profile`, `destinations` / `dest` → `destination`, `backup list` → `backup runs`.

The server backups of `cipi backup` on the host. A profile says what a run takes, when, where to and for how long it is kept; `backup status` shows each profile with its schedule, retention, last run and state (enabled, disabled, overdue, running), the reason of a failed run, and the apps and databases no enabled profile covers. `backup run` only starts the run — follow it with `backup status`. `backup verify` waits for the job and prints what the server reported.

**Profile flags:** `--scope all|files|db`, `--apps`, `--databases`, `--exclude-databases`, `--exclude-tables` (comma-separated names or globs, e.g. `--exclude-tables "*.jobs,*.sessions"`), `--exclude-paths` (paths left out of the app archives, relative to the app: `storage/app/uploads,*.mp4`, `shop:public/media` for one app only — Cipi 5.13.0+ / API 1.43+), `--every 30m|6h|1d` or `--cron "0 2 * * *"` (not both), `--dest local,s3,<named>` (required on `add`), `--keep`, `--keep-days`, `--keep-weeks`, `--encrypt` (client-side AES-256; `--encrypt=false` turns it off). `profile edit` sends only the flags you pass; an empty exclusion list (`--exclude-tables ""`, `--exclude-paths ""`) clears it. The three `--keep` flags are one retention policy: giving any of them sets the others to 0, so pass together the ones you want.

Setting backups up and their destinations: below (*Backups set up from here*, *Backups to SFTP*). `backup fetch` (download and decrypt a run) and `profile remove --purge` are run on the server.

Requires Cipi 5.9.0+, API 1.39+ and the `backups-view` (status, runs) / `backups-manage` (everything else) token abilities — existing tokens do not get them.

### PHP

```
cipi-cli php list                           Installed versions, FPM status, default (5.0.6+)
cipi-cli php install <version>              Install PHP 8.3 / 8.4 / 8.5 / 8.6
cipi-cli octane show <app>                  Octane: which PHP answers requests, which runs the rest (5.17.0+)
cipi-cli octane php <app> <version>         Put an Octane app on a PHP version, by its number
cipi-cli octane restart <app>               Restart Octane on its build and check that it answers
cipi-cli octane builds                      The FrankenPHP build of every Octane app (5.10.0+)
cipi-cli octane upgrade [app]               Newest FrankenPHP build on the app's own PHP
```

Switching the default PHP version and removing versions are host-only (`cipi php switch|remove`).

An **Octane** app runs two PHP: the one built into its FrankenPHP binary answers the requests, the server's `php<X.Y>` runs Composer, artisan, cron and the queue workers. `octane show` prints both and, when they differ, the command that puts them on one. `octane php <app> 8.5` takes the version by its number, as for any other app: the FrankenPHP build that carries it is activated (and fetched from FrankenPHP's releases when the server has none), Octane is restarted and checked — the previous build comes back when it does not answer — then the rest of the app moves. That PHP has to be installed on the server (`php install`). Requires Cipi 5.17.0+, cipi/api 1.47+ and `php-view` / `php-manage`.

A server installed before Cipi 4.5.4 can still have PHP 7.4–8.2. `php list` shows them marked `(legacy)`: the apps on them keep running and can be moved to a current version (`apps edit <app> --php 8.5`), and no app can be created on or moved to one — the API answers `422`.

### Server services & SSH keys

```
cipi-cli services list [name]               System services and status (5.0.6+)
cipi-cli services restart <name>            Restart a service (nginx, mariadb, …)
cipi-cli valkey                             Valkey per app: own, shared, external (5.17.0+)
cipi-cli valkey isolate <app> [--no-copy] [-y]   Give an app its own Valkey instance
cipi-cli valkey isolate --all [-y]          The same for every app still on the shared instance
cipi-cli valkey share <app> [-y]            Move it back to the shared instance
cipi-cli valkey restart <app>               Restart an app's own instance
cipi-cli valkey flush-shared [-y]           Empty the shared instance and change its password
cipi-cli ssh-keys list                      Authorized keys of the cipi user (5.0.6+)
cipi-cli ssh-keys add [key] [--file F]      Add a public key (inline or from a .pub file)
cipi-cli ssh-keys remove <id> [-y]          Remove a key by id
```

Aliases: `service` → `services`, `ssh` → `ssh-keys`.

**Valkey per app:** a Laravel app keeps cache, sessions and queues in Valkey. On the server's shared instance every app holds the same password, so each can read and write the others' keys; an app with an instance of its own cannot. `valkey isolate` copies the app's keys with the app in maintenance mode for the seconds that takes, switches its `.env` and restarts its workers (`--no-copy` starts it empty). Once nobody is left on the shared instance, `valkey flush-shared` empties it and changes its password — the new one stays on the server (`cipi credentials`): no password ever travels through the API. Requires Cipi 5.17.0+, cipi/api 1.47+ and `services-view` / `services-manage`.

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

### Metrics & health history

Cipi 5.11.0+ with cipi/api 1.41.0+ (abilities `metrics-view`, `metrics-manage`).

```bash
cipi-cli metrics                                  Server and apps now, last-hour trend, uptime 24h/30d, recent events
cipi-cli metrics show [--app shop] [--metrics cpu,mem_mb] [--range 7d]
cipi-cli metrics uptime [shop] [--range 30d]      Uptime, minutes down, incidents, per day
cipi-cli metrics events [--app] [--source deploy] [--range 7d] [--limit 50]
cipi-cli metrics settings                         Collector, database, retention, heartbeat, OpenMetrics
cipi-cli metrics retention --raw 7 --rollup 30 --resolution 15 --events 90
cipi-cli metrics probe on|off                     Probe each app's healthcheck URL every minute
cipi-cli metrics collector on|off
cipi-cli metrics heartbeat set <url> [--every 1]  An outside service alerts when the calls stop
cipi-cli metrics heartbeat off|test
cipi-cli apps basicauth agent <app> [public|protected]   cipi/agent routes outside basic auth or not
```

### Notifications

Cipi 5.12.0+ with cipi/api 1.42.0+ (abilities `notifications-view`, `notifications-manage` — or `smtp-view`, `smtp-manage`). Every enabled trigger reaches every enabled channel.

```bash
cipi-cli notifications                                  Channels (SMTP first) and how many triggers are on
cipi-cli notifications add slack ops --url https://hooks.slack.com/services/… [--test]
cipi-cli notifications add telegram oncall --token 123456:ABC… --chat-id -1001234
cipi-cli notifications add resend mail --api-key re_… --from alerts@example.com --to ops@example.com
cipi-cli notifications add webhook siem --url https://example.com/hooks/cipi --secret '…'
cipi-cli notifications add smtp --host … --user … --password … --from … --to …
cipi-cli notifications update <id> [--to …] [--clear-secret] [--enable|--disable]   Flags left out (secrets too) are kept
cipi-cli notifications enable|disable|remove <id>
cipi-cli notifications test [<id>|all]                  Exits 1 when the channel refuses it
cipi-cli notifications triggers                         Every trigger by category, on or off
cipi-cli notifications triggers off deploy_success ssh_login
cipi-cli notifications triggers all-off --except deploy_fail,backup_fail,health_fail
cipi-cli notifications triggers reset
cipi-cli notifications settings                         The time zone and the server name notifications use (5.17.0+)
cipi-cli notifications timezone Europe/Rome             The Time line of every notification ("server": the server's own)
cipi-cli notifications server-name production           "production (vmi12334)" instead of the hostname alone (--clear: back)
```

Types: smtp, resend, ses, slack, discord, mattermost, googlechat, telegram, ntfy, pushover, webhook. `cipi-cli smtp …` keeps working.

### Status

```
cipi-cli status                             Global overview — one row per server
cipi-cli status <profile>                   Full details for one server
cipi-cli <profile> status                   Same, via profile prefix
```

Global columns: NAME, IP, CPU, RAM, HDD, APPS, SVC, CIPI, API.  
API is the installed cipi/api release (`Cipi API` in the detail view); servers on cipi/api older than 1.39.1 do not report it and show `—`.  
Requires the API token ability `status-view` (`GET /api/status`, same data as `cipi status` on the host).

### Disk

```
cipi-cli disk                               Disk usage: the server filesystem, then every app (GB and %)
cipi-cli disk db                            Every database per engine, in MB
```

Aliases: `dbs`, `databases` → `disk db`.  
The figures of `cipi disk` on the host: an app is its home (releases, shared storage, logs) plus its database, largest first; the LIMIT column shows the soft limit set with `cipi app limits <app> --disk=<GB>` and flags apps over it. `disk db` lists MariaDB and PostgreSQL databases in MB, Valkey databases with their keys and memory, Meilisearch indexes with their documents. Sizes are measured by the server when asked, so the CLI waits up to three minutes. Requires Cipi 5.5.2+ (API sudoers), API 1.33+ and the `disk-view` ability.

### Jobs

```
cipi-cli jobs show <id>                     Show job status
cipi-cli jobs wait <id>                     Wait for a job to complete
cipi-cli run-commands                       List whitelisted app run binaries (5.0.3+)
```

Alias: `job` → `jobs`.  

`jobs show` and `jobs wait` need the ability the job was created under (cipi/api 1.36.1+): `deploy-manage` for a deploy, `dbs-manage` for a database password, `apps-create` for an app create, and so on. The token that queued a job can always read it; any other gets `404 Job not found`.
Most write commands wait for jobs automatically; use these to inspect a job by ID. When a job fails, the CLI prints the reason the server recorded; with `--json`, `jobs show` and `jobs wait` include the job's `output` when the server sends it.

### Search, Node, and host insights

```
cipi-cli search status                      Meilisearch status (5.2.2+)
cipi-cli node list                          Installed Node runtimes (5.4.0+)
cipi-cli node install <major>               Install a Node major, e.g. 24 (5.9.0+)
cipi-cli node upgrade [major] [--restart]   Latest patch of one major, or of every installed one (5.9.0+)
cipi-cli node default <major|system>        Set the server-wide Node (5.9.0+)
cipi-cli node remove <major> [-y]           Remove a Node major (5.9.0+)
cipi-cli monitor                            System monitor checks (5.3.0+)
cipi-cli packages                           Optional host packages: the catalog and what is installed
cipi-cli packages install <id>              Install an entry, e.g. browsershot, libreoffice (5.14.0+)
cipi-cli packages remove <id> [-y]          Remove an entry, and only what nothing else needs (5.14.0+)
cipi-cli zt                                 Zero Trust status: Cloudflare Tunnel (5.3.0+) and the mesh network — Tailscale, NetBird (5.18.0+)
cipi-cli zt join <tailscale|netbird>        Put the server on a mesh network (5.18.0+)  [--key-file --hostname --tags --control-url --direct --adopt …]
cipi-cli zt lock <ssh|http> [-y]            Close SSH, or 80/443, to the world: open on the mesh only (5.18.0+)
cipi-cli zt unlock <ssh|http>               Reopen it (5.18.0+)
cipi-cli zt ssh <on|off>                    Tailscale SSH (5.18.0+)
cipi-cli zt leave [-y]                      Take the server off the network: reopens what the locks closed (5.18.0+)
```

`zt` reads both sides of Zero Trust. Cloudflare Tunnel publishes the sites; a mesh network — Tailscale or NetBird — is how the people who run the server reach it. The tunnel's lines come first (state as Cloudflare sees it, locks, how many hostnames); then the mesh: connected or not and the client's own word for why, the server's address and name on the network, peers online, the node key's expiry (Tailscale), the command to SSH in over it and the last SSH login the server saw come over the mesh, whether port 22 and 80/443 are closed to the world, the server's warnings, how this client reaches the server (`via mesh`, `tunnel`, `public`…) and, on `refused` lines, what the server would not do from here and why. `--json` has everything the server said, hostnames and Access included.

**The mesh is managed from here too** (Cipi 5.18.0+, cipi/api 1.48+, ability `zt-manage`); Cloudflare's tunnel is changed at the server's shell (`cipi zt`).

```bash
cipi-cli zt join tailscale --tags cipi          # asks for the auth key; a tagged machine's key never expires
cipi-cli zt join netbird --key-file ~/nb.key --control-url https://netbird.example.com
cipi-cli zt                                     # the address on the network: ssh cipi@100.x.y.z once, from a device of it
cipi-cli zt lock ssh                            # port 22 from the network only
cipi-cli zt lock http                           # 80/443 too — private sites, or sites published through Cloudflare
cipi-cli zt unlock http
cipi-cli zt leave
```

- **`zt join`** installs the client on the server, signs in and lets the mesh through the firewall; it closes nothing and takes a minute or two. The key is asked for at a prompt, read from `--key-file`, or given with `--key` (which leaves it in the shell's history); the server uses it once and keeps it nowhere. `--adopt` takes a client that is already installed and connected on the server. Options: `--hostname`, `--control-url` (a Headscale, a self-hosted NetBird), `--direct` (also open the client's UDP port), `--accept-dns`, `--accept-routes`, and for Tailscale `--tags`, `--ssh`, `--reauth`.
- **`zt lock ssh|http`** asks for confirmation (`-y` skips it). **The server refuses what would lock people out, and says why** — there is no `--force` over the API: `ssh` until sshd has accepted a login that came over the mesh, while the node key can expire, with the firewall off, or with Tailscale SSH on; `http` while an app renews its certificate over HTTP-01, and when this client reaches the API on the public address the lock would close. For that last one, point the profile's endpoint at the server's mesh name or address (or at its hostname on the Cloudflare tunnel) and run it again.
- **`zt leave`** is refused when this client reaches the server over the mesh itself; that is done at the server's shell.
- **`zt ssh on|off`** is Tailscale SSH: the tailnet's policy says who signs in. Tailscale only.

`zt-manage` is as strong as `ssh-manage` — whoever holds it can put the server on a network of their own. A token made with a list of abilities does not have it until it is created again with `zt-manage` in the list; a `--full-access` token has it.

`node install|upgrade|default|remove` are async jobs and take even (LTS) majors only — the CLI rejects anything else before calling the server. `node upgrade --restart` moves the SSR apps on the upgraded runtime to the new binary blue/green; without it they take it at their next deploy. Requires Cipi 5.9.0+, API 1.39+ and the `node-runtimes-manage` ability — existing tokens do not get it.

`packages install|remove` are async jobs on the server's **closed catalog** (Cipi 5.14.0): `image-optimizers`, `avif`, `ffmpeg`, `imagemagick`, `poppler-utils`, `browsershot` (the libraries and fonts the Chrome Puppeteer downloads needs — Chrome itself comes with `npm install puppeteer` in the app), `fonts-cjk`, `pdftk`, `libreoffice` (headless), `pandoc`, `tesseract`, and the security scanners [Hackly](https://github.com/andreapollastri/hackly) runs: `sec-recon` (nmap, dig, whois) and `sec-gitleaks` from Ubuntu; `sec-nuclei`, `sec-zap`, `sec-nikto`, `sec-wpscan`, `sec-trivy`, `sec-semgrep`, `sec-checkov` as upstream releases Cipi pins by version and SHA-256. The list shows what each entry brings (`ffmpeg, ffprobe`, or *libraries*), its VERSION (`ubuntu`, or the pinned one — `2.16.1 → 2.17.0` when an older one is installed) and, for a partial install, what is missing — `install` completes a partial entry and updates an outdated one. `remove` asks for confirmation unless `-y`; a library entry keeps whatever other software still uses. Requires Cipi 5.14.0+, API 1.44+ and the `packages-manage` ability.

### Backups to SFTP

```
cipi-cli backup destination add <name> --host <h> --user <u> [--port 22] [--path <dir>]   An SFTP destination (5.18.0+)
cipi-cli backup destination edit <name> [--host] [--port] [--user] [--path] [--new-key]
cipi-cli backup destination key <name>           The public key the remote must accept
cipi-cli backup destination test [name]          Write and remove a file there
cipi-cli backup destination remove <name> [-y]   One no profile uses (its archives stay)
```

A server reached over SSH is a backup destination next to `local` and the S3 ones: a VPS with a user of its own for backups, a **Hetzner Storage Box** (`--port 23`), a NAS. Cipi makes a key for it on the server and prints its public half with how to install it — `ssh-copy-id -p 23 -s …` for a Storage Box, the `authorized_keys` line elsewhere. No password is asked for or sent; only SFTP is asked of the remote. A profile names it like any other: `--dest local,box`.

### Backups set up from here

```
cipi-cli backup configure                                       On the server: the local folder and a nightly profile
cipi-cli backup destination add <name> --bucket <b> --key <id> [--region R] [--endpoint URL] [--use P,…]   An S3 bucket
cipi-cli backup destination add <name> --host <h> --user <u> [--port 22] [--path <dir>] [--use P,…]        An SFTP server
cipi-cli backup destination edit <name> [--bucket] [--key] [--region] [--endpoint URL|none] [--use P,…]
cipi-cli backup key show                                        The encryption key (logged and notified on the server)
cipi-cli backup key rotate [-y]                                 A new key for the runs to come
cipi-cli backup prune [--profile P] [--dry-run]                 Apply the retention now
```

Backups start on the server: `configure` sets up the local folder and a nightly profile that keeps every app and database there. **S3 buckets and SFTP servers are then destinations on a par**, each with a name (`s3`, `offsite`, `sftp`, `box`…); `--use default` sends those profiles there too (Cipi 5.19.1+). **A secret key is never a flag** (it would stay in the shell's history): it is asked at a prompt, not shown as it is typed, or read from the first line of `--secret-file <path>` or of standard input (`--secret-stdin`), and sent once — the server reads it on its standard input, and no answer carries it back. On a change, a key or a secret left out stays as the server has it; a new `--key` asks for the secret that goes with it. **Every bucket is reached with its keys before anything is saved.** `--endpoint` for every provider but AWS: Hetzner `https://fsn1.your-objectstorage.com`, Cloudflare R2 `https://<account-id>.r2.cloudflarestorage.com` (`--region auto`), DigitalOcean `https://fra1.digitaloceanspaces.com`, Backblaze B2 `https://s3.eu-central-003.backblazeb2.com`, Scaleway `https://s3.fr-par.scw.cloud`, MinIO. The local and temp folders are set on the server. Requires Cipi 5.19.0+, API 1.49+ and `backups-manage`.

### How the server updates Cipi

```
cipi-cli updates                                        What is set, trusted, waiting (5.18.0+)
cipi-cli updates --delay 12h                            Wait before installing a new release (hours, days; off)
cipi-cli updates --verify on --fingerprint SHA256:…     Only releases signed by a trusted key
cipi-cli updates --verify off --delay off
```

Both safeguards are off by default: every night the server installs the latest Cipi, as before. **Signed releases are not available yet**: Cipi releases are not signed at the moment, so the server refuses `--verify on` and `updates` says so — use `--delay` meanwhile. Once they are, `--verify on` installs a release only when its tag `v<version>` is signed by a key the server trusts and points at what was downloaded; the first trust takes the fingerprint of a shipped key, compared with the one published at [cipi.sh](https://cipi.sh/docs/self-update#signers). `--delay` installs a new release once the server has seen it for that long. Starting an update is not done from cipi-cli. Abilities `updates-view`, `updates-manage`.

### Transfers between servers

```
cipi-cli sync push [app...] --to <profile>      Send apps to another server of yours and import them there (5.18.0+)
cipi-cli sync push [app...] --host <address>    …to a server that is not a profile (it must trust this server's key)
                                                [--port 22 --with-db --with-storage --keep-trust -y]
cipi-cli sync check <host> [--port 22]          Can this server send to that one: key accepted, Cipi answers
cipi-cli sync key [--raw]                       This server's sync key
cipi-cli sync trust [key]                       Let another server send apps here  [--from <profile> | --key-file <file>]
cipi-cli sync untrust [key]                     Take that key back out             [--from <profile> | --key-file <file>]
```

`sync` copies apps from one Cipi server to another — a move to a bigger machine, production copied onto staging, a second server kept ready. **The server of the profile in use is the source**: it exports the apps (settings, `.env`, deploy recipe, workers, crons — and databases and uploaded files when asked), encrypts the archive, sends it over SSH and has the target import it. Apps that are not on the target are created there; apps that are, updated. Nothing is removed or stopped on the source, and DNS is yours to move.

```bash
cipi-cli prod sync push --to staging --with-db            # every app of prod, onto the server of the staging profile
cipi-cli prod sync push shop blog --to staging --with-db --with-storage
cipi-cli prod sync push --to new --host 10.0.0.12         # same, over a private address
cipi-cli prod sync push shop --host 203.0.113.9           # a server that is not a profile
```

- **`--to <profile>` does the preparation**: this server's key is added on the target, the way is checked, the transfer runs, and the key is taken out again when it ends — whatever its end. `--keep-trust` leaves it, for transfers that repeat; a key that was trusted before is left where it was. The address is the host of that profile's endpoint unless `--host` names another (a private or mesh address, or the real one of a server whose API sits behind a proxy).
- **`--host` alone** is for a server this client has no profile for. It has to trust the source first: `cipi-cli sync key` here, `cipi sync trust` there.
- **Nothing starts before a check**: the source is asked whether the target accepts its key and runs a Cipi that can take the transfer, and the answer is said in words — not trusted yet, no SSH answer at that address, a host key that changed, SSH but no Cipi, a Cipi too old. `sync check` asks the same without sending anything.
- **Encryption is the server's**: the archive is encrypted (AES-256) with a passphrase the source makes for that one transfer and hands to the target over the same SSH connection. There is no flag for it: it is never shown and never passes through this client or the API. The archive is removed from both servers when the import ends.
- **`--with-db`** dumps the databases on the source and restores them on the target, **replacing the database of an app that is already there**. **`--with-storage`** adds each app's `shared/storage`, which can be large. Without either, an app arrives with its settings and an empty database.
- **Laravel and custom apps travel; Node apps are skipped** — `cipi sync` does not recreate them yet. Create them on the target (`apps create --node …`): the code comes from Git with the first deploy.
- The transfer is an async job, followed for up to two hours; its output is printed when it ends. `-y` skips the confirmation.
- **`sync trust` gives the holder of that key a shell as `cipi` on the server**: only the key of a server of yours. `--from <profile>` reads the key from that server, so nothing is copied by hand: `cipi-cli staging sync trust --from prod`.

Requires Cipi 5.18.0+ on both servers, cipi/api 1.48+ and the `sync-manage` ability — on the source, and on the target for `--to`, `trust` and `untrust`. `sync-manage` is as strong as `ssh-manage`: whoever holds it can have the server send its apps, databases included, to a host of their choosing. Exports to a file (`cipi sync export|import|list`) stay at the server's shell.

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

**How long it waits:** as long as the server lets the job run, and a minute more — two hours for `deploy` (the frontend build of a release alone has 30 minutes, and up to an hour where the app asks for it), ten minutes for every other command. A job the API stops there ends as failed and the CLI says why. If the wait itself is over first, the command says that the job is still running and how to pick it up again: `cipi-cli jobs wait <id>`, which waits for as long as a deploy may take. (Before, the CLI stopped after 120 polls — under nine minutes — and a deploy with a long build "timed out after polling" while it was still going.)

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

A token only has the abilities it was created with: a feature added later (backups, Node runtimes, …) answers `403` until you create a new token that includes its ability. On Cipi 5.9.0+ `cipi api token create --full-access` creates a token with every ability, present and future.

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
| Disk usage (`disk`, `disk db`) | 5.5.2 (API sudoers) | 1.33+ |
| `apps create --node-build`, repository access check on create/edit | 5.6.0 | 1.34+ |
| cipi.yml (`yml plan`, `yml apply`, `yml auto`) | 5.6.4 | 1.35+ (`yml-view` / `yml-manage`) |
| Environment labels (`apps create/edit --env`, ENV column, `yml` env + file, `cipi-<env>.yml`) | 5.8.0 | 1.36+ |
| Deploy status (`deploy status`) | 5.8.4 | 1.37+ |
| Deploy history (`deploy history`, `deploy last`) | 5.9.0 | 1.39+ |
| Node runtimes (`node install`, `upgrade`, `default`, `remove`) | 5.9.0 | 1.39+ (`node-runtimes-manage`) |
| Backups (`backup …`) | 5.9.0 | 1.39+ (`backups-view` / `backups-manage`) |
| Database app link (`db create --app`, `db link`, `db unlink`, APP column) | 5.9.0 | 1.39+ |
| Dump upload (`db restore --upload`, `db upload-limits`) | 5.9.0 | 1.39+ |
| API release in `status` (API column, `Cipi API`) | — | 1.39.1+ |
| Anonymized dumps (`db anonymize`) | 5.0.3 | 1.14+ (plus cipi/agent 1.1.1+ in the app) |
| App logs (`apps logs`) | — | 1.11.9 |
| Server status (`status`) | — | with `GET /api/status` + `status-view` |
| Global domain map (`domains`) | 4.5.5 | — (built from `/api/apps`) |
| Aliases add/remove path | — | 1.31+ (`/aliases/{domain}`) |
| `.env` order (`apps env order`, `apps env set --order`), paused commit in `deploy status`, `backup profile --exclude-paths`, MySQL in `db engines`, basic auth agent in `apps basicauth status` | 5.13.0 | 1.43+ |
| Optional packages install / remove (`packages install`, `packages remove`), PROVIDES, VERSION and missing in `packages` | 5.14.0 | 1.44+ (`packages-manage`) |
| Page cache (`cache`), Octane PHP (`octane show|php|restart`), Valkey per app (`valkey`), notification time zone and server name (`notifications settings|timezone|server-name`) | 5.17.0 | 1.47+ (no new ability: `nginx-*`, `php-*`, `services-*`, `notifications-*`) |
| The mesh network in `zt` (Tailscale, NetBird: address, peers, node key, locks, warnings) | 5.18.0 | 1.48+ (`zt-view`, no new ability) |
| Managing the mesh (`zt join`, `zt leave`, `zt lock`, `zt unlock`, `zt ssh`) | 5.18.0 | 1.48+ (`zt-manage`) |
| Transfers between servers (`sync push`, `sync check`, `sync key`, `sync trust`, `sync untrust`) | 5.18.0 | 1.48+ (`sync-manage`) |
| SFTP backup destinations (`backup destination add|edit|key|remove`) | 5.18.0 | 1.48+ (`backups-manage`) |
| Backups set up from here (`backup configure`), S3 destinations, `backup key show|rotate`, `backup prune`; "not available yet" for signed releases in `updates` | 5.19.0 | 1.49+ (`backups-manage`) |
| A destination named `s3` like any other, `--use` (send profiles there) | 5.19.1 | 1.49+ (`backups-manage`) |
| Update policy (`updates`) | 5.18.0 | 1.48+ (`updates-view`, `updates-manage`) |

New app PHP versions must be **8.3**, **8.4**, **8.5** (Cipi 4.5.4+), or **8.6** (Cipi 5.9.1+ / API 1.39.2+).

## License

MIT
