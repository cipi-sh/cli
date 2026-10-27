package cmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appLogTypes = []string{"all", "nginx", "php", "worker", "deploy", "laravel"}

// envLabelPattern is what an app environment label may look like (production,
// staging, qa-1, …): 1 to 32 lowercase letters, digits, - or _, starting with
// a letter or digit. The API enforces the same rule.
var envLabelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// envRequires is what a server needs to know environment labels.
const envRequires = "Cipi 5.8.0+ / API 1.36+"

// validateEnvLabel rejects an environment label the API would refuse, with
// the rule spelled out.
func validateEnvLabel(label string) error {
	if envLabelPattern.MatchString(label) {
		return nil
	}
	return fmt.Errorf("invalid environment label %q — 1 to 32 lowercase letters, digits, - or _, starting with a letter or digit (e.g. production, staging)", label)
}

// envLabelOrDefault is the label to show: production when the server (before
// environment labels) sends none.
func envLabelOrDefault(env string) string {
	if env == "" {
		return "production"
	}
	return env
}

// appEnv is the environment label of an app object from the API.
func appEnv(app map[string]interface{}) string {
	env, _ := app["env"].(string)
	return envLabelOrDefault(env)
}

// warnIfEnvDropped says when the server kept no environment label: servers
// before environment labels accept the field and drop it. The job result is
// trusted when it echoes the label; otherwise the app is read back.
func warnIfEnvDropped(client *api.Client, app, want string, result map[string]interface{}) {
	if got, ok := result["env"].(string); ok && got == want {
		return
	}
	var fetched struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := client.Get(appAPIPath(app, ""), &fetched); err != nil {
		output.Dim.Printf("  Could not read back the environment label: %s\n", err)
		return
	}
	if got, _ := fetched.Data["env"].(string); got != want {
		output.Warn("The server did not keep the environment label %q (needs %s) — the app stays %s", want, envRequires, appEnv(fetched.Data))
	}
}

var appsCmd = &cobra.Command{
	Use:     "apps",
	Aliases: []string{"app"},
	Short:   "Manage applications",
	Long: `List, create, edit, suspend, and inspect applications on the selected server.

  cipi-cli apps list
  cipi-cli apps show myapp
  cipi-cli apps create
  cipi-cli apps logs myapp --type laravel

For deploys use 'cipi-cli deploy <app>'. For SSL use 'cipi-cli ssl install <app>'.

` + multiServerTip,
	Example: `  cipi-cli apps list
  cipi-cli prod apps list
  cipi-cli apps show myapp
  cipi-cli apps create --user myapp --domain example.com --php 8.4
  cipi-cli apps logs myapp --type laravel`,
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all applications",
	Long: `List applications on the active/default server.

  cipi-cli apps list
  cipi-cli prod apps list`,
	Example: `  cipi-cli apps list
  cipi-cli staging apps list
  cipi-cli apps list --json`,
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
		t := output.NewTable("APP", "ENV", "DOMAIN", "PHP", "REPOSITORY", "BRANCH", "SUSPENDED")
		for _, app := range result.Data {
			t.Row(
				str(app, "app"),
				appEnv(app),
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
	Use:   "show <name>",
	Short: "Show application details",
	Long: `Show domain, environment label, PHP version, repository, branch, aliases,
and other details for one application.

  cipi-cli apps show myapp
  cipi-cli prod apps show myapp`,
	Example: `  cipi-cli apps show myapp
  cipi-cli staging apps show myapp`,
	Args: cobra.ExactArgs(1),
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
		output.KeyValue(nil, "Environment", appEnv(app))
		output.KeyValue(nil, "Domain", str(app, "domain"))
		output.KeyValue(nil, "PHP", str(app, "php"))
		output.KeyValue(nil, "Repository", str(app, "repository"))
		output.KeyValue(nil, "Branch", str(app, "branch"))
		output.KeyValue(nil, "User", str(app, "user"))
		output.KeyValue(nil, "Custom", str(app, "custom"))
		output.KeyValue(nil, "Docroot", str(app, "docroot"))
		output.KeyValue(nil, "Suspended", str(app, "suspended"))
		output.KeyValue(nil, "Created", str(app, "created_at"))
		printAppExtras(app)

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
	Long: `Create a new application on the selected server.

Interactive prompts fill missing flags. Use --custom for non-Laravel apps
(SFTP / custom docroot) and --node for Node apps (Cipi 5.4.0+; no PHP, a
repository is required).

--node-build sets the asset build a Laravel app runs inside every deploy,
before the release goes live (e.g. "npm ci && npm run build"). It must start
with npm, npx, yarn, pnpm, bun or node; && and ; are allowed, pipes,
redirects, backticks and $( ) are not; 200 characters at most. Not for custom
or Node apps (Node apps take --build). Requires Cipi 5.6.0+ / API 1.34+.

--env labels the environment of the app: production (the default), staging or
any 1-32 character label of lowercase letters, digits, - or _. The label is
shown by apps list/show and selects the file the yml commands read:
cipi-<env>.yml first, then cipi.yml. Requires Cipi 5.8.0+ / API 1.36+; older
servers drop it and the CLI says so.

When the job completes the CLI prints the new app's credentials, deploy key
and webhook, and (Cipi 5.6.0+ / API 1.34+) whether the deploy key can read the
repository.

  cipi-cli apps create
  cipi-cli apps create --user myapp --domain example.com --php 8.4 \
    --repository git@github.com:org/repo.git --branch main
  cipi-cli apps create --user shop-staging --domain staging.shop.com --php 8.4 \
    --repository git@github.com:org/shop.git --branch develop --env staging`,
	Example: `  cipi-cli apps create
  cipi-cli apps create --user myapp --domain example.com --php 8.4 \
    --repository git@github.com:org/repo.git --branch main
  cipi-cli apps create --user shop-staging --domain staging.shop.com --php 8.4 \
    --repository git@github.com:org/shop.git --branch develop --env staging
  cipi-cli apps create --user myapp --domain example.com --php 8.4 \
    --repository git@github.com:org/repo.git --branch main \
    --node-build "npm ci && npm run build"
  cipi-cli apps create --user static --domain site.com --php 8.4 --custom --docroot public
  cipi-cli apps create --user web --domain web.com --node ssr --framework next \
    --repository git@github.com:org/web.git --branch main`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Refuse a bad label before touching the config or the server.
		env, _ := cmd.Flags().GetString("env")
		if env != "" {
			if err := validateEnvLabel(env); err != nil {
				output.Error("%s", err)
				return err
			}
		}

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
		engine, _ := cmd.Flags().GetString("engine")
		octane, _ := cmd.Flags().GetString("octane")
		nodeMode, _ := cmd.Flags().GetString("node")
		framework, _ := cmd.Flags().GetString("framework")
		nodeVersion, _ := cmd.Flags().GetString("node-version")
		build, _ := cmd.Flags().GetString("build")
		start, _ := cmd.Flags().GetString("start")
		outputDir, _ := cmd.Flags().GetString("output")
		healthPath, _ := cmd.Flags().GetString("health-path")
		nodeBuild, _ := cmd.Flags().GetString("node-build")

		// Prompt for missing required fields; once in the interactive flow, also
		// offer the optional asset build (Laravel apps only).
		interactive := false
		prompt := func(label string) string {
			interactive = true
			return output.ReadInput(label)
		}
		if user == "" {
			user = prompt("App username")
		}
		if domain == "" {
			domain = prompt("Domain")
		}
		// Node apps refuse a PHP version (Cipi 5.4.0+).
		if php == "" && nodeMode == "" {
			php = prompt("PHP version (8.3/8.4/8.5)")
		}
		if repository == "" && !custom {
			repository = prompt("Git repository SSH URL")
		}
		if branch == "" && !custom {
			branch = prompt("Git branch")
		}
		isLaravel := !custom && nodeMode == "" && framework == ""
		if interactive && isLaravel && nodeBuild == "" && !cmd.Flags().Changed("node-build") {
			nodeBuild = output.ReadLine("Build command, e.g. npm ci && npm run build (optional, Enter to skip)")
		}

		if !custom && repository == "" {
			output.Error("Repository is required")
			return fmt.Errorf("repository is required")
		}

		body := map[string]interface{}{
			"user":   user,
			"domain": domain,
		}
		if php != "" {
			body["php"] = php
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
		if engine != "" {
			body["engine"] = engine
		}
		if octane != "" {
			switch octane {
			case "true", "1", "yes":
				body["octane"] = true
			case "false", "0", "no":
				body["octane"] = false
			default:
				body["octane"] = octane
			}
		}
		if nodeMode != "" {
			body["node"] = nodeMode
		}
		if framework != "" {
			body["framework"] = framework
		}
		if nodeVersion != "" {
			body["node_version"] = nodeVersion
		}
		if build != "" {
			body["build"] = build
		}
		if start != "" {
			body["start"] = start
		}
		if outputDir != "" {
			body["output"] = outputDir
		}
		if healthPath != "" {
			body["health_path"] = healthPath
		}
		if nodeBuild != "" {
			body["node_build"] = nodeBuild
		}
		if env != "" {
			body["env"] = env
		}

		output.Info("Creating app '%s'...", user)
		job, err := client.DoAsyncAndWaitResult("POST", "/api/apps", body)
		if err != nil {
			output.Error("Failed to create app: %s", err)
			return err
		}

		result, _ := jobResultMap(job)
		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": result})
			return nil
		}

		output.Success("App '%s' created successfully", user)
		printAppCreateResult(result)
		// Servers before API 1.34 drop node_build silently: say so when the
		// result came back without the build it should echo.
		if nodeBuild != "" && result != nil {
			if _, ok := result["build"]; !ok {
				output.Warn("The server did not confirm the build command (needs Cipi 5.6.0+ / API 1.34+) — set it with: cipi-cli deploy config set %s --node-build %q", user, nodeBuild)
			}
		}
		if env != "" {
			warnIfEnvDropped(client, user, env, result)
		}
		fmt.Println()
		return nil
	},
}

var appsEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit an existing application",
	Long: `Update PHP version, repository, branch, primary domain, environment label,
and/or Node settings.

Pass at least one flag: --php, --repository, --branch, --domain, --env, or a
Node flag (--node, --node-version, --build, --start, --output, --health-path).
--node-version also pins a Laravel app to a Node major ('default' unpins it).
--env relabels the environment (production, staging, …; Cipi 5.8.0+ / API
1.36+) and with it the cipi-<env>.yml the yml commands read.

With Cipi 5.6.0+ / API 1.34+, changing --repository or --branch also reports
whether the deploy key can read the repository (and the fix to run if not).

  cipi-cli apps edit myapp --php 8.4
  cipi-cli apps edit myapp --branch develop --domain new.example.com
  cipi-cli apps edit myapp --env staging`,
	Example: `  cipi-cli apps edit myapp --php 8.4
  cipi-cli apps edit myapp --repository git@github.com:org/repo.git --branch main
  cipi-cli apps edit myapp --domain new.example.com
  cipi-cli apps edit myapp --env staging`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Refuse a bad label before touching the config or the server.
		env, _ := cmd.Flags().GetString("env")
		if env != "" {
			if err := validateEnvLabel(env); err != nil {
				output.Error("%s", err)
				return err
			}
		}

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
		if v, _ := cmd.Flags().GetString("node"); v != "" {
			body["node"] = v
		}
		if v, _ := cmd.Flags().GetString("node-version"); v != "" {
			body["node_version"] = v
		}
		if v, _ := cmd.Flags().GetString("build"); v != "" {
			body["build"] = v
		}
		if v, _ := cmd.Flags().GetString("start"); v != "" {
			body["start"] = v
		}
		if v, _ := cmd.Flags().GetString("output"); v != "" {
			body["output"] = v
		}
		if v, _ := cmd.Flags().GetString("health-path"); v != "" {
			body["health_path"] = v
		}
		if env != "" {
			body["env"] = env
		}

		if len(body) == 0 {
			output.Error("No fields to update — use --php, --repository, --branch, --domain, --env, or Node flags")
			return fmt.Errorf("no fields specified")
		}

		output.Info("Updating app '%s'...", args[0])
		job, err := client.DoAsyncAndWaitResult("PUT", fmt.Sprintf("/api/apps/%s", args[0]), body)
		if err != nil {
			output.Error("Failed to update app: %s", err)
			return err
		}

		result, _ := jobResultMap(job)
		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": result})
			return nil
		}

		output.Success("App '%s' updated successfully", args[0])
		// Cipi 5.6.0+ / API 1.34+ re-checks the deploy key when the repository
		// or branch changes; older servers report nothing here.
		if ra, ok := result["repository_access"].(map[string]interface{}); ok {
			printRepositoryAccess(ra, "next")
		}
		if env != "" {
			warnIfEnvDropped(client, args[0], env, result)
		}
		fmt.Println()
		return nil
	},
}

var appsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an application permanently",
	Long: `Delete an application and its site from the server. This cannot be undone.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli apps delete myapp
  cipi-cli apps delete myapp -y`,
	Example: `  cipi-cli apps delete myapp
  cipi-cli apps delete myapp -y`,
	Args: cobra.ExactArgs(1),
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
	Use:   "suspend <name>",
	Short: "Suspend an application (HTTP 503 maintenance page)",
	Long: `Suspend an application so visitors see an HTTP 503 page.

Use 'cipi-cli apps unsuspend <name>' to bring it back online.

  cipi-cli apps suspend myapp
  cipi-cli prod apps suspend myapp`,
	Example: `  cipi-cli apps suspend myapp
  cipi-cli staging apps suspend myapp`,
	Args: cobra.ExactArgs(1),
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
	Use:   "logs <name>",
	Short: "Read application logs",
	Long: `Read paginated log snapshots for an application.

Types: all, nginx, php, worker, deploy, laravel.
Page 1 returns the most recent lines.

  cipi-cli apps logs myapp
  cipi-cli apps logs myapp --type laravel --page 1 --per-page 100`,
	Example: `  cipi-cli apps logs myapp
  cipi-cli apps logs myapp --type nginx
  cipi-cli apps logs myapp -t laravel -p 2 --per-page 100
  cipi-cli prod apps logs myapp --type deploy`,
	Args: cobra.ExactArgs(1),
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
	Use:   "unsuspend <name>",
	Short: "Unsuspend an application (bring it back online)",
	Long: `Remove the maintenance/503 page and bring the application back online.

  cipi-cli apps unsuspend myapp
  cipi-cli prod apps unsuspend myapp`,
	Example: `  cipi-cli apps unsuspend myapp
  cipi-cli staging apps unsuspend myapp`,
	Args: cobra.ExactArgs(1),
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

// printAppExtras prints the optional fields exposed by newer Cipi API versions
// (engine, Octane, Node, routing, basic auth); absent or empty fields are skipped.
func printAppExtras(app map[string]interface{}) {
	optional := func(label, key string) {
		v, ok := app[key]
		if !ok || v == nil || v == "" {
			return
		}
		output.KeyValue(nil, label, formatFieldValue(v))
	}
	optional("Engine", "engine")
	optional("Octane", "octane")
	optional("Octane port", "octane_port")
	if boolVal(app, "node") {
		output.KeyValue(nil, "Node", "yes")
		optional("Node mode", "node_mode")
	}
	optional("Node version", "node_version")
	optional("Force HTTPS", "force_https")
	optional("WWW redirect", "www_redirect")
	optional("Basic auth", "basic_auth")
	if r, ok := app["redirect"].(map[string]interface{}); ok && len(r) > 0 {
		redirect := str(r, "to")
		if code := str(r, "code"); code != "—" {
			redirect += " (" + code + ")"
		}
		if enabled, ok := r["enabled"].(bool); ok && !enabled {
			redirect += " [disabled]"
		}
		output.KeyValue(nil, "Redirect", redirect)
	}
	if n := len(sliceVal(app, "redirects")); n > 0 {
		output.KeyValue(nil, "Path redirects", fmt.Sprintf("%d (cipi-cli redirect list %s)", n, str(app, "app")))
	}
	if n := len(sliceVal(app, "proxies")); n > 0 {
		output.KeyValue(nil, "Proxies", fmt.Sprintf("%d (cipi-cli proxies list %s)", n, str(app, "app")))
	}
}

func sliceVal(m map[string]interface{}, key string) []interface{} {
	v, _ := m[key].([]interface{})
	return v
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
	appsCreateCmd.Flags().String("engine", "", "Database engine (mariadb|pgsql, Cipi 4.8+)")
	appsCreateCmd.Flags().String("octane", "", "Octane mode (true|false|frankenphp, Cipi 5.0+)")
	appsCreateCmd.Flags().String("node", "", "Node app mode (spa|static|ssr, Cipi 5.4.0+)")
	appsCreateCmd.Flags().String("framework", "", "Node framework preset (next, nuxt, sveltekit, astro, remix, vite)")
	appsCreateCmd.Flags().String("node-version", "", "Node major version")
	appsCreateCmd.Flags().String("build", "", "Node build command")
	appsCreateCmd.Flags().String("start", "", "Node start command (SSR)")
	appsCreateCmd.Flags().String("output", "", "Node build output directory (spa/static)")
	appsCreateCmd.Flags().String("health-path", "", "Node health path (SSR)")
	appsCreateCmd.Flags().String("node-build", "", `Laravel asset build run inside every deploy, e.g. "npm ci && npm run build" (Cipi 5.6.0+ / API 1.34+; not for custom/Node apps)`)
	appsCreateCmd.Flags().String("env", "", "environment label: production (default), staging, …")

	appsEditCmd.Flags().String("php", "", "PHP version")
	appsEditCmd.Flags().String("repository", "", "Git repository SSH URL")
	appsEditCmd.Flags().String("branch", "", "Git branch")
	appsEditCmd.Flags().String("domain", "", "New primary domain (requires Cipi 4.6.2+ / API 1.9.0+)")
	appsEditCmd.Flags().String("node", "", "Node app mode (spa|static|ssr)")
	appsEditCmd.Flags().String("node-version", "", "Node major version")
	appsEditCmd.Flags().String("build", "", "Node build command")
	appsEditCmd.Flags().String("start", "", "Node start command (SSR)")
	appsEditCmd.Flags().String("output", "", "Node build output directory")
	appsEditCmd.Flags().String("health-path", "", "Node health path (SSR)")
	appsEditCmd.Flags().String("env", "", "environment label: production (default), staging, …")

	appsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	appsLogsCmd.Flags().StringP("type", "t", "all", "Log type (all, nginx, php, worker, deploy, laravel)")
	appsLogsCmd.Flags().IntP("page", "p", 1, "Page number (1 = most recent lines)")
	appsLogsCmd.Flags().Int("per-page", 50, "Lines per log file per page (max 1000)")

	appsCmd.AddCommand(appsListCmd, appsShowCmd, appsCreateCmd, appsEditCmd, appsDeleteCmd, appsLogsCmd, appsSuspendCmd, appsUnsuspendCmd)
	rootCmd.AddCommand(appsCmd)
}
