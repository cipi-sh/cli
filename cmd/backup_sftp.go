package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Backup destinations: SFTP servers (cipi/api 1.48, Cipi 5.18.0) — a server
// over SSH, a VPS with a backup user, a Hetzner Storage Box: Cipi makes the
// key on the server, no secret goes through this client — and S3 buckets
// (cipi/api 1.49, Cipi 5.19.0, backup_setup.go).

const backupSftpAPI = "1.48"

var (
	sftpNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	sftpHostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.:-]{0,252}[A-Za-z0-9])?$`)
	sftpUserRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`)
	sftpPathRe = regexp.MustCompile(`^/?[A-Za-z0-9._/-]{0,255}$`)
)

// sftpBody is the body of POST|PUT /api/backups/destinations: only what is given.
func sftpBody(name, host string, port int, user, path string, portSet, pathSet, create bool) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if create {
		if !sftpNameRe.MatchString(name) || name == "local" || name == "s3" {
			return nil, fmt.Errorf("name: lowercase letters, digits and dashes, 2 to 32 — not local or s3")
		}
		body["name"] = name
		body["type"] = "sftp"
		if host == "" || user == "" {
			return nil, fmt.Errorf("--host and --user are needed")
		}
	}
	if host != "" {
		if !sftpHostRe.MatchString(host) {
			return nil, fmt.Errorf("--host is an IP address or a hostname")
		}
		body["host"] = host
	}
	if user != "" {
		if !sftpUserRe.MatchString(user) {
			return nil, fmt.Errorf("--user: letters, digits, . _ -")
		}
		body["user"] = user
	}
	if portSet || create {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("--port is from 1 to 65535")
		}
		body["port"] = port
	}
	if pathSet || create {
		path = strings.TrimRight(strings.TrimSpace(path), "/")
		if !sftpPathRe.MatchString(path) || strings.Contains(path, "..") {
			return nil, fmt.Errorf("--path is a folder on the remote: letters, digits, . _ - /, no ..")
		}
		body["path"] = path
	}
	return body, nil
}

// sftpInstallHint is how to install the key on the remote.
func sftpInstallHint(name, host string, port int, user, key string) []string {
	if strings.HasSuffix(host, ".your-storagebox.de") {
		return []string{
			"A Hetzner Storage Box: save the key as cipi-backup-" + name + ".pub, then (it asks the box's password once):",
			fmt.Sprintf("  ssh-copy-id -p %d -s -f -i cipi-backup-%s.pub %s@%s", port, name, user, host),
		}
	}
	return []string{
		"On the remote, as " + user + ":",
		"  mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '" + strings.ReplaceAll(key, "'", "") + "' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys",
	}
}

type sftpAnswer struct {
	Data struct {
		Name      string `json:"name"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Path      string `json:"path"`
		PublicKey string `json:"public_key"`
	} `json:"data"`
}

func sftpSave(cmd *cobra.Command, args []string, create bool) error {
	// --bucket, --key…: an S3 bucket (cipi/api 1.49, Cipi 5.19.0).
	if s3FlagsGiven(cmd) {
		if cmd.Flags().Changed("host") || cmd.Flags().Changed("user") || cmd.Flags().Changed("port") || cmd.Flags().Changed("path") || cmd.Flags().Changed("new-key") {
			err := fmt.Errorf("an S3 bucket (--bucket …) or an SFTP server (--host …), not both")
			output.Error("%s", err)
			return err
		}
		return s3Save(cmd, args[0], create)
	}
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	user, _ := cmd.Flags().GetString("user")
	path, _ := cmd.Flags().GetString("path")
	newKey, _ := cmd.Flags().GetBool("new-key")
	body, err := sftpBody(args[0], host, port, user, path, cmd.Flags().Changed("port"), cmd.Flags().Changed("path"), create)
	if err != nil {
		output.Error("%s", err)
		return err
	}
	if newKey {
		body["new_key"] = true
	}
	client, err := mustClient()
	if err != nil {
		return err
	}
	var answer sftpAnswer
	if create {
		err = client.Post("/api/backups/destinations", body, &answer)
	} else {
		err = client.Put("/api/backups/destinations/"+args[0], body, &answer)
	}
	if err != nil {
		output.Error("Failed to save the destination: %s", featureErrorHint(err, backupSftpAPI, "backups-manage"))
		return err
	}
	if jsonFlag {
		output.PrintJSON(answer)
		return nil
	}
	d := answer.Data
	output.Success("SFTP destination %s: sftp://%s@%s:%d/%s", d.Name, d.User, d.Host, d.Port, d.Path)
	fmt.Println()
	fmt.Println("The remote has to accept this key — Cipi signs in with it, never with a password:")
	fmt.Println("  " + d.PublicKey)
	fmt.Println()
	for _, line := range sftpInstallHint(d.Name, d.Host, d.Port, d.User, d.PublicKey) {
		fmt.Println(line)
	}
	fmt.Println()
	output.Info("Then: cipi-cli backup destination test %s — and name it in a profile: --dest local,%s", d.Name, d.Name)
	fmt.Println()
	return nil
}

var backupDestinationAddCmd = &cobra.Command{
	Use:   "add <name> (--bucket … | --host … --user …)",
	Short: "Add a destination: an S3 bucket, or an SFTP server",
	Long: `Add a backup destination with a name of its own — profiles send archives
to it next to "local" and "s3" (the default bucket).

An S3 bucket — AWS, Hetzner, Cloudflare R2, DigitalOcean Spaces, Backblaze
B2, Scaleway, MinIO… (--endpoint for every provider but AWS):

  cipi-cli backup destination add offsite --bucket acme-offsite --key AKIA… --endpoint https://<account-id>.r2.cloudflarestorage.com --region auto

The secret key is asked at a prompt (--secret-file or --secret-stdin to read
it) and sent once: it never rides a command line. The server reaches the
bucket with the keys before it saves anything. Cipi 5.19.0+, cipi/api 1.49+.

An SFTP server — a VPS with a backup user, a Hetzner Storage Box (port 23),
a NAS:

  cipi-cli backup destination add box --host u1234.your-storagebox.de --port 23 --user u1234 --path cipi
  cipi-cli backup destination add vps --host 203.0.113.9 --user backup

Cipi makes a key for it on the server and prints its public half: install it
on the remote, then "cipi-cli backup destination test <name>". No password.
Only SFTP is asked of the remote — a chrooted internal-sftp account works.
Cipi 5.18.0+, cipi/api 1.48+.

Requires the backups-manage ability.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          func(cmd *cobra.Command, args []string) error { return sftpSave(cmd, args, true) },
}

var backupDestinationEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Change a destination: what is given changes",
	Long: `Change what is given of a destination; the rest stays.

  cipi-cli backup destination edit offsite --region eu-west-1
  cipi-cli backup destination edit offsite --key AKIA…          asks the secret that goes with it
  cipi-cli backup destination edit offsite --endpoint none      back to AWS
  cipi-cli backup destination edit box --path backups
  cipi-cli backup destination edit box --new-key                a new key: install it on the remote again

An S3 bucket is reached with its keys again before it is saved. The default
bucket ("s3") is changed with "cipi-cli backup configure --type s3".`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          func(cmd *cobra.Command, args []string) error { return sftpSave(cmd, args, false) },
}

var backupDestinationKeyCmd = &cobra.Command{
	Use:   "key <name>",
	Short: "The public key an SFTP destination must accept",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer struct {
			Data struct {
				PublicKey string `json:"public_key"`
			} `json:"data"`
		}
		if err := client.Get("/api/backups/destinations/"+args[0]+"/key", &answer); err != nil {
			output.Error("Failed to read the key: %s", featureErrorHint(err, backupSftpAPI, "backups-view"))
			return err
		}
		fmt.Println(answer.Data.PublicKey)
		return nil
	},
}

var backupDestinationRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a destination no profile uses (its archives stay)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Remove the destination %s? The archives on it stay there, and Cipi stops pruning them.", args[0])) {
			output.Warn("Aborted")
			return nil
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := client.Delete("/api/backups/destinations/"+args[0], nil, &raw); err != nil {
			output.Error("Failed to remove it: %s", featureErrorHint(err, backupSftpAPI, "backups-manage"))
			return err
		}
		output.Success("Destination %s removed — its archives stay there; an SFTP server's key is deleted on the server: remove it from the remote's authorized_keys too", args[0])
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{backupDestinationAddCmd, backupDestinationEditCmd} {
		c.Flags().String("host", "", "Host of the SFTP server")
		c.Flags().Int("port", 22, "SSH port (23 for a Hetzner Storage Box)")
		c.Flags().String("user", "", "User on the remote")
		c.Flags().String("path", "", "Folder on the remote (empty: the login's home)")
	}
	backupDestinationEditCmd.Flags().Bool("new-key", false, "Make a new key")
	backupDestinationRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	backupDestinationCmd.AddCommand(backupDestinationAddCmd, backupDestinationEditCmd, backupDestinationKeyCmd, backupDestinationRemoveCmd)
}
