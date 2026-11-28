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

// Backups set up and managed from here (cipi/api 1.49, Cipi 5.19.0): on the
// server first (the local folder and the nightly profile), then S3 buckets
// and SFTP servers on a par — each with a name, the profiles to send there
// chosen when it is added — the encryption key, the retention. A secret key
// is read from a prompt, a file or stdin — never a flag — and sent once: the
// API writes it to the CLI's standard input on the server.

const backupSetupAPI = "1.49"

var (
	s3BucketRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)
	s3RegionRe   = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	s3EndpointRe = regexp.MustCompile(`^https?://[A-Za-z0-9.:/_-]{1,255}$`)
	// A key or a secret: printable, no space, no quote, no backslash — the
	// server writes them to /root/.aws/credentials.
	s3CredRe = regexp.MustCompile(`^[\x21\x23-\x26\x28-\x5b\x5d-\x7e]{1,256}$`)
)

// s3Body is the body of an S3 destination: only what
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
				if !sftpNameRe.MatchString(name) || name == "local" {
					err = fmt.Errorf("name: lowercase letters, digits and dashes, 2 to 32 — not local (this server)")
				}
				body["type"], body["name"] = "s3", name
			}
			if err == nil {
				err = addUse(cmd, body)
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

// useProfiles reads --use: the profiles sent to a new destination too.
func useProfiles(v string) ([]string, error) {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !sftpNameRe.MatchString(p) {
			return nil, fmt.Errorf("--use: profile names, comma-separated")
		}
		out = append(out, p)
	}
	return out, nil
}

// addUse puts --use into the body of a new or changed destination.
func addUse(cmd *cobra.Command, body map[string]interface{}) error {
	v, _ := cmd.Flags().GetString("use")
	profiles, err := useProfiles(v)
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		body["profiles"] = profiles
	}
	return nil
}

var backupConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set up backups on the server: the local folder and a nightly profile",
	Long: `Set up backups on the server — "cipi backup configure" there: a nightly
profile takes every app and every database and keeps them on the server.
Then send them elsewhere too — S3 buckets and SFTP servers, on a par:

  cipi-cli backup configure
  cipi-cli backup destination add s3 --bucket acme-backups --key AKIA… --endpoint https://fsn1.your-objectstorage.com --use default
  cipi-cli backup destination add sftp --host u1234.your-storagebox.de --port 23 --user u1234 --use default

The local and temp folders are set on the server.

Requires Cipi 5.19.0+, cipi/api 1.49+ and the backups-manage ability.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer json.RawMessage
		if err := client.Put("/api/backups/config", map[string]interface{}{}, &answer); err != nil {
			output.Error("Failed to set up backups: %s", featureErrorHint(err, backupSetupAPI, "backups-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(answer)
			return nil
		}
		output.Success("Backups set up on the server: a nightly profile keeps every app and database there")
		output.Info("A copy only on the server is lost with it — send it elsewhere too:")
		fmt.Println("  cipi-cli backup destination add s3 --bucket <b> --key <id> [--endpoint <url>] --use default")
		fmt.Println("  cipi-cli backup destination add sftp --host <host> --user <user> [--port 23] --use default")
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

func init() {
	addS3Flags(backupDestinationAddCmd)
	addS3Flags(backupDestinationEditCmd)
	for _, c := range []*cobra.Command{backupDestinationAddCmd, backupDestinationEditCmd} {
		c.Flags().String("use", "", "Send these profiles there too: default[,…] (Cipi 5.19.1+)")
	}
	backupKeyRotateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	backupPruneCmd.Flags().String("profile", "", "Only this profile")
	backupPruneCmd.Flags().Bool("dry-run", false, "List what would be deleted, delete nothing")
	backupKeyCmd.AddCommand(backupKeyShowCmd, backupKeyRotateCmd)
	backupCmd.AddCommand(backupConfigureCmd, backupKeyCmd, backupPruneCmd)
}
