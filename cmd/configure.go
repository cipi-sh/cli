package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

const profilesHelp = `A profile is one Cipi server (API endpoint + token).

Add a server:
  cipi-cli configure --profile prod
  cipi-cli profiles add staging

List servers:
  cipi-cli profiles

Set the default server (used when you omit the profile prefix):
  cipi-cli profiles use prod

Run a command against a server:
  cipi-cli prod apps list          # explicit profile
  cipi-cli apps list               # uses the default profile

Config file: ~/.cipi/config.json`

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Add or update a server profile (endpoint + token)",
	Long: `Add or update a server profile.

Each profile maps to one Cipi server. Use a distinct name per server
(e.g. prod, staging, client-a).

` + profilesHelp,
	Example: `  # Interactive setup for a server named "prod"
  cipi-cli configure --profile prod

  # Same via api token add
  cipi-cli api token add prod

  # Non-interactive
  cipi-cli configure --profile staging --endpoint https://api.example.com --token "1|..."

  # Interactive (asks for profile name — does not assume "default")
  cipi-cli configure`,
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		token, _ := cmd.Flags().GetString("token")
		return runConfigure(profile, endpoint, token)
	},
}

var configureShowCmd = &cobra.Command{
	Use:   "show [profile]",
	Short: "Show configuration for one or all profiles",
	Long:  "Alias of 'cipi-cli profiles show'. Prefer the profiles command.",
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
	Short: "List configured server profiles",
	Long:  "Alias of 'cipi-cli profiles'. Prefer the profiles command.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProfiles()
	},
}

var configureDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a server profile",
	Long:  "Alias of 'cipi-cli profiles delete'. Prefer the profiles command.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		return deleteProfile(args[0], yes)
	},
}

var configureDefaultCmd = &cobra.Command{
	Use:   "default <profile>",
	Short: "Set the default server profile",
	Long:  "Alias of 'cipi-cli profiles use'. Prefer the profiles command.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setDefaultProfile(args[0])
	},
}

func runConfigure(profile, endpoint, token string) error {
	fmt.Println()

	if profile == "" {
		names, _, err := config.ListProfiles()
		if err == nil && len(names) > 0 {
			output.Info("Existing profiles: %s", strings.Join(names, ", "))
		}
		profile = output.ReadInput("Profile name (e.g. prod, staging)")
		profile = strings.TrimSpace(profile)
		if profile == "" {
			output.Error("Profile name is required — pick an alias for this server (e.g. prod)")
			output.Info("Example: cipi-cli api token add prod")
			return fmt.Errorf("profile name is required")
		}
	}

	if err := config.ValidateProfileName(profile); err != nil {
		output.Error("%s", err)
		return err
	}

	if config.ProfileExists(profile) {
		output.Warn("Profile %q already exists — it will be updated", profile)
	}

	if endpoint == "" {
		if existing, err := config.GetProfile(profile); err == nil && existing.Endpoint != "" {
			endpoint = output.ReadInput(fmt.Sprintf("Cipi API endpoint [%s]", existing.Endpoint))
			if endpoint == "" {
				endpoint = existing.Endpoint
			}
		} else {
			endpoint = output.ReadInput("Cipi API endpoint (e.g. https://api.example.com)")
		}
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

	_, defaultProfile, _ := config.ListProfiles()
	output.Success("Server profile %q saved → %s", profile, config.Path())
	fmt.Println()
	output.Info("Target this server:  cipi-cli %s apps list", profile)
	if defaultProfile == profile {
		output.Info("It is the default profile — you can omit the name:  cipi-cli apps list")
	} else {
		output.Info("Make it default:     cipi-cli profiles use %s", profile)
	}
	output.Info("List all servers:    cipi-cli profiles")
	fmt.Println()
	return nil
}

func listProfiles() error {
	names, defaultProfile, err := config.ListProfiles()
	if err != nil {
		if strings.Contains(err.Error(), "not configured") {
			if jsonFlag {
				output.PrintJSON(map[string]interface{}{
					"profiles": []string{},
					"default":  "",
					"path":     config.Path(),
				})
				return nil
			}
			output.Header("Server profiles")
			output.KeyValue(nil, "Config file", config.Path())
			fmt.Println()
			output.Warn("No servers configured yet")
			fmt.Println()
			output.Info("Add your first server:")
			output.Dim.Println("  cipi-cli configure --profile prod")
			output.Dim.Println("  cipi-cli profiles add staging")
			fmt.Println()
			return nil
		}
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

	output.Header("Server profiles")
	output.KeyValue(nil, "Config file", config.Path())
	if defaultProfile != "" {
		output.KeyValue(nil, "Default", defaultProfile)
	}
	fmt.Println()

	if len(names) == 0 {
		output.Warn("No servers configured yet")
		fmt.Println()
		output.Info("Add a server:  cipi-cli configure --profile prod")
		fmt.Println()
		return nil
	}

	table := output.NewTable("PROFILE", "ENDPOINT", "DEFAULT")
	for _, name := range names {
		profile, err := config.GetProfile(name)
		if err != nil {
			return err
		}
		isDefault := ""
		if name == defaultProfile {
			isDefault = "yes"
		}
		table.Row(name, profile.Endpoint, isDefault)
	}
	table.Flush()

	output.Dim.Println("  Usage:")
	output.Dim.Println("    cipi-cli <profile> <command>     target a server for one command")
	output.Dim.Println("    cipi-cli profiles use <profile>  set the default server")
	output.Dim.Println("    cipi-cli profiles add <name>     add another server")
	fmt.Println()
	return nil
}

func deleteProfile(name string, yes bool) error {
	if !yes {
		if !output.Confirm(fmt.Sprintf("Delete server profile %q?", name)) {
			output.Warn("Cancelled")
			return nil
		}
	}

	if err := config.DeleteProfile(name); err != nil {
		output.Error("%s", err)
		return err
	}

	output.Success("Server profile %q deleted", name)
	return nil
}

func setDefaultProfile(name string) error {
	if err := config.SetDefaultProfile(name); err != nil {
		output.Error("%s", err)
		return err
	}
	output.Success("Default server set to %q", name)
	fmt.Println()
	output.Info("Commands without a profile prefix now use %q:", name)
	output.Dim.Println("  cipi-cli apps list")
	fmt.Println()
	output.Info("Still target another server explicitly:")
	output.Dim.Printf("  cipi-cli <other-profile> apps list\n")
	fmt.Println()
	return nil
}

func showProfile(name string) error {
	profile, err := config.GetProfile(name)
	if err != nil {
		output.Error("%s", err)
		return err
	}

	_, defaultProfile, _ := config.ListProfiles()

	if jsonFlag {
		output.PrintJSON(map[string]interface{}{
			"profile":  name,
			"endpoint": profile.Endpoint,
			"token":    maskToken(profile.Token),
			"default":  name == defaultProfile,
			"path":     config.Path(),
		})
		return nil
	}

	output.Header("Server profile")
	output.KeyValue(nil, "Profile", name)
	if name == defaultProfile {
		output.KeyValue(nil, "Default", "yes")
	}
	output.KeyValue(nil, "Config file", config.Path())
	output.KeyValue(nil, "Endpoint", profile.Endpoint)
	output.KeyValue(nil, "Token", maskToken(profile.Token))
	fmt.Println()
	output.Info("Target this server:  cipi-cli %s apps list", name)
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

	output.Header("Server profiles")
	output.KeyValue(nil, "Config file", config.Path())
	if defaultProfile != "" {
		output.KeyValue(nil, "Default", defaultProfile)
	}
	fmt.Println()

	if len(names) == 0 {
		output.Warn("No servers configured yet")
		fmt.Println()
		return nil
	}

	for _, name := range names {
		profile, err := config.GetProfile(name)
		if err != nil {
			return err
		}
		title := name
		if name == defaultProfile {
			title = name + " (default)"
		}
		output.Header(title)
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
	configureCmd.Flags().String("profile", "", "Server profile name (e.g. prod, staging). Prompted if omitted")
	configureCmd.Flags().String("endpoint", "", "Cipi API endpoint URL")
	configureCmd.Flags().String("token", "", "API authentication token")
	configureDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	configureCmd.AddCommand(configureShowCmd, configureListCmd, configureDeleteCmd, configureDefaultCmd)
	rootCmd.AddCommand(configureCmd)
}
