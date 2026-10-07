package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

type smtpResponse struct {
	Data    map[string]interface{} `json:"data"`
	Message string                 `json:"message,omitempty"`
}

var smtpCmd = &cobra.Command{
	Use:   "smtp",
	Short: "Manage server SMTP notifications",
	Long: `Configure the SMTP relay Cipi uses for server notifications (monitor
alerts, healthcheck failures, …).

Requires Cipi 5.0.7+ (API 1.15+) and smtp-view / smtp-manage abilities.
The password is never returned by the API.

  cipi-cli smtp show
  cipi-cli smtp set --host smtp.example.com --user me --password '…' \
    --from cipi@example.com --to ops@example.com
  cipi-cli smtp test
  cipi-cli smtp enable|disable
  cipi-cli smtp delete

` + multiServerTip,
	Example: `  cipi-cli smtp show
  cipi-cli prod smtp set --to oncall@example.com
  cipi-cli smtp test`,
}

var smtpShowCmd = &cobra.Command{
	Use:     "show",
	Aliases: []string{"status"},
	Short:   "Show SMTP configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result smtpResponse
		if err := client.Get("/api/smtp", &result); err != nil {
			output.Error("Failed to read SMTP config: %s", apiErrorHint(err, "1.15.0", "smtp-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		printSmtpStatus(result.Data)
		return nil
	},
}

var smtpSetCmd = &cobra.Command{
	Use:     "set",
	Aliases: []string{"configure"},
	Short:   "Configure SMTP (merges with the current settings)",
	Long: `Configure the SMTP relay. Flags you omit keep their current value, so you
can change a single field (e.g. --to) on a server that is already configured.

--password is required the first time; afterwards omit it to keep the stored
one. A test email is sent after saving unless --no-test is passed.

  cipi-cli smtp set --host smtp.example.com --port 587 --user me \
    --password '…' --from cipi@example.com --to ops@example.com
  cipi-cli smtp set --to oncall@example.com
  cipi-cli smtp set --tls=false --no-test`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var current smtpResponse
		if err := client.Get("/api/smtp", &current); err != nil {
			output.Error("Failed to read SMTP config: %s", apiErrorHint(err, "1.15.0", "smtp-view"))
			return err
		}
		configured, _ := current.Data["configured"].(bool)

		body := map[string]interface{}{}
		for _, key := range []string{"host", "user", "from", "to"} {
			if cmd.Flags().Changed(key) {
				v, _ := cmd.Flags().GetString(key)
				body[key] = v
			} else if configured {
				if v, ok := current.Data[key].(string); ok && v != "" {
					body[key] = v
				}
			}
		}
		if cmd.Flags().Changed("port") {
			port, _ := cmd.Flags().GetInt("port")
			body["port"] = port
		} else if configured {
			if port, err := strconv.Atoi(fmt.Sprintf("%v", current.Data["port"])); err == nil {
				body["port"] = port
			}
		}
		for _, key := range []string{"tls", "enabled"} {
			if cmd.Flags().Changed(key) {
				v, _ := cmd.Flags().GetBool(key)
				body[key] = v
			} else if configured {
				if v, ok := current.Data[key].(bool); ok {
					body[key] = v
				}
			}
		}
		if v, _ := cmd.Flags().GetString("password"); v != "" {
			body["password"] = v
		}
		if noTest, _ := cmd.Flags().GetBool("no-test"); noTest {
			body["test"] = false
		}

		var missing []string
		for _, key := range []string{"host", "user", "from", "to"} {
			if _, ok := body[key]; !ok {
				missing = append(missing, "--"+key)
			}
		}
		if !configured && body["password"] == nil {
			missing = append(missing, "--password")
		}
		if len(missing) > 0 {
			output.Error("SMTP is not configured yet — missing %s", strings.Join(missing, ", "))
			return fmt.Errorf("missing flags: %s", strings.Join(missing, ", "))
		}

		var result smtpResponse
		if err := client.Put("/api/smtp", body, &result); err != nil {
			output.Error("Failed to configure SMTP: %s", apiErrorHint(err, "1.15.0", "smtp-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("SMTP configured")
		printSmtpMessage(result.Message)
		printSmtpStatus(result.Data)
		return nil
	},
}

func newSmtpActionCmd(action, short, done string) *cobra.Command {
	return &cobra.Command{
		Use:   action,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := mustClient()
			if err != nil {
				return err
			}

			var result smtpResponse
			if err := client.Post("/api/smtp/"+action, nil, &result); err != nil {
				output.Error("SMTP %s failed: %s", action, apiErrorHint(err, "1.15.0", "smtp-manage"))
				return err
			}

			if jsonFlag {
				output.PrintJSON(result)
				return nil
			}

			output.Success("%s", done)
			printSmtpMessage(result.Message)
			fmt.Println()
			return nil
		},
	}
}

var smtpDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove the SMTP configuration",
	Long: `Remove the stored SMTP configuration (server notifications stop).

Prompts for confirmation unless -y / --yes is passed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm("Remove the SMTP configuration?") {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result smtpResponse
		if err := client.Delete("/api/smtp", nil, &result); err != nil {
			output.Error("Failed to remove SMTP config: %s", apiErrorHint(err, "1.15.0", "smtp-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("SMTP configuration removed")
		fmt.Println()
		return nil
	},
}

func printSmtpStatus(data map[string]interface{}) {
	output.Header("SMTP")
	if configured, _ := data["configured"].(bool); !configured {
		output.Warn("SMTP is not configured — use 'cipi-cli smtp set'")
		fmt.Println()
		return
	}
	enabled := output.Red.Sprint("no")
	if b, _ := data["enabled"].(bool); b {
		enabled = output.Green.Sprint("yes")
	}
	output.KeyValue(nil, "Enabled", enabled)
	output.KeyValue(nil, "Host", str(data, "host"))
	output.KeyValue(nil, "Port", str(data, "port"))
	output.KeyValue(nil, "User", str(data, "user"))
	output.KeyValue(nil, "TLS", str(data, "tls"))
	output.KeyValue(nil, "From", str(data, "from"))
	output.KeyValue(nil, "To", str(data, "to"))
	fmt.Println()
}

func printSmtpMessage(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	for _, line := range strings.Split(msg, "\n") {
		output.Dim.Printf("  %s\n", line)
	}
}

func init() {
	smtpSetCmd.Flags().String("host", "", "SMTP host")
	smtpSetCmd.Flags().Int("port", 587, "SMTP port")
	smtpSetCmd.Flags().String("user", "", "SMTP username")
	smtpSetCmd.Flags().String("password", "", "SMTP password (required the first time; omit to keep the stored one)")
	smtpSetCmd.Flags().String("from", "", "Sender address")
	smtpSetCmd.Flags().String("to", "", "Notification recipient address")
	smtpSetCmd.Flags().Bool("tls", true, "Use STARTTLS/TLS")
	smtpSetCmd.Flags().Bool("enabled", true, "Enable notifications after saving")
	smtpSetCmd.Flags().Bool("no-test", false, "Do not send a test email after saving")
	smtpDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	smtpCmd.AddCommand(
		smtpShowCmd,
		smtpSetCmd,
		newSmtpActionCmd("enable", "Enable SMTP notifications", "SMTP notifications enabled"),
		newSmtpActionCmd("disable", "Disable SMTP notifications (keeps the configuration)", "SMTP notifications disabled"),
		newSmtpActionCmd("test", "Send a test email", "Test email sent"),
		smtpDeleteCmd,
	)
	rootCmd.AddCommand(smtpCmd)
}
