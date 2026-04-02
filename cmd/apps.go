package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsCmd = &cobra.Command{
	Use:     "apps",
	Aliases: []string{"app"},
	Short:   "Manage applications",
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all applications",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		var result struct {
			Data []map[string]interface{} `json:"data"`
		}

		if err := client.Get("/api/apps", &result); err != nil {
			output.Error("Failed to list apps: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(result.Data) == 0 {
			output.Warn("No applications found")
			return nil
		}

		output.Header("Applications")
		t := output.NewTable("APP", "DOMAIN", "PHP", "REPOSITORY", "BRANCH")
		for _, app := range result.Data {
			t.Row(
				str(app, "app"),
				str(app, "domain"),
				str(app, "php"),
				truncate(str(app, "repository"), 40),
				str(app, "branch"),
			)
		}
		t.Flush()
		output.Dim.Printf("  Total: %d app(s)\n\n", len(result.Data))
		return nil
	},
}

var appsShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show application details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}

		if err := client.Get(fmt.Sprintf("/api/apps/%s", args[0]), &result); err != nil {
			output.Error("Failed to get app: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		app := result.Data
		output.Header(fmt.Sprintf("App: %s", str(app, "app")))
		output.KeyValue(nil, "App", str(app, "app"))
		output.KeyValue(nil, "Domain", str(app, "domain"))
		output.KeyValue(nil, "PHP", str(app, "php"))
		output.KeyValue(nil, "Repository", str(app, "repository"))
		output.KeyValue(nil, "Branch", str(app, "branch"))
		output.KeyValue(nil, "User", str(app, "user"))
		output.KeyValue(nil, "Custom", str(app, "custom"))
		output.KeyValue(nil, "Docroot", str(app, "docroot"))
		output.KeyValue(nil, "Created", str(app, "created_at"))

		if aliases, ok := app["aliases"].([]interface{}); ok && len(aliases) > 0 {
			fmt.Println()
			output.Dim.Println("  Aliases")
			for _, a := range aliases {
				fmt.Printf("    • %v\n", a)
			}
		}

		fmt.Println()
		return nil
	},
}

var appsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		user, _ := cmd.Flags().GetString("user")
		domain, _ := cmd.Flags().GetString("domain")
		php, _ := cmd.Flags().GetString("php")
		repository, _ := cmd.Flags().GetString("repository")
		branch, _ := cmd.Flags().GetString("branch")
		custom, _ := cmd.Flags().GetBool("custom")
		docroot, _ := cmd.Flags().GetString("docroot")

		if user == "" {
			user = output.ReadInput("App username")
		}
		if domain == "" {
			domain = output.ReadInput("Domain")
		}
		if php == "" {
			php = output.ReadInput("PHP version (8.2/8.3/8.4/8.5)")
		}

		body := map[string]interface{}{
			"user":   user,
			"domain": domain,
			"php":    php,
		}

		if custom {
			body["custom"] = true
			if docroot != "" {
				body["docroot"] = docroot
			}
		}
		if repository != "" {
			body["repository"] = repository
		}
		if branch != "" {
			body["branch"] = branch
		}

		output.Info("Creating app '%s'...", user)
		if err := client.DoAsyncAndWait("POST", "/api/apps", body); err != nil {
			output.Error("Failed to create app: %s", err)
			return err
		}

		output.Success("App '%s' created successfully", user)
		fmt.Println()
		return nil
	},
}

var appsEditCmd = &cobra.Command{
	Use:   "edit [name]",
	Short: "Edit an existing application",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		body := make(map[string]interface{})

		if v, _ := cmd.Flags().GetString("php"); v != "" {
			body["php"] = v
		}
		if v, _ := cmd.Flags().GetString("repository"); v != "" {
			body["repository"] = v
		}
		if v, _ := cmd.Flags().GetString("branch"); v != "" {
			body["branch"] = v
		}
		if v, _ := cmd.Flags().GetString("domain"); v != "" {
			body["domain"] = v
		}

		if len(body) == 0 {
			output.Error("No fields to update — use --php, --repository, --branch, or --domain")
			return fmt.Errorf("no fields specified")
		}

		output.Info("Updating app '%s'...", args[0])
		if err := client.DoAsyncAndWait("PUT", fmt.Sprintf("/api/apps/%s", args[0]), body); err != nil {
			output.Error("Failed to update app: %s", err)
			return err
		}

		output.Success("App '%s' updated successfully", args[0])
		fmt.Println()
		return nil
	},
}

var appsDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an application",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Delete app '%s'? This cannot be undone.", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Deleting app '%s'...", args[0])
		if err := client.DoAsyncAndWait("DELETE", fmt.Sprintf("/api/apps/%s", args[0]), nil); err != nil {
			output.Error("Failed to delete app: %s", err)
			return err
		}

		output.Success("App '%s' deleted successfully", args[0])
		fmt.Println()
		return nil
	},
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok && v != nil {
		switch val := v.(type) {
		case string:
			return val
		case bool:
			if val {
				return "yes"
			}
			return "no"
		case float64:
			if val == float64(int(val)) {
				return fmt.Sprintf("%.0f", val)
			}
			return fmt.Sprintf("%g", val)
		default:
			return fmt.Sprintf("%v", val)
		}
	}
	return "—"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func init() {
	appsCreateCmd.Flags().String("user", "", "App username")
	appsCreateCmd.Flags().String("domain", "", "Domain name")
	appsCreateCmd.Flags().String("php", "", "PHP version (8.2/8.3/8.4/8.5)")
	appsCreateCmd.Flags().String("repository", "", "Git repository SSH URL")
	appsCreateCmd.Flags().String("branch", "", "Git branch")
	appsCreateCmd.Flags().Bool("custom", false, "Create as custom app (non-Laravel)")
	appsCreateCmd.Flags().String("docroot", "", "Custom document root")

	appsEditCmd.Flags().String("php", "", "PHP version")
	appsEditCmd.Flags().String("repository", "", "Git repository SSH URL")
	appsEditCmd.Flags().String("branch", "", "Git branch")
	appsEditCmd.Flags().String("domain", "", "Domain name")

	appsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	appsCmd.AddCommand(appsListCmd, appsShowCmd, appsCreateCmd, appsEditCmd, appsDeleteCmd)
	rootCmd.AddCommand(appsCmd)
}
