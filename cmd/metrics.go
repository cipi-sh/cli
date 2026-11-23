package cmd

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Metrics & health history — cipi/api 1.41.0, Cipi 5.11.0. All synchronous.

const metricsMinAPI = "1.41.0"

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Metrics & health history: CPU, memory, requests, 5xx, uptime",
	Long: `The server's own history (Cipi 5.11.0+, cipi/api 1.41.0+): every minute the
core records host CPU, load, memory, swap, disk and network, and per app CPU,
memory, PHP-FPM children and max_children hits, requests and 5xx per minute,
queue workers and the healthcheck's status and time.

  cipi-cli metrics                         server and apps now, last hour, uptime 24h
  cipi-cli metrics show --app shop --range 7d
  cipi-cli metrics uptime [shop] --range 30d
  cipi-cli metrics events --source deploy
  cipi-cli metrics settings
  cipi-cli metrics retention --raw 7 --rollup 30 --resolution 15 --events 90
  cipi-cli metrics heartbeat set https://hc-ping.com/<uuid> --every 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/metrics", &result); err != nil {
			output.Error("Failed to read metrics: %s", featureErrorHint(err, metricsMinAPI, "metrics-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		host, _ := result.Data["host"].(map[string]interface{})
		hv, _ := host["values"].(map[string]interface{})
		output.Header("Server")
		fmt.Printf("  CPU %s%%   load %s   memory %s%% (%s MB)   swap %s%%   disk %s%%   net ↓%s ↑%s\n\n",
			metNum(hv["cpu"], 1), metNum(hv["load1"], 2), metNum(hv["mem"], 1), metNum(hv["mem_used_mb"], 0),
			metNum(hv["swap"], 1), metNum(hv["disk"], 0), metRate(hv["net_rx"]), metRate(hv["net_tx"]))

		apps, _ := result.Data["apps"].([]interface{})
		if len(apps) > 0 {
			t := output.NewTable("APP", "CPU%", "MEM MB", "REQ/MIN", "5XX", "HTTP", "UP 24H", "UP 30D", "LAST HOUR (req)")
			for _, a := range apps {
				m, _ := a.(map[string]interface{})
				v, _ := m["values"].(map[string]interface{})
				up, _ := m["uptime"].(map[string]interface{})
				sp, _ := m["spark"].(map[string]interface{})
				http := "—"
				if v["http_code"] != nil {
					http = fmt.Sprintf("%s %sms", metNum(v["http_code"], 0), metNum(v["http_ms"], 0))
				}
				t.Row(metStr(m["app"]), metNum(v["cpu"], 1), metNum(v["mem_mb"], 0), metNum(v["req"], 0), metNum(v["http_5xx"], 0),
					http, metPct(up["24h"]), metPct(up["30d"]), metSpark(metFloats(sp["req"])))
			}
			t.Flush()
		}

		if events, _ := result.Data["events"].([]interface{}); len(events) > 0 {
			output.Header("Recent events")
			metPrintEvents(events, 5)
		}
		return nil
	},
}

var metricsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "A metric over time (host, or an app with --app)",
	Long: `~240 points of [timestamp, average, maximum] per metric. Within the raw
retention (7 days by default) from the per-minute samples, beyond it from the
15-minute rollups (30 days by default).

Host: cpu, load1, mem, mem_used_mb, swap, disk, net_rx, net_tx
App:  cpu, mem_mb, procs, fpm_procs, fpm_max, fpm_saturated, req, http_5xx,
      http_ms, http_ok, http_code, workers_up, workers_total`,
	RunE: func(cmd *cobra.Command, args []string) error {
		app, _ := cmd.Flags().GetString("app")
		metrics, _ := cmd.Flags().GetString("metrics")
		rng, _ := cmd.Flags().GetString("range")

		q := url.Values{}
		if metrics != "" {
			q.Set("metrics", metrics)
		}
		if rng != "" {
			q.Set("range", rng)
		}
		path := "/api/metrics/series"
		if app != "" {
			path = appAPIPath(app, "/metrics")
		}
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data struct {
				Scope   string                     `json:"scope"`
				Metrics []string                   `json:"metrics"`
				Step    int                        `json:"step"`
				Source  string                     `json:"source"`
				Series  map[string][][]interface{} `json:"series"`
			} `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read the series: %s", featureErrorHint(err, metricsMinAPI, "metrics-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		label := strings.TrimPrefix(result.Data.Scope, "app:")
		if rng == "" {
			rng = "24h"
		}
		output.Header(fmt.Sprintf("%s — last %s (step %s, %s data)", label, rng, (time.Duration(result.Data.Step) * time.Second).String(), result.Data.Source))
		t := output.NewTable("METRIC", "MIN", "AVG", "MAX", "TREND")
		for _, m := range result.Data.Metrics {
			pts := result.Data.Series[m]
			if len(pts) == 0 {
				t.Row(m, "—", "—", "—", "")
				continue
			}
			avgs := make([]float64, 0, len(pts))
			mx := math.Inf(-1)
			for _, p := range pts {
				if len(p) >= 3 {
					avgs = append(avgs, metFloat(p[1]))
					mx = math.Max(mx, metFloat(p[2]))
				}
			}
			mn, sum := math.Inf(1), 0.0
			for _, a := range avgs {
				mn = math.Min(mn, a)
				sum += a
			}
			tail := avgs
			if len(tail) > 60 {
				tail = tail[len(tail)-60:]
			}
			t.Row(m, metFmt(mn, 2), metFmt(sum/float64(len(avgs)), 2), metFmt(mx, 2), metSpark(tail))
		}
		t.Flush()
		return nil
	},
}

var metricsUptimeCmd = &cobra.Command{
	Use:   "uptime [app]",
	Short: "Healthcheck uptime, minutes down, incidents, per day",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rng, _ := cmd.Flags().GetString("range")
		q := url.Values{}
		if len(args) == 1 {
			q.Set("app", args[0])
		}
		if rng != "" {
			q.Set("range", rng)
		}
		path := "/api/metrics/uptime"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read uptime: %s", featureErrorHint(err, metricsMinAPI, "metrics-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		rows := []interface{}{result.Data}
		if apps, ok := result.Data["apps"].([]interface{}); ok {
			rows = apps
		}
		if rng == "" {
			rng = "30d"
		}
		output.Header("Uptime — last " + rng)
		if len(rows) == 0 {
			output.Info("No app has a healthcheck (cipi health set <app> on the server)")
			return nil
		}
		t := output.NewTable("APP", "UPTIME", "DOWN", "AVG", "INCIDENTS", "DAYS")
		for _, r := range rows {
			m, _ := r.(map[string]interface{})
			days, _ := m["days"].([]interface{})
			dv := make([]float64, 0, len(days))
			for _, d := range days {
				if p, ok := d.([]interface{}); ok && len(p) == 2 && p[1] != nil {
					dv = append(dv, metFloat(p[1]))
				}
			}
			avg := "—"
			if m["avg_ms"] != nil {
				avg = metNum(m["avg_ms"], 0) + " ms"
			}
			t.Row(metStr(m["app"]), metPct(m["uptime"]), metNum(m["down_minutes"], 0)+"m", avg, metNum(m["incidents"], 0), metSpark(dv))
		}
		t.Flush()
		return nil
	},
}

var metricsEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Monitor state changes, health incidents, deploys, heartbeat",
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		for _, f := range []string{"app", "source", "range"} {
			if v, _ := cmd.Flags().GetString(f); v != "" {
				q.Set(f, v)
			}
		}
		if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		path := "/api/metrics/events"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data struct {
				Events []interface{} `json:"events"`
			} `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read events: %s", featureErrorHint(err, metricsMinAPI, "metrics-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Header("Events")
		if len(result.Data.Events) == 0 {
			fmt.Println("  none")
			return nil
		}
		metPrintEvents(result.Data.Events, 0)
		return nil
	},
}

var metricsSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Collector, database, retention, probe, heartbeat, OpenMetrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/metrics/settings", &result); err != nil {
			output.Error("Failed to read metrics settings: %s", featureErrorHint(err, metricsMinAPI, "metrics-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Header("Metrics settings")
		printDataWrapper(result.Data)
		return nil
	},
}

var metricsRetentionCmd = &cobra.Command{
	Use:   "retention",
	Short: "Change how long the history is kept",
	Long: `  --raw         per-minute samples, days (1–90, default 7)
  --rollup      rollups, days (up to 400, not shorter than --raw; default 30)
  --resolution  rollup resolution in minutes: 5, 10, 15, 30 or 60 (default 15)
  --events      events, days (7–730, default 90)

A shorter retention deletes older data at the next hourly prune.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]interface{}{}
		for flag, key := range map[string]string{"raw": "raw_days", "rollup": "rollup_days", "resolution": "resolution_minutes", "events": "events_days"} {
			if cmd.Flags().Changed(flag) {
				v, _ := cmd.Flags().GetInt(flag)
				body[key] = v
			}
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to change: --raw, --rollup, --resolution, --events")
		}
		return metricsPutSettings(body)
	},
}

var metricsProbeCmd = &cobra.Command{
	Use:       "probe on|off",
	Short:     "Probe each app's healthcheck URL every minute (default on)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "on" && args[0] != "off" {
			return fmt.Errorf("on or off")
		}
		return metricsPutSettings(map[string]interface{}{"probe": args[0] == "on"})
	},
}

var metricsCollectorCmd = &cobra.Command{
	Use:       "collector on|off",
	Short:     "Turn the collector on or off (the history is kept)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "on" && args[0] != "off" {
			return fmt.Errorf("on or off")
		}
		return metricsPutSettings(map[string]interface{}{"enabled": args[0] == "on"})
	},
}

func metricsPutSettings(body map[string]interface{}) error {
	client, err := mustClient()
	if err != nil {
		return err
	}
	var result struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := client.Put("/api/metrics/settings", body, &result); err != nil {
		output.Error("Failed to change metrics settings: %s", featureErrorHint(err, metricsMinAPI, "metrics-manage"))
		return err
	}
	if jsonFlag {
		output.PrintJSON(result)
		return nil
	}
	r, _ := result.Data["retention"].(map[string]interface{})
	output.Success("Metrics: collector %s, probe %s — raw %sd · %s-minute rollups %sd · events %sd",
		metOnOff(result.Data["enabled"]), metOnOff(result.Data["probe"]),
		metNum(r["raw_days"], 0), metNum(r["resolution_minutes"], 0), metNum(r["rollup_days"], 0), metNum(r["events_days"], 0))
	return nil
}

var metricsHeartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Call a URL every N minutes so an outside service alerts when the server stops",
}

var metricsHeartbeatSetCmd = &cobra.Command{
	Use:   "set <url>",
	Short: "Set the heartbeat URL (healthchecks.io, Uptime Kuma push, …)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		every, _ := cmd.Flags().GetInt("every")
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put("/api/metrics/heartbeat", map[string]interface{}{"url": args[0], "every": every}, &result); err != nil {
			output.Error("Failed to set the heartbeat: %s", featureErrorHint(err, metricsMinAPI, "metrics-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Heartbeat: every %d min → %s", every, metStr(result.Data["url_host"]))
		output.Info("Set the receiver to expect a call every %d min (plus a grace period).", every)
		return nil
	},
}

var metricsHeartbeatOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Stop the heartbeat",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		if err := client.Delete("/api/metrics/heartbeat", nil, nil); err != nil {
			output.Error("Failed to stop the heartbeat: %s", featureErrorHint(err, metricsMinAPI, "metrics-manage"))
			return err
		}
		output.Success("Heartbeat off")
		return nil
	},
}

var metricsHeartbeatTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Call the heartbeat URL once now",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post("/api/metrics/heartbeat/test", nil, &result); err != nil {
			output.Error("Heartbeat test failed: %s", featureErrorHint(err, metricsMinAPI, "metrics-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		if ok, _ := result.Data["ok"].(bool); ok {
			output.Success("HTTP %s ← %s", metNum(result.Data["code"], 0), metStr(result.Data["host"]))
			return nil
		}
		output.Error("HTTP %s ← %s", metNum(result.Data["code"], 0), metStr(result.Data["host"]))
		return fmt.Errorf("heartbeat call failed")
	},
}

// ── cipi-cli apps basicauth agent ─────────────────────────────

var appsBasicAuthAgentCmd = &cobra.Command{
	Use:   "agent <name> [public|protected]",
	Short: "cipi/agent routes left out of basic auth, or not (Cipi 5.11.0+)",
	Long: `While basic auth is on and the live release has cipi/agent, its routes
(/cipi/ or CIPI_ROUTE_PREFIX) are left out of it: webhook, health, MCP and the
anonymizer check their own tokens, and a Git provider cannot send the site's
password. "protected" keeps them behind basic auth too.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if len(args) == 2 {
			if args[1] != "public" && args[1] != "protected" {
				return fmt.Errorf("public or protected")
			}
			err = client.Put(appAPIPath(args[0], "/basicauth/agent"), map[string]string{"mode": args[1]}, &result)
		} else {
			err = client.Get(appAPIPath(args[0], "/basicauth/agent"), &result)
		}
		if err != nil {
			output.Error("cipi/agent and basic auth: %s", featureErrorHint(err, metricsMinAPI, "apps-basicauth"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		switch {
		case result.Data["public"] == true:
			output.Success("%s: cipi/agent routes (%s) are reachable without basic auth", args[0], metStr(result.Data["prefix"]))
		case result.Data["mode"] == "protected":
			output.Info("%s: cipi/agent routes stay behind basic auth", args[0])
		default:
			output.Info("%s: nothing left out (basic auth off, or no cipi/agent in the live release)", args[0])
		}
		return nil
	},
}

// ── formatting helpers ────────────────────────────────────────

func metPrintEvents(events []interface{}, limit int) {
	t := output.NewTable("WHEN", "SOURCE", "SUBJECT", "STATUS", "SUMMARY")
	for i, e := range events {
		if limit > 0 && i >= limit {
			break
		}
		m, _ := e.(map[string]interface{})
		when := time.Unix(int64(metFloat(m["ts"])), 0).Format("01-02 15:04")
		status := metStr(m["status"])
		switch status {
		case "ok":
			status = output.Green.Sprint(status)
		case "warn":
			status = output.Yellow.Sprint(status)
		default:
			status = output.Red.Sprint(status)
		}
		t.Row(when, metStr(m["source"]), metStr(m["subject"]), status, metStr(m["summary"]))
	}
	t.Flush()
}

func metSpark(vals []float64) string {
	if len(vals) == 0 {
		return ""
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	mn, mx := vals[0], vals[0]
	for _, v := range vals {
		mn = math.Min(mn, v)
		mx = math.Max(mx, v)
	}
	var b strings.Builder
	for _, v := range vals {
		k := 0
		if mx > mn {
			k = int((v - mn) / (mx - mn) * 7)
		}
		b.WriteRune(bars[k])
	}
	return b.String()
}

func metFloats(v interface{}) []float64 {
	arr, _ := v.([]interface{})
	out := make([]float64, 0, len(arr))
	for _, x := range arr {
		if x != nil {
			out = append(out, metFloat(x))
		}
	}
	return out
}

func metFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func metNum(v interface{}, decimals int) string {
	if v == nil {
		return "—"
	}
	return metFmt(metFloat(v), decimals)
}

func metFmt(f float64, decimals int) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "—"
	}
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

func metPct(v interface{}) string {
	if v == nil {
		return "—"
	}
	f := metFloat(v)
	s := metFmt(f, 2) + "%"
	switch {
	case f < 99:
		return output.Red.Sprint(s)
	case f < 99.9:
		return output.Yellow.Sprint(s)
	}
	return output.Green.Sprint(s)
}

func metRate(v interface{}) string {
	if v == nil {
		return "—"
	}
	f := metFloat(v)
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return metFmt(f, 1) + " " + units[i]
}

func metStr(v interface{}) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprint(v)
}

func metOnOff(v interface{}) string {
	if b, ok := v.(bool); ok && b {
		return "on"
	}
	return "off"
}

func init() {
	metricsShowCmd.Flags().String("app", "", "App name (default: the host)")
	metricsShowCmd.Flags().String("metrics", "", "Comma-separated metric names (default: all of the scope)")
	metricsShowCmd.Flags().String("range", "", "How far back: 90m, 24h, 7d, 30d (default 24h)")
	metricsUptimeCmd.Flags().String("range", "", "Window: 24h, 7d, 30d (default 30d)")
	metricsEventsCmd.Flags().String("app", "", "App name")
	metricsEventsCmd.Flags().String("source", "", "monitor, health, deploy or heartbeat")
	metricsEventsCmd.Flags().String("range", "", "How far back (default 7d)")
	metricsEventsCmd.Flags().Int("limit", 0, "At most this many (1–1000, default 100)")
	metricsRetentionCmd.Flags().Int("raw", 7, "Per-minute samples, days (1–90)")
	metricsRetentionCmd.Flags().Int("rollup", 30, "Rollups, days (up to 400)")
	metricsRetentionCmd.Flags().Int("resolution", 15, "Rollup resolution, minutes: 5, 10, 15, 30, 60")
	metricsRetentionCmd.Flags().Int("events", 90, "Events, days (7–730)")
	metricsHeartbeatSetCmd.Flags().Int("every", 1, "Minutes between calls (1–60)")

	metricsHeartbeatCmd.AddCommand(metricsHeartbeatSetCmd, metricsHeartbeatOffCmd, metricsHeartbeatTestCmd)
	metricsCmd.AddCommand(metricsShowCmd, metricsUptimeCmd, metricsEventsCmd, metricsSettingsCmd,
		metricsRetentionCmd, metricsProbeCmd, metricsCollectorCmd, metricsHeartbeatCmd)
	rootCmd.AddCommand(metricsCmd)

	appsBasicAuthCmd.AddCommand(appsBasicAuthAgentCmd)
}
