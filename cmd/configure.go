package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure API endpoint and authentication token",
	Long:  "Set up the connection to your Cipi server API. Credentials are stored per profile in ~/.cipi/config.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		token, _ := cmd.Flags().GetString("token")

		if profile == "" {
			profile = "default"
		}

		fmt.Println()

		if err := config.ValidateProfileName(profile); err != nil {
			output.Error("%s", err)
			return err
		}

		if endpoint == "" {
			endpoint = output.ReadInput("Cipi API endpoint (e.g. https://api.example.com)")
		}
		if token == "" {
			token = output.ReadInput("API token")
		}

		endpoint = strings.TrimRight(endpoint, "/")

		if endpoint == "" || token == "" {
			output.Error("Endpoint and token are required")
			return fmt.Errorf("missing required fields")
		}

		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}

		cfg := &config.Profile{
			Endpoint: endpoint,
			Token:    token,
		}

		if err := config.SaveProfile(profile, cfg); err != nil {
			output.Error("Failed to save configuration: %s", err)
			return err
		}

		output.Success("Profile %q saved to %s", profile, config.Path())
		fmt.Println()
		return nil
	},
}

var configureShowCmd = &cobra.Command{
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

var configureListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var configureDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		return deleteProfile(args[0], yes)
	},
}

var configureDefaultCmd = &cobra.Command{
	Use:   "default <profile>",
	Short: "Set the default profile for commands without a profile prefix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setDefaultProfile(args[0])
	},
}

func listProfiles() error {
	names, defaultProfile, err := config.ListProfiles()
	if err != nil {
		output.Error("%s", err)
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]interface{}{
			"profiles": names,
			"default":  defaultProfile,
			"path":     config.Path(),
		})
		return nil
	}

	output.Header("Profiles")
	output.KeyValue(nil, "Config file", config.Path())
	if defaultProfile != "" {
		output.KeyValue(nil, "Default", defaultProfile)
	}
	fmt.Println()
	if len(names) == 0 {
		output.Warn("No profiles configured")
		return nil
	}
	for _, name := range names {
		marker := ""
		if name == defaultProfile {
			marker = " (default)"
		}
		fmt.Printf("  %s%s\n", name, marker)
	}
	fmt.Println()
	return nil
}

func deleteProfile(name string, yes bool) error {
	if !yes {
		if !output.Confirm(fmt.Sprintf("Delete profile %q?", name)) {
			output.Warn("Cancelled")
			return nil
		}
	}

	if err := config.DeleteProfile(name); err != nil {
		output.Error("%s", err)
		return err
	}

	output.Success("Profile %q deleted", name)
	return nil
}

func setDefaultProfile(name string) error {
	if err := config.SetDefaultProfile(name); err != nil {
		output.Error("%s", err)
		return err
	}
	output.Success("Default profile set to %q", name)
	return nil
}

func showProfile(name string) error {
	profile, err := config.GetProfile(name)
	if err != nil {
		output.Error("%s", err)
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]string{
			"profile":  name,
			"endpoint": profile.Endpoint,
			"token":    maskToken(profile.Token),
			"path":     config.Path(),
		})
		return nil
	}

	output.Header("Configuration")
	output.KeyValue(nil, "Profile", name)
	output.KeyValue(nil, "Config file", config.Path())
	output.KeyValue(nil, "Endpoint", profile.Endpoint)
	output.KeyValue(nil, "Token", maskToken(profile.Token))
	fmt.Println()
	return nil
}

func showAllProfiles() error {
	names, defaultProfile, err := config.ListProfiles()
	if err != nil {
		output.Error("%s", err)
		return err
	}

	if jsonFlag {
		profiles := make(map[string]map[string]string, len(names))
		for _, name := range names {
			profile, err := config.GetProfile(name)
			if err != nil {
				return err
			}
			profiles[name] = map[string]string{
				"endpoint": profile.Endpoint,
				"token":    maskToken(profile.Token),
			}
		}
		output.PrintJSON(map[string]interface{}{
			"profiles": profiles,
			"default":  defaultProfile,
			"path":     config.Path(),
		})
		return nil
	}

	output.Header("Configuration")
	output.KeyValue(nil, "Config file", config.Path())
	if defaultProfile != "" {
		output.KeyValue(nil, "Default", defaultProfile)
	}
	fmt.Println()

	for _, name := range names {
		profile, err := config.GetProfile(name)
		if err != nil {
			return err
		}
		output.Header(name)
		output.KeyValue(nil, "Endpoint", profile.Endpoint)
		output.KeyValue(nil, "Token", maskToken(profile.Token))
		fmt.Println()
	}
	return nil
}

func maskToken(token string) string {
	if len(token) <= 10 {
		return "****"
	}
	return token[:6] + "..." + token[len(token)-4:]
}

func init() {
	configureCmd.Flags().String("profile", "default", "Profile name (e.g. prod, staging)")
	configureCmd.Flags().String("endpoint", "", "Cipi API endpoint URL")
	configureCmd.Flags().String("token", "", "API authentication token")
	configureDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	configureCmd.AddCommand(configureShowCmd, configureListCmd, configureDeleteCmd, configureDefaultCmd)
	rootCmd.AddCommand(configureCmd)
}
