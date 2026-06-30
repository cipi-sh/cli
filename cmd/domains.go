package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

type domainRow struct {
	Domain     string `json:"domain"`
	App        string `json:"app"`
	Kind       string `json:"kind"`
	Type       string `json:"type"`
	PHP        string `json:"php"`
	Docroot    string `json:"docroot"`
	Branch     string `json:"branch"`
	Repository string `json:"repository"`
	Suspended  bool   `json:"suspended"`
}

var domainsCmd = &cobra.Command{
	Use:     "domains",
	Aliases: []string{"domain"},
	Short:   "List all domains and aliases across every app",
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
			output.Error("Failed to list domains: %s", err)
			return err
		}

		if err := enrichApps(client, result.Data); err != nil {
			output.Error("Failed to load app details: %s", err)
			return err
		}

		rows := buildDomainRows(result.Data)

		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": rows})
			return nil
		}

		if len(rows) == 0 {
			output.Warn("No domains found")
			return nil
		}

		output.Header("Domains")
		t := output.NewTable("DOMAIN", "APP", "KIND", "TYPE", "PHP", "DOCROOT", "BRANCH", "REPOSITORY", "STATUS")
		suspendedApps := map[string]struct{}{}
		for _, row := range rows {
			if row.Suspended {
				suspendedApps[row.App] = struct{}{}
			}
			t.Row(
				row.Domain,
				row.App,
				output.KindBadge(row.Kind),
				row.Type,
				row.PHP,
				row.Docroot,
				row.Branch,
				truncate(row.Repository, 40),
				output.StatusSuspended(boolSuspended(row.Suspended)),
			)
		}
		t.Flush()

		suspNote := ""
		if len(suspendedApps) > 0 {
			suspNote = fmt.Sprintf(" — %d suspended", len(suspendedApps))
		}
		output.Footer("%d domain(s) across %d app(s)%s", len(rows), len(result.Data), suspNote)
		return nil
	},
}

func boolSuspended(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func buildDomainRows(apps []map[string]interface{}) []domainRow {
	rows := make([]domainRow, 0, len(apps))
	for _, app := range apps {
		appName := str(app, "app")
		if appName == "—" {
			continue
		}

		suspended := boolVal(app, "suspended")
		php := str(app, "php")
		branch := str(app, "branch")
		if branch == "—" {
			branch = "-"
		}
		repo := formatDomainRepository(app)
		appType := domainAppType(app)
		docroot := domainAppDocroot(app)

		domain := str(app, "domain")
		if domain != "—" {
			rows = append(rows, domainRow{
				Domain:     domain,
				App:        appName,
				Kind:       "primary",
				Type:       appType,
				PHP:        php,
				Docroot:    docroot,
				Branch:     branch,
				Repository: repo,
				Suspended:  suspended,
			})
		}

		for _, alias := range stringSlice(app, "aliases") {
			rows = append(rows, domainRow{
				Domain:     alias,
				App:        appName,
				Kind:       "alias",
				Type:       appType,
				PHP:        php,
				Docroot:    docroot,
				Branch:     branch,
				Repository: repo,
				Suspended:  suspended,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Domain < rows[j].Domain
	})
	return rows
}

func enrichApps(client *api.Client, apps []map[string]interface{}) error {
	for i, app := range apps {
		name := str(app, "app")
		if name == "—" {
			continue
		}

		var show struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(fmt.Sprintf("/api/apps/%s", name), &show); err != nil {
			return fmt.Errorf("app %s: %w", name, err)
		}

		for _, key := range []string{"custom", "docroot"} {
			if v, ok := show.Data[key]; ok {
				apps[i][key] = v
			}
		}
	}
	return nil
}

func domainAppType(app map[string]interface{}) string {
	if boolVal(app, "custom") {
		return "Custom"
	}
	return "Laravel"
}

func domainAppDocroot(app map[string]interface{}) string {
	if boolVal(app, "custom") {
		docroot := str(app, "docroot")
		if docroot == "—" || docroot == "" {
			return "/"
		}
		return "/" + strings.TrimPrefix(docroot, "/")
	}
	return "public"
}

func formatDomainRepository(app map[string]interface{}) string {
	repo := str(app, "repository")
	if repo != "—" && repo != "" {
		return repo
	}
	if boolVal(app, "custom") {
		return "(SFTP only)"
	}
	return "—"
}

func boolVal(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(val, "true") || val == "yes" || val == "1"
	case float64:
		return val != 0
	default:
		return fmt.Sprintf("%v", val) == "true"
	}
}

func stringSlice(m map[string]interface{}, key string) []string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(domainsCmd)
}
