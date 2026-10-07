package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var sshKeysCmd = &cobra.Command{
	Use:     "ssh-keys",
	Aliases: []string{"ssh"},
	Short:   "Manage SSH keys of the cipi user",
	Long: `List, add, or remove authorized SSH keys of the server's cipi user.

Requires Cipi 5.0.6+ (API 1.15+) and ssh-view / ssh-manage abilities.

  cipi-cli ssh-keys list
  cipi-cli ssh-keys add --file ~/.ssh/id_ed25519.pub
  cipi-cli ssh-keys remove 2

` + multiServerTip,
	Example: `  cipi-cli ssh-keys list
  cipi-cli prod ssh-keys add --file ~/.ssh/id_ed25519.pub
  cipi-cli ssh-keys add "ssh-ed25519 AAAAC3Nza... me@laptop"
  cipi-cli ssh-keys remove 2 -y`,
}

var sshKeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List authorized SSH keys",
	Long: `List authorized keys of the cipi user with their numeric id (used by
'ssh-keys remove'), type, comment, and fingerprint.

  cipi-cli ssh-keys list
  cipi-cli prod ssh-keys list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/ssh/keys", &result); err != nil {
			output.Error("Failed to list SSH keys: %s", apiErrorHint(err, "1.15.0", "ssh-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(result.Data) == 0 {
			output.Warn("No SSH keys found")
			return nil
		}

		output.Header("SSH keys (cipi user)")
		t := output.NewTable("ID", "TYPE", "COMMENT", "FINGERPRINT", "CURRENT")
		for _, k := range result.Data {
			current := ""
			if b, _ := k["current_session"].(bool); b {
				current = output.Yellow.Sprint("this session")
			}
			t.Row(str(k, "id"), str(k, "type"), str(k, "comment"), str(k, "fingerprint"), current)
		}
		t.Flush()
		output.Dim.Printf("  Total: %d key(s)\n\n", len(result.Data))
		return nil
	},
}

var sshKeysAddCmd = &cobra.Command{
	Use:   "add [public key...]",
	Short: "Add an authorized SSH public key",
	Long: `Add an SSH public key (ssh-rsa, ssh-ed25519, or ecdsa-sha2-*) to the
cipi user. Pass the key inline or read it from a .pub file with --file.

  cipi-cli ssh-keys add --file ~/.ssh/id_ed25519.pub
  cipi-cli ssh-keys add "ssh-ed25519 AAAAC3Nza... me@laptop"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")

		var key string
		switch {
		case file != "" && len(args) > 0:
			output.Error("Pass the key inline or with --file, not both")
			return fmt.Errorf("conflicting key sources")
		case file != "":
			if rest, ok := strings.CutPrefix(file, "~/"); ok {
				if home, err := os.UserHomeDir(); err == nil {
					file = filepath.Join(home, rest)
				}
			}
			data, err := os.ReadFile(file)
			if err != nil {
				output.Error("Cannot read key file: %s", err)
				return err
			}
			key = string(data)
		default:
			key = strings.Join(args, " ")
		}
		key = strings.TrimSpace(key)
		if key == "" {
			output.Error("No key given — pass it inline or use --file <path.pub>")
			return fmt.Errorf("missing key")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post("/api/ssh/keys", map[string]string{"key": key}, &result); err != nil {
			output.Error("Failed to add SSH key: %s", apiErrorHint(err, "1.15.0", "ssh-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("SSH key added")
		fmt.Println()
		return nil
	},
}

var sshKeysRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove an authorized SSH key by id",
	Long: `Remove an authorized SSH key by the numeric id shown in 'ssh-keys list'.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli ssh-keys remove 2
  cipi-cli ssh-keys remove 2 -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil || id < 1 {
			output.Error("Key id must be a positive integer (see 'cipi-cli ssh-keys list')")
			return fmt.Errorf("invalid key id %q", args[0])
		}

		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Remove SSH key #%d from the cipi user?", id)) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Delete(fmt.Sprintf("/api/ssh/keys/%d", id), nil, nil); err != nil {
			output.Error("Failed to remove SSH key: %s", apiErrorHint(err, "1.15.0", "ssh-manage"))
			return err
		}

		output.Success("SSH key #%d removed", id)
		fmt.Println()
		return nil
	},
}

func init() {
	sshKeysAddCmd.Flags().StringP("file", "f", "", "Read the public key from a file (e.g. ~/.ssh/id_ed25519.pub)")
	sshKeysRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	sshKeysCmd.AddCommand(sshKeysListCmd, sshKeysAddCmd, sshKeysRemoveCmd)
	rootCmd.AddCommand(sshKeysCmd)
}
