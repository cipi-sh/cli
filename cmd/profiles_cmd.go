package cmd

import (
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:     "profiles",
	Aliases: []string{"profile", "servers", "server"},
	Short:   "Manage server profiles (multi-server)",
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
	Long: `Add or update a server profile (same as 'cipi-cli configure --profile <name>').

You will be prompted for the API endpoint and token unless you pass flags.`,
	Example: `  cipi-cli profiles add prod
  cipi-cli profiles add staging --endpoint https://api.example.com --token "1|..."
  cipi-cli servers add client-a`,
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
	Example: `  cipi-cli profiles show
  cipi-cli profiles show prod`,
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
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a server profile",
	Example: `  cipi-cli profiles delete staging
  cipi-cli profiles delete staging -y`,
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
