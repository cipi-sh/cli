package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// diskTimeout replaces the default 30 s client timeout for GET /api/disk*:
// the server measures every app home and asks each engine when the request
// arrives, so a server with large apps takes a while to answer.
const diskTimeout = 180 * time.Second

type diskUsageData struct {
	Disk struct {
		Mount       string  `json:"mount"`
		SizeGB      float64 `json:"size_gb"`
		UsedGB      float64 `json:"used_gb"`
		FreeGB      float64 `json:"free_gb"`
		UsedPercent int     `json:"used_percent"`
	} `json:"disk"`
	Apps []struct {
		App          string   `json:"app"`
		FilesGB      float64  `json:"files_gb"`
		DatabaseGB   float64  `json:"database_gb"`
		TotalGB      float64  `json:"total_gb"`
		Percent      float64  `json:"percent"`
		FilesKB      int64    `json:"files_kb"`
		DatabaseKB   int64    `json:"database_kb"`
		TotalKB      int64    `json:"total_kb"`
		LimitGB      *float64 `json:"limit_gb"`
		LimitPercent *int     `json:"limit_percent"`
		OverLimit    bool     `json:"over_limit"`
	} `json:"apps"`
	AppsTotalGB  float64 `json:"apps_total_gb"`
	AppsPercent  float64 `json:"apps_percent"`
	OtherGB      float64 `json:"other_gb"`
	OtherPercent float64 `json:"other_percent"`
}

type diskEngineData struct {
	Engine    string `json:"engine"`
	Databases []struct {
		Name      string   `json:"name"`
		SizeMB    *float64 `json:"size_mb"`
		Keys      *int64   `json:"keys"`
		Documents *int64   `json:"documents"`
	} `json:"databases"`
	OnDiskMB *float64 `json:"on_disk_mb"`
	MemoryMB *float64 `json:"memory_mb"`
	Note     *string  `json:"note"`
}

var diskCmd = &cobra.Command{
	Use:   "disk",
	Short: "Disk usage: the server and every app, or every database",
	Long: `Who is filling the disk — the figures of "cipi disk" on the host, read from
GET /api/disk and GET /api/disk/dbs.

  cipi-cli disk            the filesystem /home is on, then every app (GB and %)
  cipi-cli disk db         every database per engine, in MB
  cipi-cli disk --json     the same figures for scripts

An app's share is its home (releases, shared storage, logs) plus its database.
The server measures sizes when asked, so a server with large apps takes a few
seconds. A soft limit per app is set on the host with
"cipi app limits <app> --disk=<GB>" (alerts only, nothing is blocked).

Requires Cipi 5.5.2+ (API sudoers), cipi/api 1.33+ and the disk-view ability.

` + multiServerTip,
	Example: `  cipi-cli disk
  cipi-cli prod disk
  cipi-cli disk db
  cipi-cli disk --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := diskClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get("/api/disk", &result); err != nil {
			output.Error("Failed to read disk usage: %s", diskErrorHint(err))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data diskUsageData
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse disk usage: %s", err)
			return err
		}

		printDiskUsage(&data)
		return nil
	},
}

var diskDbCmd = &cobra.Command{
	Use:     "db",
	Aliases: []string{"dbs", "databases"},
	Short:   "Size of every database per engine (MB)",
	Long: `Every database of every installed engine, in MB — "cipi disk db" on the host,
read from GET /api/disk/dbs.

MariaDB and PostgreSQL report one size per database. Valkey only knows how
many keys each of its databases holds, and Meilisearch how many documents each
index has: for those two the size is the engine's, in memory and on disk.
Engines that are not installed are left out.

  cipi-cli disk db
  cipi-cli prod disk db --json`,
	Example: `  cipi-cli disk db
  cipi-cli prod disk db
  cipi-cli disk db --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := diskClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get("/api/disk/dbs", &result); err != nil {
			output.Error("Failed to read database sizes: %s", diskErrorHint(err))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var engines []diskEngineData
		if err := json.Unmarshal(result.Data, &engines); err != nil {
			output.Error("Failed to parse database sizes: %s", err)
			return err
		}

		printDiskDatabases(engines)
		return nil
	},
}

func diskClient() (*api.Client, error) {
	client, err := mustClient()
	if err != nil {
		return nil, err
	}
	client.HTTPClient.Timeout = diskTimeout
	return client, nil
}

// diskErrorHint adds what a 503 from these endpoints usually means: the server
// runs a Cipi older than 5.5.2, whose API sudoers do not allow `cipi disk`.
func diskErrorHint(err error) string {
	msg := apiErrorHint(err, "1.33.0", "disk-view")
	if strings.Contains(err.Error(), "HTTP 503") || strings.Contains(err.Error(), "terminal is required") {
		msg += " — the server needs Cipi 5.5.2+ (cipi self-update) so the API may run cipi disk"
	}
	return msg
}

func printDiskUsage(data *diskUsageData) {
	output.Header("Server disk")
	fs := output.NewTable("MOUNT", "SIZE", "USED", "FREE", "USE")
	fs.Row(
		dash(data.Disk.Mount),
		fmt.Sprintf("%.2f GB", data.Disk.SizeGB),
		fmt.Sprintf("%.2f GB", data.Disk.UsedGB),
		fmt.Sprintf("%.2f GB", data.Disk.FreeGB),
		colorPercent(fmt.Sprintf("%d%%", data.Disk.UsedPercent), float64(data.Disk.UsedPercent), 80, 90),
	)
	fs.Flush()

	output.Header(fmt.Sprintf("Apps (%d) — %% of the %.2f GB on %s", len(data.Apps), data.Disk.SizeGB, dash(data.Disk.Mount)))
	if len(data.Apps) == 0 {
		output.Warn("No apps yet")
	} else {
		over := 0
		t := output.NewTable("APP", "FILES", "DATABASE", "TOTAL", "DISK", "LIMIT")
		for _, a := range data.Apps {
			limit := "—"
			if a.LimitGB != nil {
				pct := 0
				if a.LimitPercent != nil {
					pct = *a.LimitPercent
				}
				limit = fmt.Sprintf("%s GB (%d%%)", formatGB(*a.LimitGB), pct)
				switch {
				case a.OverLimit:
					over++
					limit = output.Red.Sprint(limit + " over")
				case pct >= 90:
					limit = output.Yellow.Sprint(limit)
				}
			}
			t.Row(
				a.App,
				diskHuman(a.FilesKB, a.FilesGB),
				diskHuman(a.DatabaseKB, a.DatabaseGB),
				output.Cyan.Sprint(diskHuman(a.TotalKB, a.TotalGB)),
				fmt.Sprintf("%.1f%%", a.Percent),
				limit,
			)
		}
		t.Row(output.Bold.Sprint("All apps"), "", "", output.Bold.Sprintf("%.2f GB", data.AppsTotalGB), output.Bold.Sprintf("%.1f%%", data.AppsPercent), "")
		t.Row("Everything else", "", "", fmt.Sprintf("%.2f GB", data.OtherGB), fmt.Sprintf("%.1f%%", data.OtherPercent), "")
		t.Flush()
		if over > 0 {
			output.Error("%d app(s) over the limit", over)
		}
	}
	output.Dim.Println("  Everything else: system, packages, logs, local backups, other databases.")
	output.Dim.Println("  Every database: cipi-cli disk db · Limit: cipi app limits <app> --disk=<GB>|none on the host (alerts, blocks nothing)")
	fmt.Println()
}

func printDiskDatabases(engines []diskEngineData) {
	if len(engines) == 0 {
		output.Warn("No database engine found")
		fmt.Println()
		return
	}

	for _, e := range engines {
		output.Header(diskEngineLabel(e.Engine) + " — sizes in MB")
		if e.Note != nil && *e.Note != "" {
			output.Warn("%s", *e.Note)
		}

		var t *output.Table
		switch e.Engine {
		case "valkey":
			t = output.NewTable("DATABASE", "KEYS", "SIZE")
		case "meilisearch":
			t = output.NewTable("INDEX", "DOCUMENTS", "SIZE")
		default:
			t = output.NewTable("DATABASE", "SIZE")
		}

		if len(e.Databases) == 0 {
			switch e.Engine {
			case "valkey":
				output.Dim.Println("  No keys stored")
			case "meilisearch":
				output.Dim.Println("  No indexes")
			default:
				output.Dim.Println("  No databases")
			}
		} else {
			for _, db := range e.Databases {
				switch e.Engine {
				case "valkey":
					t.Row(db.Name, formatCount(db.Keys), formatMB(db.SizeMB))
				case "meilisearch":
					t.Row(db.Name, formatCount(db.Documents), formatMB(db.SizeMB))
				default:
					t.Row(db.Name, formatMB(db.SizeMB))
				}
			}
			t.Flush()
		}

		if e.MemoryMB != nil {
			output.KeyValue(nil, "Memory in use", formatMB(e.MemoryMB))
		}
		if e.OnDiskMB != nil {
			output.KeyValue(nil, "On disk", formatMB(e.OnDiskMB)+" (whole engine)")
		}
		fmt.Println()
	}
}

func diskEngineLabel(engine string) string {
	switch engine {
	case "mariadb":
		return "MariaDB"
	case "pgsql":
		return "PostgreSQL"
	case "valkey":
		return "Valkey"
	case "meilisearch":
		return "Meilisearch"
	default:
		return engine
	}
}

// diskHuman mirrors `cipi disk`: GB with two decimals, MB below 0.01 GB so a
// small database does not read as zero. kb is preferred; gb is the fallback
// when the API did not send the KiB figure.
func diskHuman(kb int64, gb float64) string {
	if kb <= 0 && gb > 0 {
		kb = int64(gb * 1048576)
	}
	switch {
	case kb >= 10486:
		return fmt.Sprintf("%.2f GB", float64(kb)/1048576)
	case kb >= 103:
		return fmt.Sprintf("%.1f MB", float64(kb)/1024)
	case kb > 0:
		return "<0.1 MB"
	default:
		return "0.00 GB"
	}
}

func formatGB(gb float64) string {
	s := fmt.Sprintf("%.2f", gb)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func formatMB(mb *float64) string {
	if mb == nil {
		return "—"
	}
	if *mb > 0 && *mb < 0.1 {
		return "<0.1 MB"
	}
	return fmt.Sprintf("%.1f MB", *mb)
}

func formatCount(n *int64) string {
	if n == nil {
		return "—"
	}
	return fmt.Sprintf("%d", *n)
}

func colorPercent(label string, value, warn, danger float64) string {
	switch {
	case value >= danger:
		return output.Red.Sprint(label)
	case value >= warn:
		return output.Yellow.Sprint(label)
	default:
		return output.Green.Sprint(label)
	}
}

func init() {
	diskCmd.AddCommand(diskDbCmd)
	rootCmd.AddCommand(diskCmd)
}
