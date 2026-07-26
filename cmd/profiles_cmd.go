package cmd

import (
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:     "profiles",
	Aliases: []string{"profile", "servers", "server"},
	Short:   "Manage server profiles (one profile = one server)",
	Long: `Manage connections to one or more Cipi servers.

` + profilesHelp,
	Example: `  # List configured servers
  cipi-cli profiles
  cipi-cli servers

  # Add another server
  cipi-cli profiles add staging

  # Inspect a server
  cipi-cli profiles show prod

  # Set the default server
  cipi-cli profiles use prod

  # Delete a server profile
  cipi-cli profiles delete staging`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var profilesAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add or update a server profile",
	Long: `Add or update a server profile (same as 'cipi-cli configure --profile <name>'
and 'cipi-cli api token add <name>').

You will be prompted for the API endpoint and token unless you pass flags.
If the name is omitted, you will be asked — it will not silently write to "default".`,
	Example: `  cipi-cli profiles add prod
  cipi-cli profiles add staging --endpoint https://api.example.com --token "1|..."
  cipi-cli servers add client-a
  cipi-cli configure --profile prod
  cipi-cli api token add prod`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profile := ""
		if len(args) == 1 {
			profile = args[0]
		}
		endpoint, _ := cmd.Flags().GetString("endpoint")
		token, _ := cmd.Flags().GetString("token")
		return runConfigure(profile, endpoint, token)
	},
}

var profilesShowCmd = &cobra.Command{
	Use:   "show [profile]",
	Short: "Show details for one or all server profiles",
	Long: `Show endpoint (and masked token info) for one profile, or all profiles.

  cipi-cli profiles show
  cipi-cli profiles show prod`,
	Example: `  cipi-cli profiles show
  cipi-cli profiles show prod
  cipi-cli servers show staging`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return showProfile(args[0])
		}
		return showAllProfiles()
	},
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured server profiles",
	Long: `List all configured servers (same as bare 'cipi-cli profiles').

Shows profile name, endpoint, and which one is the default.`,
	Example: `  cipi-cli profiles list
  cipi-cli profiles
  cipi-cli servers`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a server profile",
	Long: `Remove a server profile from the local config (~/.cipi/config.json).

Does not change anything on the remote Cipi server — only deletes local
credentials for that profile name.

If you delete the default profile, another remaining profile becomes default
when only one is left.`,
	Example: `  cipi-cli profiles delete staging
  cipi-cli profiles delete staging -y
  cipi-cli servers delete old-prod`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		return deleteProfile(args[0], yes)
	},
}

var profilesDefaultCmd = &cobra.Command{
	Use:     "default <profile>",
	Aliases: []string{"use"},
	Short:   "Set the default server profile",
	Long: `Set which server is used when you omit the profile prefix.

After this:
  cipi-cli apps list              → uses the default profile
  cipi-cli staging apps list      → still targets "staging" explicitly`,
	Example: `  cipi-cli profiles use prod
  cipi-cli profiles default prod
  cipi-cli servers use staging`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setDefaultProfile(args[0])
	},
}

func init() {
	profilesAddCmd.Flags().String("endpoint", "", "Cipi API endpoint URL")
	profilesAddCmd.Flags().String("token", "", "API authentication token")
	profilesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	profilesCmd.AddCommand(profilesAddCmd, profilesListCmd, profilesShowCmd, profilesDeleteCmd, profilesDefaultCmd)
	rootCmd.AddCommand(profilesCmd)
}
