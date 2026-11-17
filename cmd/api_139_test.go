package cmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Tests for the commands added with cipi/api 1.37 – 1.39.1: deploy status and
// history, Node runtimes, backups, databases linked to an app.

func plainOutput(t *testing.T) {
	t.Helper()
	old := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = old })
}

func TestFeatureErrorHint(t *testing.T) {
	const upgrade = "update cipi/api to 1.39.0+"
	const ability = "lacks the backups-manage ability"

	cases := []struct {
		name string
		err  error
		want string // substring expected after the error itself, "" = the error as it is
	}{
		{"403 names the ability", &api.APIError{Message: "Invalid ability provided.", Status: 403}, ability},
		{"non-JSON 403", errors.New("API error (HTTP 403): Forbidden"), ability},
		{"missing route on an old cipi/api", &api.APIError{Message: "The route api/backups could not be found.", Status: 404}, upgrade},
		{"non-JSON 404", errors.New("API error (HTTP 404): <html>Not Found</html>"), upgrade},
		{"unknown app is not a version problem", &api.APIError{ErrorText: "App 'nope' not found", Status: 404}, ""},
		{"an old Cipi says what it needs itself", &api.APIError{ErrorText: "Backup management from the panel needs Cipi 5.9.0+ — this server runs Cipi 5.8.4. Run: cipi self-update", Status: 503}, ""},
		{"the CLI refusing a request", &api.APIError{ErrorText: "Profile 'daily' already exists", Status: 422}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := featureErrorHint(tc.err, "1.39.0", "backups-manage")
			if !strings.HasPrefix(got, tc.err.Error()) {
				t.Fatalf("hint %q does not start with the error %q", got, tc.err.Error())
			}
			if tc.want == "" && got != tc.err.Error() {
				t.Fatalf("hint %q, want the error as it is", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("hint %q does not contain %q", got, tc.want)
			}
		})
	}
}

func TestValidateNodeMajor(t *testing.T) {
	for _, major := range []string{"20", "22", "24", "98"} {
		if err := validateNodeMajor(major, false); err != nil {
			t.Errorf("%q: unexpected error %v", major, err)
		}
	}
	for _, major := range []string{"", "21", "8", "18", "100", "22.1", "v22", "lts", "system", "default"} {
		if err := validateNodeMajor(major, false); err == nil {
			t.Errorf("%q: expected an error", major)
		}
	}

	if err := validateNodeMajor("system", true); err != nil {
		t.Errorf("system as the server default: unexpected error %v", err)
	}
	if err := validateNodeMajor("23", true); err == nil || !strings.Contains(err.Error(), "or system") {
		t.Errorf("23 as the server default: error %v does not offer system", err)
	}
}

func TestFormatSeconds(t *testing.T) {
	cases := map[int64]string{0: "0s", 45: "45s", 95: "1m35s", 3723: "1h2m3s", -1: "—"}
	for seconds, want := range cases {
		if got := formatSeconds(seconds); got != want {
			t.Errorf("formatSeconds(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestDeployRun(t *testing.T) {
	plainOutput(t)

	cases := []struct {
		name         string
		run          map[string]interface{}
		wantResult   string
		wantDuration string
	}{
		{"ok", map[string]interface{}{"result": "ok", "duration": float64(95), "exit": float64(0)}, "ok", "1m35s"},
		{"failed with its exit code", map[string]interface{}{"result": "failed", "duration": float64(12), "exit": float64(2)}, "failed (exit 2)", "12s"},
		{"failed without an exit code", map[string]interface{}{"result": "failed", "duration": nil}, "failed", "—"},
		{"nothing known", map[string]interface{}{}, "—", "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deployRunResult(tc.run); got != tc.wantResult {
				t.Errorf("result = %q, want %q", got, tc.wantResult)
			}
			if got := deployRunDuration(tc.run); got != tc.wantDuration {
				t.Errorf("duration = %q, want %q", got, tc.wantDuration)
			}
		})
	}
}

func TestBackupProfileBody(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]interface{}
	}{
		{"nothing passed, nothing sent", nil, map[string]interface{}{}},
		{
			name: "a new profile",
			args: []string{"--scope", "db", "--databases", "shop,shop_*", "--dest", "local", "--dest", "s3", "--every", "6h", "--keep", "7", "--encrypt"},
			want: map[string]interface{}{
				"scope":        "db",
				"databases":    []string{"shop", "shop_*"},
				"destinations": []string{"local", "s3"},
				"every":        "6h",
				"keep":         7,
				"encrypt":      true,
			},
		},
		{
			name: "an empty exclusion is sent, to clear it",
			args: []string{"--exclude-tables", "", "--encrypt=false", "--keep-days", "0"},
			want: map[string]interface{}{
				"exclude_tables": []string{},
				"encrypt":        false,
				"keep_days":      0,
			},
		},
		{
			name: "cron and retention",
			args: []string{"--cron", "0 2 * * *", "--keep", "14", "--keep-weeks", "8", "--apps", "shop", "--exclude-databases", "tmp_*"},
			want: map[string]interface{}{
				"cron":              "0 2 * * *",
				"keep":              14,
				"keep_weeks":        8,
				"apps":              []string{"shop"},
				"exclude_databases": []string{"tmp_*"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "edit"}
			addBackupProfileFlags(cmd)
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatalf("parsing %v: %v", tc.args, err)
			}
			if got := backupProfileBody(cmd); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("body = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBackupSchedule(t *testing.T) {
	cron := "0 2 * * *"
	secs := func(n int64) *int64 { return &n }

	cases := []struct {
		name     string
		cron     *string
		interval *int64
		want     string
	}{
		{"cron wins", &cron, secs(3600), "0 2 * * *"},
		{"days", nil, secs(86400), "every 1d"},
		{"hours", nil, secs(21600), "every 6h"},
		{"minutes", nil, secs(1800), "every 30m"},
		{"an interval that is not whole minutes", nil, secs(90), "every 90s"},
		{"none", nil, nil, "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backupSchedule(tc.cron, tc.interval); got != tc.want {
				t.Fatalf("backupSchedule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBackupRetention(t *testing.T) {
	cases := []struct {
		keep, days, weeks int
		want              string
	}{
		{7, 0, 0, "7 runs"},
		{14, 30, 8, "14 runs, 30 days, 8 weeks"},
		{0, 0, 4, "4 weeks"},
		{0, 0, 0, "—"},
	}
	for _, tc := range cases {
		if got := backupRetention(tc.keep, tc.days, tc.weeks); got != tc.want {
			t.Errorf("backupRetention(%d, %d, %d) = %q, want %q", tc.keep, tc.days, tc.weeks, got, tc.want)
		}
	}
}

func TestBackupProfileState(t *testing.T) {
	plainOutput(t)

	cases := []struct {
		name string
		p    backupProfile
		want string
	}{
		{"running wins over everything", backupProfile{Enabled: true, Overdue: true, Running: true}, "running"},
		{"disabled is not overdue", backupProfile{Enabled: false, Overdue: true}, "disabled"},
		{"overdue", backupProfile{Enabled: true, Overdue: true}, "overdue"},
		{"enabled", backupProfile{Enabled: true}, "enabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backupProfileState(&tc.p); got != tc.want {
				t.Fatalf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDbListTable(t *testing.T) {
	t.Run("an older core: name and size", func(t *testing.T) {
		headers, rows := dbListTable([]map[string]interface{}{
			{"name": "shop", "size": "12.34 MB"},
		})
		if !reflect.DeepEqual(headers, []string{"NAME", "SIZE"}) {
			t.Fatalf("headers = %v", headers)
		}
		if !reflect.DeepEqual(rows, [][]string{{"shop", "12.34 MB"}}) {
			t.Fatalf("rows = %v", rows)
		}
	})

	t.Run("Cipi 5.9.0+: engine, size_mb and the app", func(t *testing.T) {
		headers, rows := dbListTable([]map[string]interface{}{
			{"name": "shop", "engine": "mariadb", "size_mb": 12.34, "app": "shop", "link": "own"},
			{"name": "shop_reports", "engine": "pgsql", "size_mb": 0.04, "app": "shop", "link": "linked"},
			{"name": "legacy", "engine": "mariadb", "size_mb": float64(3), "app": nil, "link": nil},
		})
		if !reflect.DeepEqual(headers, []string{"NAME", "ENGINE", "SIZE", "APP"}) {
			t.Fatalf("headers = %v", headers)
		}
		want := [][]string{
			{"shop", "mariadb", "12.3 MB", "shop (own)"},
			{"shop_reports", "pgsql", "<0.1 MB", "shop (linked)"},
			{"legacy", "mariadb", "3.0 MB", "—"},
		}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %v, want %v", rows, want)
		}
	})
}

func TestAppDatabases(t *testing.T) {
	cases := []struct {
		name string
		app  map[string]interface{}
		want []string
	}{
		{"an older server sends none", map[string]interface{}{"app": "shop"}, nil},
		{"empty", map[string]interface{}{"databases": []interface{}{}}, nil},
		{"names", map[string]interface{}{"databases": []interface{}{"shop", "shop_reports"}}, []string{"shop", "shop_reports"}},
		{
			name: "objects with an engine",
			app: map[string]interface{}{"databases": []interface{}{
				map[string]interface{}{"name": "shop", "engine": "mariadb"},
				map[string]interface{}{"name": "shop_reports"},
			}},
			want: []string{"shop (mariadb)", "shop_reports"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appDatabases(tc.app); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("appDatabases = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCLIOutputLines(t *testing.T) {
	got := cliOutputLines("\x1b[32m[OK]\x1b[0m daily: 4 archives open\r\n\n   \n  s3: reachable  \n")
	want := []string{"[OK] daily: 4 archives open", "  s3: reachable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cliOutputLines = %q, want %q", got, want)
	}
}
