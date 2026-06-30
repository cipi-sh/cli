package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appLogTypes = []string{"all", "nginx", "php", "worker", "deploy", "laravel"}

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
		t := output.NewTable("APP", "DOMAIN", "PHP", "REPOSITORY", "BRANCH", "SUSPENDED")
		for _, app := range result.Data {
			t.Row(
				str(app, "app"),
				str(app, "domain"),
				str(app, "php"),
				truncate(str(app, "repository"), 40),
				str(app, "branch"),
				str(app, "suspended"),
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
		output.KeyValue(nil, "Suspended", str(app, "suspended"))
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
			php = output.ReadInput("PHP version (8.3/8.4/8.5)")
		}
		if repository == "" && !custom {
			repository = output.ReadInput("Git repository SSH URL")
		}
		if branch == "" && !custom {
			branch = output.ReadInput("Git branch")
		}

		if !custom && repository == "" {
			output.Error("Repository is required")
			return fmt.Errorf("repository is required")
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

var appsSuspendCmd = &cobra.Command{
	Use:   "suspend [name]",
	Short: "Suspend an application (serve an HTTP 503 page)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Suspending app '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/suspend", args[0]), nil); err != nil {
			output.Error("Failed to suspend app: %s", err)
			return err
		}

		output.Success("App '%s' suspended successfully", args[0])
		fmt.Println()
		return nil
	},
}

var appsLogsCmd = &cobra.Command{
	Use:   "logs [name]",
	Short: "Read application logs",
	Long:  "Read paginated log snapshots (nginx, PHP-FPM, Laravel, worker, deploy). Page 1 returns the most recent lines.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logType, _ := cmd.Flags().GetString("type")
		page, _ := cmd.Flags().GetInt("page")
		perPage, _ := cmd.Flags().GetInt("per-page")

		logType = strings.ToLower(strings.TrimSpace(logType))
		if !isAppLogType(logType) {
			output.Error("Invalid log type %q — use one of: %s", logType, strings.Join(appLogTypes, ", "))
			return fmt.Errorf("invalid log type")
		}
		if page < 1 {
			output.Error("Page must be at least 1")
			return fmt.Errorf("invalid page")
		}
		if perPage < 1 || perPage > 1000 {
			output.Error("per-page must be between 1 and 1000")
			return fmt.Errorf("invalid per-page")
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		query := url.Values{}
		query.Set("type", logType)
		query.Set("page", fmt.Sprintf("%d", page))
		query.Set("per_page", fmt.Sprintf("%d", perPage))

		path := fmt.Sprintf("/api/apps/%s/logs?%s", url.PathEscape(args[0]), query.Encode())

		var result struct {
			Data map[string]interface{} `json:"data"`
		}

		if err := client.Get(path, &result); err != nil {
			msg := api.RouteNotFoundHint(err, "1.11.9", "GET /api/apps/{name}/logs")
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Status >= 500 {
				msg = fmt.Sprintf("%s — check storage/logs/laravel.log on the Cipi API host", apiErr.Error())
			}
			output.Error("Failed to read logs: %s", msg)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		printAppLogs(result.Data)
		return nil
	},
}

var appsUnsuspendCmd = &cobra.Command{
	Use:   "unsuspend [name]",
	Short: "Unsuspend an application (bring it back online)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Unsuspending app '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/unsuspend", args[0]), nil); err != nil {
			output.Error("Failed to unsuspend app: %s", err)
			return err
		}

		output.Success("App '%s' unsuspended successfully", args[0])
		fmt.Println()
		return nil
	},
}

func isAppLogType(t string) bool {
	for _, v := range appLogTypes {
		if t == v {
			return true
		}
	}
	return false
}

func printAppLogs(data map[string]interface{}) {
	if len(data) == 0 {
		output.Warn("No log data returned")
		return
	}

	app := str(data, "app")
	logType := str(data, "type")
	page := str(data, "page")
	perPage := str(data, "per_page")

	output.Header(fmt.Sprintf("App logs: %s", app))
	output.KeyValue(nil, "Type", logType)
	output.KeyValue(nil, "Page", page)
	output.KeyValue(nil, "Per page", perPage)

	if types, ok := data["available_types"].([]interface{}); ok && len(types) > 0 {
		parts := make([]string, 0, len(types))
		for _, t := range types {
			parts = append(parts, fmt.Sprintf("%v", t))
		}
		output.KeyValue(nil, "Available", strings.Join(parts, ", "))
	}

	if warnings, ok := data["warnings"].([]interface{}); ok {
		for _, w := range warnings {
			output.Warn("%v", w)
		}
	}

	files, _ := data["files"].([]interface{})
	if len(files) == 0 {
		output.Warn("No log files matched this filter")
		fmt.Println()
		return
	}

	for _, item := range files {
		file, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		path := str(file, "path")
		filePage := str(file, "page")
		totalPages := str(file, "total_pages")
		totalLines := str(file, "total_lines")

		fmt.Println()
		output.Dim.Printf("  %s\n", path)
		output.Dim.Printf("  page %s/%s (%s lines total)\n", filePage, totalPages, totalLines)
		output.Dim.Println("  " + strings.Repeat("─", 60))

		lines, _ := file["lines"].([]interface{})
		if len(lines) == 0 {
			output.Dim.Println("  (empty)")
			continue
		}
		for _, line := range lines {
			fmt.Printf("  %v\n", line)
		}
	}

	fmt.Println()
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
	appsCreateCmd.Flags().String("php", "", "PHP version (8.3/8.4/8.5)")
	appsCreateCmd.Flags().String("repository", "", "Git repository SSH URL")
	appsCreateCmd.Flags().String("branch", "", "Git branch")
	appsCreateCmd.Flags().Bool("custom", false, "Create as custom app (non-Laravel)")
	appsCreateCmd.Flags().String("docroot", "", "Custom document root")

	appsEditCmd.Flags().String("php", "", "PHP version")
	appsEditCmd.Flags().String("repository", "", "Git repository SSH URL")
	appsEditCmd.Flags().String("branch", "", "Git branch")
	appsEditCmd.Flags().String("domain", "", "New primary domain (requires Cipi 4.6.2+ / API 1.9.0+)")

	appsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	appsLogsCmd.Flags().StringP("type", "t", "all", "Log type (all, nginx, php, worker, deploy, laravel)")
	appsLogsCmd.Flags().IntP("page", "p", 1, "Page number (1 = most recent lines)")
	appsLogsCmd.Flags().Int("per-page", 50, "Lines per log file per page (max 1000)")

	appsCmd.AddCommand(appsListCmd, appsShowCmd, appsCreateCmd, appsEditCmd, appsDeleteCmd, appsLogsCmd, appsSuspendCmd, appsUnsuspendCmd)
	rootCmd.AddCommand(appsCmd)
}
