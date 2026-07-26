package cmd

import (
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Manage local API credentials (server profiles)",
	Long: `Manage local API credentials stored as server profiles.

Create a token on the Cipi server with:
  cipi api token create

Then store it in this CLI under a named profile:
  cipi-cli api token add prod

Each profile is one server (endpoint + token). Same as:
  cipi-cli configure --profile prod
  cipi-cli profiles add prod`,
	Example: `  cipi-cli api token add prod
  cipi-cli api token add staging --endpoint https://api.example.com --token "1|..."`,
}

var apiTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage API tokens stored in local profiles",
	Long: `Store and update API tokens for named server profiles.

Create the token on the server first:
  cipi api token create

Then add it locally:
  cipi-cli api token add prod`,
}

var apiTokenAddCmd = &cobra.Command{
	Use:   "add [profile]",
	Short: "Add or update an API token for a server profile",
	Long: `Store an API endpoint and token under a named server profile.

The profile name is required (positional argument or --profile).
If omitted interactively, you will be prompted — it will not silently
overwrite the "default" profile.

Create the token on the server first:
  cipi api token create

Same as 'cipi-cli configure --profile <name>' and 'cipi-cli profiles add <name>'.`,
	Example: `  # Add token for server profile "prod"
  cipi-cli api token add prod

  # Same with flags
  cipi-cli api token add --profile staging
  cipi-cli api token add prod --endpoint https://api.example.com --token "1|..."

  # Interactive (asks for profile name, endpoint, token)
  cipi-cli api token add`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		if len(args) == 1 {
			profile = args[0]
		}
		endpoint, _ := cmd.Flags().GetString("endpoint")
		token, _ := cmd.Flags().GetString("token")
		return runConfigure(profile, endpoint, token)
	},
}

func init() {
	apiTokenAddCmd.Flags().String("profile", "", "Server profile name (e.g. prod, staging)")
	apiTokenAddCmd.Flags().String("endpoint", "", "Cipi API endpoint URL")
	apiTokenAddCmd.Flags().String("token", "", "API authentication token")
	apiTokenCmd.AddCommand(apiTokenAddCmd)
	apiCmd.AddCommand(apiTokenCmd)
	rootCmd.AddCommand(apiCmd)
}
