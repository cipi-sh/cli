package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Backups set up and managed from here (cipi/api 1.49, Cipi 5.19.0): where
// they go — this server only, the default bucket or an SFTP server — S3
// buckets as destinations of their own, the encryption key, the retention.
// A secret key is read from a prompt, a file or stdin — never a flag — and
// sent once: the API writes it to the CLI's standard input on the server.

const backupSetupAPI = "1.49"

var (
	s3BucketRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)
	s3RegionRe   = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	s3EndpointRe = regexp.MustCompile(`^https?://[A-Za-z0-9.:/_-]{1,255}$`)
	// A key or a secret: printable, no space, no quote, no backslash — the
	// server writes them to /root/.aws/credentials.
	s3CredRe = regexp.MustCompile(`^[\x21\x23-\x26\x28-\x5b\x5d-\x7e]{1,256}$`)
)

// s3Body is the body of an S3 destination or of the default bucket: only what
// is given. A new one needs its bucket, key and secret; a change keeps what is
// left out. An endpoint given empty on a change is "none" (back to AWS).
func s3Body(bucket, key, secret, region, endpoint string, endpointSet, create bool) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if create && (bucket == "" || key == "" || secret == "") {
		return nil, fmt.Errorf("an S3 bucket needs --bucket, --key and its secret key (asked, or --secret-file / --secret-stdin)")
	}
	if bucket != "" {
		if !s3BucketRe.MatchString(bucket) {
			return nil, fmt.Errorf("--bucket: lowercase letters, digits, . _ - (up to 63)")
		}
		body["bucket"] = bucket
	}
	if key != "" {
		if !s3CredRe.MatchString(key) {
			return nil, fmt.Errorf("--key: printable characters, no space, no quote")
		}
		body["key"] = key
	}
	if secret != "" {
		if !s3CredRe.MatchString(secret) {
			return nil, fmt.Errorf("the secret key: printable characters, no space, no quote")
		}
		body["secret"] = secret
	}
	if region != "" {
		if !s3RegionRe.MatchString(region) {
			return nil, fmt.Errorf("--region: letters, digits and dashes — eu-central-1, fsn1, auto")
		}
		body["region"] = region
	}
	if endpointSet {
		switch {
		case endpoint == "" || endpoint == "none":
			if !create {
				body["endpoint"] = "none"
			}
		case strings.Contains(endpoint, "<"):
			return nil, fmt.Errorf("--endpoint: fill in the part in angle brackets")
		case !s3EndpointRe.MatchString(endpoint):
			return nil, fmt.Errorf("--endpoint is an https:// URL — none for AWS")
		default:
			body["endpoint"] = endpoint
		}
	}
	return body, nil
}

// s3Secret is the secret key: the first line of --secret-file or of stdin
// (--secret-stdin), or typed at a prompt when asked for one. Never a flag of
// its own: it would stay in the shell's history.
func s3Secret(cmd *cobra.Command, ask bool) (string, error) {
	file, _ := cmd.Flags().GetString("secret-file")
	fromStdin, _ := cmd.Flags().GetBool("secret-stdin")
	if file != "" && fromStdin {
		return "", fmt.Errorf("one of --secret-file and --secret-stdin")
	}
	var data []byte
	var err error
	switch {
	case file != "":
		data, err = os.ReadFile(file)
	case fromStdin:
		data, err = io.ReadAll(io.LimitReader(os.Stdin, 4096))
	case ask:
		return strings.TrimSpace(output.ReadSecret("Secret access key")), nil
	default:
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the secret key: %w", err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	if line = strings.TrimSpace(line); line == "" {
		return "", fmt.Errorf("no secret key: the file or stdin is empty")
	}
	return line, nil
}

// s3FlagsGiven: the flags of an S3 destination were used (add and edit tell
// an S3 bucket from an SFTP server by them).
func s3FlagsGiven(cmd *cobra.Command) bool {
	for _, f := range []string{"bucket", "key", "region", "endpoint", "secret-file", "secret-stdin"} {
		if cmd.Flags().Changed(f) {
			return true
		}
	}
	return false
}

func s3Save(cmd *cobra.Command, name string, create bool) error {
	bucket, _ := cmd.Flags().GetString("bucket")
	key, _ := cmd.Flags().GetString("key")
	region, _ := cmd.Flags().GetString("region")
	endpoint, _ := cmd.Flags().GetString("endpoint")
	if create && !cmd.Flags().Changed("region") {
		region = "eu-central-1"
	}
	// A new access key goes with its secret: asked, unless read from a file or stdin.
	secret, err := s3Secret(cmd, create || key != "")
	if err == nil {
		var body map[string]interface{}
		body, err = s3Body(bucket, key, secret, region, endpoint, cmd.Flags().Changed("endpoint"), create)
		if err == nil {
			if create {
				if !sftpNameRe.MatchString(name) || name == "local" || name == "s3" {
					err = fmt.Errorf("name: lowercase letters, digits and dashes, 2 to 32 — not local or s3 (the default bucket is \"backup configure\")")
				}
				body["type"], body["name"] = "s3", name
			} else if name == "s3" {
				err = fmt.Errorf("'s3' is the default bucket: cipi-cli backup configure --type s3 …")
			}
			if err == nil {
				return s3Send(name, body, create)
			}
		}
	}
	output.Error("%s", err)
	return err
}

func s3Send(name string, body map[string]interface{}, create bool) error {
	client, err := mustClient()
	if err != nil {
		return err
	}
	if create || body["secret"] != nil || body["key"] != nil || body["bucket"] != nil {
		output.Info("Reaching the bucket with its keys — nothing is saved if it does not answer...")
	}
	var answer struct {
		Data struct {
			Name     string  `json:"name"`
			Bucket   string  `json:"bucket"`
			Region   string  `json:"region"`
			Endpoint *string `json:"endpoint"`
		} `json:"data"`
	}
	if create {
		err = client.Post("/api/backups/destinations", body, &answer)
	} else {
		err = client.Put("/api/backups/destinations/"+name, body, &answer)
	}
	if err != nil {
		output.Error("Failed to save the destination: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
		return err
	}
	if jsonFlag {
		output.PrintJSON(answer)
		return nil
	}
	d := answer.Data
	where := "s3://" + d.Bucket
	if d.Endpoint != nil && *d.Endpoint != "" {
		where += " at " + *d.Endpoint
	}
	if create {
		output.Success("S3 destination %s added: %s", name, where)
		output.Info("Name it in a profile: cipi-cli backup profile edit <profile> --dest local,%s", name)
	} else {
		output.Success("S3 destination %s updated: %s", name, where)
	}
	fmt.Println()
	return nil
}

// setupBody is the body of PUT /api/backups/config: where backups go.
func setupBody(cmd *cobra.Command) (map[string]interface{}, error) {
	typ, _ := cmd.Flags().GetString("type")
	if lo, _ := cmd.Flags().GetBool("local-only"); lo {
		if typ != "" && typ != "local" {
			return nil, fmt.Errorf("--local-only is --type local")
		}
		typ = "local"
	}
	if typ == "" {
		switch {
		case s3FlagsGiven(cmd):
			typ = "s3"
		case cmd.Flags().Changed("host") || cmd.Flags().Changed("user"):
			typ = "sftp"
		default:
			return nil, fmt.Errorf("say where backups go: --type local, --type s3 --bucket … --key …, or --type sftp --host … --user …")
		}
	}
	switch typ {
	case "local":
		return map[string]interface{}{"type": "local"}, nil
	case "s3":
		bucket, _ := cmd.Flags().GetString("bucket")
		key, _ := cmd.Flags().GetString("key")
		region, _ := cmd.Flags().GetString("region")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		if bucket == "" {
			return nil, fmt.Errorf("--type s3 needs --bucket (and --key, and the secret key on a first setup)")
		}
		// The secret is asked when a key is given: a new key goes with its secret.
		secret, err := s3Secret(cmd, key != "")
		if err != nil {
			return nil, err
		}
		body, err := s3Body(bucket, key, secret, region, endpoint, cmd.Flags().Changed("endpoint"), false)
		if err != nil {
			return nil, err
		}
		body["type"] = "s3"
		return body, nil
	case "sftp":
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		user, _ := cmd.Flags().GetString("user")
		path, _ := cmd.Flags().GetString("path")
		name, _ := cmd.Flags().GetString("name")
		body, err := sftpBody(name, host, port, user, path, true, true, true)
		if err != nil {
			return nil, err
		}
		body["type"] = "sftp"
		return body, nil
	}
	return nil, fmt.Errorf("--type is local, s3 or sftp")
}

var backupConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set up backups: this server only, an S3 bucket or an SFTP server",
	Long: `Say where backups go — "cipi backup configure" on the server. A first setup
makes the nightly profile, sent there: every app and every database.

  cipi-cli backup configure --type s3 --bucket acme-backups --key AKIA… --endpoint https://fsn1.your-objectstorage.com
  cipi-cli backup configure --type sftp --host u1234.your-storagebox.de --port 23 --user u1234 --path cipi
  cipi-cli backup configure --type local

s3: the default bucket — "s3" in a profile's destinations. The server reaches
it with the keys before it saves anything. The secret key is asked at a
prompt (--secret-file or --secret-stdin to read it) and sent once: it never
rides a command line, on this machine or on the server. On a server already
set up, a key or a secret left out stays as it is; --region or --endpoint
alone change just that ("--endpoint none": back to AWS).

sftp: an SFTP server — "sftp" in profiles (--name for another). Cipi makes
an SSH key for it and this prints it with how to install it; then
"cipi-cli backup destination test sftp". No password.

local: this server only. Such a copy is lost with the server.

The local and temp folders are set on the server.

Requires Cipi 5.19.0+, cipi/api 1.49+ and the backups-manage ability.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := setupBody(cmd)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		if body["type"] == "local" && !jsonFlag {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes && !output.Confirm("Keep backups on this server only? They do not survive losing the server.") {
				output.Warn("Aborted")
				return nil
			}
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		if body["type"] == "s3" {
			output.Info("Reaching the bucket with its keys — nothing is saved if it does not answer...")
		}
		var answer struct {
			Type        string `json:"type"`
			Destination *struct {
				Name      string `json:"name"`
				Host      string `json:"host"`
				Port      int    `json:"port"`
				User      string `json:"user"`
				Path      string `json:"path"`
				PublicKey string `json:"public_key"`
			} `json:"destination"`
		}
		if err := client.Put("/api/backups/config", body, &answer); err != nil {
			output.Error("Failed to set up backups: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(answer)
			return nil
		}
		switch body["type"] {
		case "local":
			output.Success("Backups set up — on this server only")
		case "s3":
			output.Success("Backups set up — the default bucket is s3://%v", body["bucket"])
		case "sftp":
			if d := answer.Destination; d != nil {
				output.Success("Backups set up — to the SFTP server %s: sftp://%s@%s:%d/%s", d.Name, d.User, d.Host, d.Port, d.Path)
				fmt.Println()
				fmt.Println("The remote has to accept this key — Cipi signs in with it, never with a password:")
				fmt.Println("  " + d.PublicKey)
				fmt.Println()
				for _, line := range sftpInstallHint(d.Name, d.Host, d.Port, d.User, d.PublicKey) {
					fmt.Println(line)
				}
				fmt.Println()
				output.Info("Then: cipi-cli backup destination test %s", d.Name)
			}
		}
		output.Info("Review it: cipi-cli backup status")
		fmt.Println()
		return nil
	},
}

var backupKeyCmd = &cobra.Command{
	Use:   "key",
	Short: "The encryption key of the backups: show it, rotate it",
	Long: `The archives of a profile with encryption are encrypted on the server
(AES-256) before they leave it. The key lives only there: keep a copy off
the server — without it nobody can read those archives.

  cipi-cli backup key show      the key — the server logs it and notifies its owner
  cipi-cli backup key rotate    a new key for the runs to come; the old one stays on the server

Requires Cipi 5.19.0+, cipi/api 1.49+ and the backups-manage ability.`,
}

var backupKeyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the encryption key (logged and notified on the server)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer struct {
			Data struct {
				Key string `json:"key"`
			} `json:"data"`
		}
		if err := client.Get("/api/backups/key", &answer); err != nil {
			output.Error("Failed to read the key: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
			return err
		}
		fmt.Println(answer.Data.Key)
		return nil
	},
}

var backupKeyRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "A new encryption key for the runs to come",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm("Make a new encryption key? The archives already made keep needing the old one, which stays on the server.") {
			output.Warn("Aborted")
			return nil
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := client.Post("/api/backups/key/rotate", nil, &raw); err != nil {
			output.Error("Failed to rotate the key: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
			return err
		}
		output.Success("A new encryption key is in use — save it off the server: cipi-cli backup key show")
		return nil
	},
}

var backupPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Apply the retention now (async job; --dry-run lists what it would delete)",
	Long: `Apply each profile's retention now — or one profile's — on every destination
("cipi backup prune" on the server). --dry-run lists the runs it would delete
and deletes nothing. The CLI waits for the job and prints what the server said.

  cipi-cli backup prune --dry-run
  cipi-cli backup prune --profile nightly

Requires Cipi 5.19.0+, cipi/api 1.49+ and the backups-manage ability.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		dry, _ := cmd.Flags().GetBool("dry-run")
		body := map[string]interface{}{}
		if profile != "" {
			if !sftpNameRe.MatchString(profile) {
				err := fmt.Errorf("--profile: lowercase letters, digits and dashes")
				output.Error("%s", err)
				return err
			}
			body["profile"] = profile
		}
		if dry {
			body["dry_run"] = true
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var send interface{}
		if len(body) > 0 {
			send = body
		}
		job, err := client.DoAsyncAndWaitResult("POST", "/api/backups/prune", send)
		if job != nil {
			printCLIOutput(job.Output)
		}
		if err != nil {
			output.Error("Prune failed: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
			return err
		}
		if dry {
			output.Success("Nothing deleted — that is what the retention would delete")
		} else {
			output.Success("Retention applied")
		}
		fmt.Println()
		return nil
	},
}

// addS3Flags: the flags of an S3 bucket — the secret key never one of them.
func addS3Flags(c *cobra.Command) {
	c.Flags().String("bucket", "", "S3: the bucket")
	c.Flags().String("key", "", "S3: the access key ID")
	c.Flags().String("region", "", "S3: the region (eu-central-1 when not given)")
	c.Flags().String("endpoint", "", "S3: the endpoint, for every provider but AWS (none: back to AWS)")
	c.Flags().String("secret-file", "", "S3: read the secret key from this file (first line)")
	c.Flags().Bool("secret-stdin", false, "S3: read the secret key from standard input (first line)")
}

func addConfigureFlags(c *cobra.Command) {
	addS3Flags(c)
	c.Flags().String("type", "", "Where backups go: local, s3 or sftp")
	c.Flags().Bool("local-only", false, "This server only (--type local)")
	c.Flags().String("host", "", "SFTP: the server")
	c.Flags().Int("port", 22, "SFTP: SSH port (23 for a Hetzner Storage Box)")
	c.Flags().String("user", "", "SFTP: the user on the remote")
	c.Flags().String("path", "", "SFTP: folder on the remote (empty: the login's home)")
	c.Flags().String("name", "sftp", "SFTP: its name in profiles")
	c.Flags().BoolP("yes", "y", false, "Skip confirmation (--type local)")
}

func init() {
	addS3Flags(backupDestinationAddCmd)
	addS3Flags(backupDestinationEditCmd)
	addConfigureFlags(backupConfigureCmd)
	backupKeyRotateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	backupPruneCmd.Flags().String("profile", "", "Only this profile")
	backupPruneCmd.Flags().Bool("dry-run", false, "List what would be deleted, delete nothing")
	backupKeyCmd.AddCommand(backupKeyShowCmd, backupKeyRotateCmd)
	backupCmd.AddCommand(backupConfigureCmd, backupKeyCmd, backupPruneCmd)
}
