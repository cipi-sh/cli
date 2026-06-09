package cmd

import (
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:     "profiles",
	Aliases: []string{"profile"},
	Short:   "Manage server profiles",
	Long:    "List, inspect, and manage configured server profiles stored in ~/.cipi/config.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var profilesShowCmd = &cobra.Command{
	Use:   "show [profile]",
	Short: "Show configuration for one or all profiles",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return showProfile(args[0])
		}
		return showAllProfiles()
	},
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		return deleteProfile(args[0], yes)
	},
}

var profilesDefaultCmd = &cobra.Command{
	Use:   "default <profile>",
	Short: "Set the default profile for commands without a profile prefix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setDefaultProfile(args[0])
	},
}

func init() {
	profilesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	profilesCmd.AddCommand(profilesListCmd, profilesShowCmd, profilesDeleteCmd, profilesDefaultCmd)
	rootCmd.AddCommand(profilesCmd)
}
