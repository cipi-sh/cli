package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

type serverStatusData struct {
	System    statusSystem      `json:"system"`
	Resources statusResources   `json:"resources"`
	Services  map[string]string `json:"services"`
	PHP       []statusPHP       `json:"php"`
	Apps      int               `json:"apps"`
}

type statusSystem struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Uptime   string `json:"uptime"`
	Cipi     string `json:"cipi"`
}

type statusResources struct {
	CPU    statusCPU     `json:"cpu"`
	Memory *statusMemory `json:"memory"`
	Disk   *statusDisk   `json:"disk"`
}

type statusCPU struct {
	UsagePercent *int `json:"usage_percent"`
}

type statusMemory struct {
	UsedMB       int `json:"used_mb"`
	TotalMB      int `json:"total_mb"`
	UsagePercent int `json:"usage_percent"`
}

type statusDisk struct {
	Display      string `json:"display"`
	Used         string `json:"used"`
	Total        string `json:"total"`
	UsagePercent int    `json:"usage_percent"`
}

type statusPHP struct {
	Version string `json:"version"`
	Status  string `json:"status"`
	Pools   int    `json:"pools"`
}

type profileStatusResult struct {
	Profile  string            `json:"profile"`
	Endpoint string            `json:"endpoint"`
	Default  bool              `json:"default,omitempty"`
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	Data     *serverStatusData `json:"data,omitempty"`
}

var statusCmd = &cobra.Command{
	Use:   "status [profile]",
	Short: "Show server status (all profiles, or one in detail)",
	Long: `Show server status from GET /api/status (same data as "cipi status" on the host).

Requires the status-view ability on the API token.

  cipi-cli status              global overview — one row per profile
  cipi-cli status prod         full details for one server
  cipi-cli prod status         same, via profile prefix

` + multiServerTip,
	Example: `  cipi-cli status
  cipi-cli status prod
  cipi-cli staging status
  cipi-cli status --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bare "status" / "status all" → global overview table.
		if len(args) == 0 || (len(args) == 1 && strings.EqualFold(args[0], "all")) {
			return runStatusGlobal()
		}
		return runStatusDetail(args[0])
	},
}

func runStatusDetail(profileName string) error {
	var (
		client *api.Client
		name   string
		err    error
	)

	if profileName != "" {
		name = profileName
		client, err = api.NewClientForProfile(profileName)
	} else {
		var profile *config.Profile
		profile, name, err = config.LoadWithName()
		if err == nil {
			client = api.NewClientFromProfile(profile)
		}
	}
	if err != nil {
		output.Error("%s", err)
		return err
	}

	data, err := fetchStatus(client)
	if err != nil {
		msg := api.RouteNotFoundHint(err, "latest", "GET /api/status + status-view")
		output.Error("Failed to get status for %q: %s", name, msg)
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]interface{}{
			"profile":  name,
			"endpoint": client.BaseURL,
			"data":     data,
		})
		return nil
	}

	printStatusDetail(name, client.BaseURL, data)
	return nil
}

func runStatusGlobal() error {
	// If a profile prefix was used (cipi-cli prod status), show detail for that server.
	if config.ActiveProfile() != "" {
		return runStatusDetail(config.ActiveProfile())
	}

	names, defaultProfile, err := config.ListProfiles()
	if err != nil {
		output.Error("%s", err)
		return err
	}
	if len(names) == 0 {
		output.Warn("No servers configured")
		return nil
	}

	results := collectProfileStatuses(names, defaultProfile)

	if jsonFlag {
		output.PrintJSON(map[string]interface{}{
			"default": defaultProfile,
			"servers": results,
		})
		return nil
	}

	output.Header("Servers")
	t := output.NewTable("NAME", "IP", "CPU", "RAM", "HDD", "APPS", "SVC", "CIPI")
	failed := 0
	for _, r := range results {
		label := r.Profile
		if r.Default {
			label = r.Profile + "*"
		}
		if !r.OK || r.Data == nil {
			failed++
			errMsg := compactError(r.Error)
			t.Row(label, "—", "—", "—", "—", "—", errMsg, "—")
			continue
		}
		d := r.Data
		t.Row(
			label,
			dash(d.System.IP),
			formatCPU(d.Resources.CPU),
			formatRamRow(d.Resources.Memory),
			formatHddRow(d.Resources.Disk),
			fmt.Sprintf("%d", d.Apps),
			servicesSummary(d.Services),
			dash(d.System.Cipi),
		)
	}
	t.Flush()
	output.Dim.Println("  * default profile")
	output.Dim.Println("  Detail: cipi-cli status <name>")
	fmt.Println()

	if failed > 0 {
		output.Warn("%d of %d servers unreachable", failed, len(results))
		fmt.Println()
	}
	return nil
}

func collectProfileStatuses(names []string, defaultProfile string) []profileStatusResult {
	results := make([]profileStatusResult, 0, len(names))
	for _, name := range names {
		profile, err := config.LoadNamed(name)
		if err != nil {
			results = append(results, profileStatusResult{
				Profile: name,
				Default: name == defaultProfile,
				OK:      false,
				Error:   err.Error(),
			})
			continue
		}
		client := api.NewClientFromProfile(profile)
		data, err := fetchStatus(client)
		if err != nil {
			results = append(results, profileStatusResult{
				Profile:  name,
				Endpoint: client.BaseURL,
				Default:  name == defaultProfile,
				OK:       false,
				Error:    api.RouteNotFoundHint(err, "latest", "GET /api/status + status-view"),
			})
			continue
		}
		results = append(results, profileStatusResult{
			Profile:  name,
			Endpoint: client.BaseURL,
			Default:  name == defaultProfile,
			OK:       true,
			Data:     data,
		})
	}
	return results
}

func fetchStatus(client *api.Client) (*serverStatusData, error) {
	var result struct {
		Data serverStatusData `json:"data"`
	}
	if err := client.Get("/api/status", &result); err != nil {
		return nil, err
	}
	return &result.Data, nil
}

func printStatusDetail(profile, endpoint string, data *serverStatusData) {
	title := "Server status"
	if profile != "" {
		title = fmt.Sprintf("Server status — %s", profile)
	}
	output.Header(title)
	if endpoint != "" {
		output.KeyValue(nil, "Endpoint", endpoint)
	}
	output.KeyValue(nil, "Hostname", dash(data.System.Hostname))
	output.KeyValue(nil, "IP", dash(data.System.IP))
	output.KeyValue(nil, "OS", dash(data.System.OS))
	output.KeyValue(nil, "Uptime", dash(data.System.Uptime))
	output.KeyValue(nil, "Cipi", dash(data.System.Cipi))
	output.KeyValue(nil, "CPU", formatCPU(data.Resources.CPU))
	output.KeyValue(nil, "RAM", formatMem(data.Resources.Memory))
	output.KeyValue(nil, "HDD", formatDisk(data.Resources.Disk))
	output.KeyValue(nil, "Apps", fmt.Sprintf("%d", data.Apps))
	fmt.Println()

	if len(data.Services) > 0 {
		output.Bold.Println("  Services")
		names := make([]string, 0, len(data.Services))
		for name := range data.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			state := data.Services[name]
			colored := state
			switch strings.ToLower(state) {
			case "running":
				colored = output.Green.Sprint(state)
			case "stopped":
				colored = output.Red.Sprint(state)
			}
			output.KeyValue(nil, name, colored)
		}
		fmt.Println()
	}

	if len(data.PHP) > 0 {
		output.Bold.Println("  PHP")
		for _, php := range data.PHP {
			state := php.Status
			if strings.EqualFold(state, "running") {
				state = output.Green.Sprint(state)
			}
			output.KeyValue(nil, "PHP "+php.Version, fmt.Sprintf("%s (%d pools)", state, php.Pools))
		}
		fmt.Println()
	}
}

func formatCPU(cpu statusCPU) string {
	if cpu.UsagePercent == nil {
		return "—"
	}
	return fmt.Sprintf("%d%%", *cpu.UsagePercent)
}

func formatMem(m *statusMemory) string {
	if m == nil {
		return "—"
	}
	return fmt.Sprintf("%d/%d MB (%d%%)", m.UsedMB, m.TotalMB, m.UsagePercent)
}

func formatRamRow(m *statusMemory) string {
	if m == nil {
		return "—"
	}
	return fmt.Sprintf("%d%% (%dM)", m.UsagePercent, m.UsedMB)
}

func formatDisk(d *statusDisk) string {
	if d == nil {
		return "—"
	}
	if d.Display != "" {
		return d.Display
	}
	if d.Used != "" && d.Total != "" {
		return fmt.Sprintf("%s/%s (%d%%)", d.Used, d.Total, d.UsagePercent)
	}
	return fmt.Sprintf("%d%%", d.UsagePercent)
}

func formatHddRow(d *statusDisk) string {
	if d == nil {
		return "—"
	}
	if d.Used != "" && d.Total != "" {
		return fmt.Sprintf("%s/%s %d%%", d.Used, d.Total, d.UsagePercent)
	}
	if d.Display != "" {
		return d.Display
	}
	return fmt.Sprintf("%d%%", d.UsagePercent)
}

func servicesSummary(services map[string]string) string {
	if len(services) == 0 {
		return "—"
	}
	running := 0
	for _, state := range services {
		if strings.EqualFold(state, "running") {
			running++
		}
	}
	if running == len(services) {
		return output.Green.Sprint("ok")
	}
	return output.Yellow.Sprintf("%d/%d", running, len(services))
}

func compactError(errMsg string) string {
	errMsg = strings.TrimSpace(errMsg)
	switch {
	case strings.Contains(strings.ToLower(errMsg), "unauthorized"),
		strings.Contains(strings.ToLower(errMsg), "unauthenticated"),
		strings.Contains(errMsg, "401"):
		return "auth"
	case strings.Contains(strings.ToLower(errMsg), "forbidden"),
		strings.Contains(errMsg, "403"):
		return "forbidden"
	case strings.Contains(strings.ToLower(errMsg), "could not be found"),
		strings.Contains(errMsg, "404"):
		return "no-status"
	case strings.Contains(strings.ToLower(errMsg), "timeout"),
		strings.Contains(strings.ToLower(errMsg), "connection refused"),
		strings.Contains(strings.ToLower(errMsg), "no such host"):
		return "offline"
	case len(errMsg) > 18:
		return errMsg[:15] + "..."
	default:
		return errMsg
	}
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
