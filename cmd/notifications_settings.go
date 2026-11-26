package cmd

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// How notifications read — the time zone of their Time line, the name they
// give the server — came with Cipi 5.17.0 and cipi/api 1.47.
const notifSettingsMinAPI = "1.47.0"

var (
	notifTimezonePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+){0,2}$`)
	notifServerNamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9 ._-]{0,38}[A-Za-z0-9._-])?$`)
)

// notifSettings is the data of GET /api/notifications/settings.
type notifSettings struct {
	Timezone       *string `json:"timezone"`
	ServerTimezone string  `json:"server_timezone"`
	ServerName     *string `json:"server_name"`
	Hostname       string  `json:"hostname"`
	Now            string  `json:"now"`
}

type notifSettingsResponse struct {
	Data notifSettings `json:"data"`
}

// notifTimezoneBody: "server" (or "system", "default") puts the server's own
// time zone back — null on the wire.
func notifTimezoneBody(arg string) (map[string]interface{}, error) {
	switch arg {
	case "server", "system", "default":
		return map[string]interface{}{"timezone": nil}, nil
	}
	if !notifTimezonePattern.MatchString(arg) {
		return nil, fmt.Errorf("unknown time zone %q — an IANA name such as Europe/Rome, America/New_York or UTC, or \"server\" for the server's own", arg)
	}
	return map[string]interface{}{"timezone": arg}, nil
}

func notifServerNameBody(name string, clear bool) (map[string]interface{}, error) {
	if clear {
		if name != "" {
			return nil, errors.New("give a name or --clear, not both")
		}
		return map[string]interface{}{"server_name": nil}, nil
	}
	if !notifServerNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid name %q — letters, digits, spaces, dots, dashes and underscores, 40 characters at most", name)
	}
	return map[string]interface{}{"server_name": name}, nil
}

func printNotifSettings(s notifSettings) {
	output.Header("Notifications are written")
	if s.Timezone != nil && *s.Timezone != "" {
		output.KeyValue(nil, "Time zone", *s.Timezone+"  (the server is on "+s.ServerTimezone+")")
	} else {
		output.KeyValue(nil, "Time zone", s.ServerTimezone+"  (the server's own)")
	}
	if s.ServerName != nil && *s.ServerName != "" {
		output.KeyValue(nil, "Server name", *s.ServerName+"  (hostname: "+s.Hostname+")")
	} else {
		output.KeyValue(nil, "Server name", s.Hostname+"  (its hostname)")
	}
	output.KeyValue(nil, "Now", s.Now)
	fmt.Println()
}

func notifSettingsGet() (*notifSettingsResponse, error) {
	client, err := mustClient()
	if err != nil {
		return nil, err
	}
	var result notifSettingsResponse
	if err := client.Get("/api/notifications/settings", &result); err != nil {
		output.Error("Failed to read the settings: %s", featureErrorHint(err, notifSettingsMinAPI, "notifications-view"))
		return nil, err
	}
	return &result, nil
}

func notifSettingsPut(body map[string]interface{}) (*notifSettingsResponse, error) {
	client, err := mustClient()
	if err != nil {
		return nil, err
	}
	var result notifSettingsResponse
	if err := client.Put("/api/notifications/settings", body, &result); err != nil {
		output.Error("Not changed: %s", featureErrorHint(err, notifSettingsMinAPI, "notifications-manage"))
		return nil, err
	}
	return &result, nil
}

var notifSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "The time zone and the server name notifications use",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := notifSettingsGet()
		if err != nil {
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printNotifSettings(result.Data)
		return nil
	},
}

var notifTimezoneCmd = &cobra.Command{
	Use:   "timezone [<Area/City> | server]",
	Short: "The time zone of the Time line of every notification",
	Long: `Notifications are written in the server's time zone — UTC on most servers.
This sets another one for the notifications only: the server's clock, cron and
logs do not change.

  cipi-cli notifications timezone                 what it is now
  cipi-cli notifications timezone Europe/Rome
  cipi-cli notifications timezone server          the server's own again`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return notifSettingsCmd.RunE(cmd, args)
		}
		body, err := notifTimezoneBody(args[0])
		if err != nil {
			output.Error("%s", err)
			return err
		}
		result, err := notifSettingsPut(body)
		if err != nil {
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		zone := result.Data.ServerTimezone + " (the server's own)"
		if result.Data.Timezone != nil && *result.Data.Timezone != "" {
			zone = *result.Data.Timezone
		}
		output.Success("Notifications are written in %s — now: %s", zone, result.Data.Now)
		fmt.Println()
		return nil
	},
}

var notifServerNameCmd = &cobra.Command{
	Use:   "server-name [<name> | --clear]",
	Short: "The name notifications give this server",
	Long: `A notification names the server by its hostname (vmi12334). With a name of
your own, subjects carry the name and bodies "name (hostname)" — the same
name the panel shows for it.

  cipi-cli notifications server-name              what it is now
  cipi-cli notifications server-name production
  cipi-cli notifications server-name --clear      the hostname again`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clear, _ := cmd.Flags().GetBool("clear")
		if len(args) == 0 && !clear {
			return notifSettingsCmd.RunE(cmd, args)
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		body, err := notifServerNameBody(name, clear)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		result, err := notifSettingsPut(body)
		if err != nil {
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		if result.Data.ServerName != nil && *result.Data.ServerName != "" {
			output.Success("Notifications name this server %s (%s)", *result.Data.ServerName, result.Data.Hostname)
		} else {
			output.Success("Notifications name this server by its hostname again (%s)", result.Data.Hostname)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	notifServerNameCmd.Flags().Bool("clear", false, "The hostname again")
	notificationsCmd.AddCommand(notifSettingsCmd, notifTimezoneCmd, notifServerNameCmd)
}
