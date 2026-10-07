package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage the Git push-to-deploy webhook of an app",
	Long: `Recreate the GitHub/GitLab push-to-deploy webhook of an application.

Requires Cipi 5.0.6+ (API 1.15+) and the apps-edit token ability.

  cipi-cli apps webhook recreate myapp
  cipi-cli apps webhook recreate myapp --rotate-secret`,
}

var appsWebhookRecreateCmd = &cobra.Command{
	Use:   "recreate <name>",
	Short: "Recreate the Git webhook (optionally rotating its secret)",
	Long: `Recreate the GitHub/GitLab webhook for an application (async job).

Use --rotate-secret to also generate a new CIPI_WEBHOOK_TOKEN. The webhook URL
and token reported by the server are printed when the job completes.

  cipi-cli apps webhook recreate myapp
  cipi-cli apps webhook recreate myapp --rotate-secret`,
	Example: `  cipi-cli apps webhook recreate myapp
  cipi-cli prod apps webhook recreate myapp --rotate-secret`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rotate, _ := cmd.Flags().GetBool("rotate-secret")

		client, err := mustClient()
		if err != nil {
			return err
		}

		var body map[string]interface{}
		if rotate {
			body = map[string]interface{}{"rotate_secret": true}
		}

		output.Info("Recreating webhook for '%s'...", args[0])
		job, err := client.DoAsyncAndWaitResult("POST", appAPIPath(args[0], "/webhook/recreate"), body)
		if err != nil {
			output.Error("Failed to recreate webhook: %s", apiErrorHint(err, "1.15.0", "apps-edit"))
			return err
		}

		result, _ := jobResultMap(job)
		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": result})
			return nil
		}

		output.Success("Webhook recreated for '%s'", args[0])
		if result != nil {
			for _, f := range []struct{ key, label string }{
				{"webhook_url", "Webhook URL"},
				{"webhook_token", "Webhook token"},
				{"webhook_id", "Webhook ID"},
				{"rotated", "Secret rotated"},
			} {
				if v, ok := result[f.key]; ok && v != nil {
					output.KeyValue(nil, f.label, formatFieldValue(v))
				}
			}
		}
		fmt.Println()
		return nil
	},
}

func init() {
	appsWebhookRecreateCmd.Flags().Bool("rotate-secret", false, "Also rotate CIPI_WEBHOOK_TOKEN")

	appsWebhookCmd.AddCommand(appsWebhookRecreateCmd)
	appsCmd.AddCommand(appsWebhookCmd)
}
