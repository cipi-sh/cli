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

### 1. Configure

Set up the connection to your Cipi API server:

```bash
cipi-cli configure
```

You will be prompted for:

- **API endpoint** — the URL of your Cipi API (e.g. `https://api.example.com`)
- **Token** — a Sanctum token created with `cipi api token create` on your server

Credentials are stored in `~/.cipi/config.json` (permissions `0600`).

You can also pass values directly:

```bash
cipi-cli configure --endpoint https://api.example.com --token "1|yourtoken..."
```

### 2. Use

```bash
cipi-cli apps list
cipi-cli apps show myapp
cipi-cli deploy myapp
cipi-cli ssl install myapp
```

## Commands

### Apps

```
cipi-cli apps list                          List all applications
cipi-cli apps show <name>                   Show application details
cipi-cli apps create [flags]                Create a new application
cipi-cli apps edit <name> [flags]           Edit an application
cipi-cli apps delete <name> [-y]            Delete an application
```

**Create flags:** `--name`, `--domain`, `--php`, `--repository`, `--branch`, `--custom`, `--docroot`

**Edit flags:** `--php`, `--repository`, `--branch`, `--domain`

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

### Jobs

```
cipi-cli jobs show <id>                     Show job status
cipi-cli jobs wait <id>                     Wait for a job to complete
```

### Configuration

```
cipi-cli configure                          Set up API endpoint and token
cipi-cli configure show                     Show current configuration
```

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
git tag v0.1.0
git push origin v0.1.0
```

The pipeline builds binaries for Linux (amd64/arm64) and macOS (amd64/arm64), generates SHA-256 checksums, and creates a GitHub Release with all artifacts attached.

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

## License

MIT
