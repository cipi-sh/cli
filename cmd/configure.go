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
	Long:  "Set up the connection to your Cipi server API. Credentials are stored in ~/.cipi/config.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		token, _ := cmd.Flags().GetString("token")

		fmt.Println()

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

		cfg := &config.Config{
			Endpoint: endpoint,
			Token:    token,
		}

		if err := config.Save(cfg); err != nil {
			output.Error("Failed to save configuration: %s", err)
			return err
		}

		output.Success("Configuration saved to %s", config.Path())
		fmt.Println()
		return nil
	},
}

var configureShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]string{
				"endpoint": cfg.Endpoint,
				"token":    maskToken(cfg.Token),
				"path":     config.Path(),
			})
			return nil
		}

		output.Header("Configuration")
		output.KeyValue(nil, "Config file", config.Path())
		output.KeyValue(nil, "Endpoint", cfg.Endpoint)
		output.KeyValue(nil, "Token", maskToken(cfg.Token))
		fmt.Println()
		return nil
	},
}

func maskToken(token string) string {
	if len(token) <= 10 {
		return "****"
	}
	return token[:6] + "..." + token[len(token)-4:]
}

func init() {
	configureCmd.Flags().String("endpoint", "", "Cipi API endpoint URL")
	configureCmd.Flags().String("token", "", "API authentication token")
	configureCmd.AddCommand(configureShowCmd)
	rootCmd.AddCommand(configureCmd)
}
