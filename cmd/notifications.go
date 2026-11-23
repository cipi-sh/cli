package cmd

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Notifications — every channel and trigger. cipi/api 1.42.0, Cipi 5.12.0.
// All synchronous; secrets are never returned (token_set, api_key_set, …).

const notificationsMinAPI = "1.42.0"

var notifTypes = []string{"smtp", "resend", "ses", "slack", "discord", "mattermost", "googlechat", "telegram", "ntfy", "pushover", "webhook"}

// The API's field name for every flag of add/update.
var notifStringFlags = map[string]string{
	"host": "host", "user": "user", "password": "password", "from": "from", "to": "to",
	"url": "url", "priority": "priority", "secret": "secret", "token": "token",
	"chat-id": "chat_id", "api-key": "api_key", "region": "region",
	"access-key": "access_key", "secret-key": "secret_key",
}

type notifResponse struct {
	Data  map[string]interface{} `json:"data"`
	Error string                 `json:"error,omitempty"`
}

var notificationsCmd = &cobra.Command{
	Use:     "notifications",
	Aliases: []string{"notification", "notify"},
	Short:   "Notification channels and triggers: SMTP, Slack, Telegram, webhook, …",
	Long: `Where the server's notifications go, and which events send them (Cipi 5.12.0+,
cipi/api 1.42.0+, abilities notifications-view / notifications-manage — or the
older smtp-view / smtp-manage). Every enabled trigger reaches every enabled channel.

Channel types and their flags:
  smtp        --host --port --user --password --from --to --tls
  resend      --api-key --from --to
  ses         --region --access-key --secret-key --from --to
  slack, discord, mattermost, googlechat   --url
  telegram    --token --chat-id
  ntfy        --url --priority
  pushover    --token --user
  webhook     --url --secret

  cipi-cli notifications
  cipi-cli notifications add slack ops --url https://hooks.slack.com/services/… --test
  cipi-cli notifications update oncall --chat-id -1009999   (secrets left out are kept)
  cipi-cli notifications test all
  cipi-cli notifications triggers
  cipi-cli notifications triggers off deploy_success ssh_login

` + multiServerTip,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Get("/api/notifications", &result); err != nil {
			output.Error("Failed to read notifications: %s", featureErrorHint(err, notificationsMinAPI, "notifications-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printNotifChannels(result.Data)
		return nil
	},
}

func notifTarget(ch map[string]interface{}) string {
	switch str(ch, "type") {
	case "smtp":
		return fmt.Sprintf("%s:%s  %s → %s", str(ch, "host"), str(ch, "port"), str(ch, "from"), notifList(ch["to"]))
	case "resend":
		return fmt.Sprintf("%s → %s", str(ch, "from"), notifList(ch["to"]))
	case "ses":
		return fmt.Sprintf("%s: %s → %s", str(ch, "region"), str(ch, "from"), notifList(ch["to"]))
	case "telegram":
		return "chat " + str(ch, "chat_id")
	case "pushover":
		return "user " + str(ch, "user")
	case "webhook":
		t := str(ch, "url")
		if b, _ := ch["secret_set"].(bool); b {
			t += "  signed"
		}
		return t
	case "ntfy":
		t := str(ch, "url")
		if p := str(ch, "priority"); p != "" && p != "default" {
			t += "  priority=" + p
		}
		return t
	}
	return str(ch, "url")
}

func notifList(v interface{}) string {
	switch x := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, ", ")
	case string:
		return x
	}
	return ""
}

func printNotifChannels(data map[string]interface{}) {
	output.Header("Notification channels")
	chans, _ := data["channels"].([]interface{})
	if len(chans) == 0 {
		output.Warn("No channels — add one: cipi-cli notifications add slack ops --url …")
	} else {
		t := output.NewTable("", "ID", "TYPE", "TARGET")
		for _, c := range chans {
			ch, _ := c.(map[string]interface{})
			dot := output.Dim.Sprint("○")
			if b, _ := ch["enabled"].(bool); b {
				dot = output.Green.Sprint("●")
			}
			t.Row(dot, str(ch, "id"), str(ch, "type"), notifTarget(ch))
		}
		t.Flush()
	}
	if tr, ok := data["triggers"].(map[string]interface{}); ok {
		fmt.Println()
		output.KeyValue(nil, "Triggers on", fmt.Sprintf("%v / %v", tr["enabled"], tr["total"]))
	}
	fmt.Println()
}

// notifBody collects the flags that were set into the API's fields.
func notifBody(cmd *cobra.Command) map[string]interface{} {
	body := map[string]interface{}{}
	for flag, field := range notifStringFlags {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetString(flag)
			body[field] = v
		}
	}
	if cmd.Flags().Changed("port") {
		v, _ := cmd.Flags().GetInt("port")
		body["port"] = v
	}
	if cmd.Flags().Changed("tls") {
		v, _ := cmd.Flags().GetBool("tls")
		body["tls"] = v
	}
	if v, _ := cmd.Flags().GetBool("test"); v {
		body["test"] = true
	}
	return body
}

func addNotifFlags(c *cobra.Command) {
	for flag := range notifStringFlags {
		c.Flags().String(flag, "", "")
	}
	c.Flags().Int("port", 587, "SMTP port")
	c.Flags().Bool("tls", true, "SMTP: TLS")
	c.Flags().Bool("test", false, "Send a test after saving")
	usage := map[string]string{
		"host": "SMTP host", "user": "SMTP user / Pushover user key", "password": "SMTP password",
		"from": "Sender (smtp, resend, ses)", "to": "Recipients, comma-separated (smtp, resend, ses)",
		"url": "Webhook / ntfy URL", "priority": "ntfy: min|low|default|high|urgent", "secret": "Webhook signing secret",
		"token": "Telegram bot token / Pushover app token", "chat-id": "Telegram chat id", "api-key": "Resend API key (re_…)",
		"region": "SES region", "access-key": "SES access key id", "secret-key": "SES secret access key",
	}
	for flag, u := range usage {
		c.Flags().Lookup(flag).Usage = u
	}
}

func printNotifSaved(verb string, data map[string]interface{}) {
	ch, _ := data["channel"].(map[string]interface{})
	if ch != nil {
		output.Success("%s '%s' (%s): %s", verb, str(ch, "id"), str(ch, "type"), notifTarget(ch))
	} else {
		output.Success("%s", verb)
	}
	if t, ok := data["test"].(map[string]interface{}); ok {
		if b, _ := t["ok"].(bool); b {
			output.Success("Test: %s", str(t, "message"))
		} else {
			output.Warn("Test failed: %s", str(t, "message"))
		}
	}
	fmt.Println()
}

var notifAddCmd = &cobra.Command{
	Use:   "add <type> [id]",
	Short: "Add a channel (smtp takes no id)",
	Example: `  cipi-cli notifications add slack ops --url https://hooks.slack.com/services/… --test
  cipi-cli notifications add telegram oncall --token 123456:ABC… --chat-id -1001234
  cipi-cli notifications add webhook siem --url https://example.com/hooks/cipi --secret '…'
  cipi-cli notifications add smtp --host smtp.example.com --user me --password '…' --from cipi@example.com --to ops@example.com`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		typ := strings.ToLower(args[0])
		known := false
		for _, t := range notifTypes {
			known = known || t == typ
		}
		if !known {
			return fmt.Errorf("unknown type %q (%s)", typ, strings.Join(notifTypes, ", "))
		}
		body := notifBody(cmd)
		body["type"] = typ
		if typ != "smtp" {
			if len(args) < 2 {
				return fmt.Errorf("an id is required: cipi-cli notifications add %s <id> …", typ)
			}
			body["id"] = args[1]
		}
		if v, _ := cmd.Flags().GetBool("disabled"); v {
			body["enabled"] = false
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Post("/api/notifications/channels", body, &result); err != nil {
			output.Error("Failed to add the channel: %s", featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printNotifSaved("Added", result.Data)
		return nil
	},
}

var notifUpdateCmd = &cobra.Command{
	Use:     "update <id>",
	Aliases: []string{"edit", "set"},
	Short:   "Change a channel — flags left out (secrets too) keep their value",
	Example: `  cipi-cli notifications update oncall --chat-id -1009999
  cipi-cli notifications update mail --to ops@example.com,dev@example.com --test
  cipi-cli notifications update siem --clear-secret
  cipi-cli notifications update smtp --to oncall@example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := notifBody(cmd)
		if v, _ := cmd.Flags().GetBool("clear-secret"); v {
			body["clear_secret"] = true
		}
		if v, _ := cmd.Flags().GetBool("enable"); v {
			body["enabled"] = true
		}
		if v, _ := cmd.Flags().GetBool("disable"); v {
			body["enabled"] = false
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to change: pass the flags to change (cipi-cli notifications update --help)")
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Put("/api/notifications/channels/"+url.PathEscape(args[0]), body, &result); err != nil {
			output.Error("Failed to update '%s': %s", args[0], featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printNotifSaved("Updated", result.Data)
		return nil
	},
}

var notifRemoveCmd = &cobra.Command{
	Use:     "remove <id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a channel",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if yes, _ := cmd.Flags().GetBool("yes"); !yes {
			if !output.Confirm(fmt.Sprintf("Remove the channel '%s'?", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Delete("/api/notifications/channels/"+url.PathEscape(args[0]), nil, &result); err != nil {
			output.Error("Failed to remove '%s': %s", args[0], featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Channel '%s' removed", args[0])
		fmt.Println()
		return nil
	},
}

func newNotifToggleCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <id>",
		Short: strings.ToUpper(action[:1]) + action[1:] + " a channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := mustClient()
			if err != nil {
				return err
			}
			var result notifResponse
			if err := client.Post("/api/notifications/channels/"+url.PathEscape(args[0])+"/"+action, nil, &result); err != nil {
				output.Error("Failed to %s '%s': %s", action, args[0], featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
				return err
			}
			if jsonFlag {
				output.PrintJSON(result)
				return nil
			}
			output.Success("Channel '%s' %sd", args[0], action)
			fmt.Println()
			return nil
		},
	}
}

var notifTestCmd = &cobra.Command{
	Use:   "test [id|all]",
	Short: "Send a test (exits 1 when the channel refuses it)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := "all"
		if len(args) == 1 {
			id = args[0]
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Post("/api/notifications/channels/"+url.PathEscape(id)+"/test", nil, &result); err != nil {
			output.Error("Test to '%s' failed: %s", id, featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("%s", str(result.Data, "message"))
		fmt.Println()
		return nil
	},
}

var notifTriggersCmd = &cobra.Command{
	Use:   "triggers",
	Short: "Which events notify — list, on, off, all-on, all-off, reset",
	Example: `  cipi-cli notifications triggers
  cipi-cli notifications triggers off deploy_success ssh_login
  cipi-cli notifications triggers on deploy_success
  cipi-cli notifications triggers all-off --except deploy_fail,backup_fail,health_fail
  cipi-cli notifications triggers reset`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result notifResponse
		if err := client.Get("/api/notifications/triggers", &result); err != nil {
			output.Error("Failed to read the triggers: %s", featureErrorHint(err, notificationsMinAPI, "notifications-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printNotifTriggers(result.Data)
		return nil
	},
}

// notifGroupTriggers keeps the catalog order inside each category, categories in first-seen order.
func notifGroupTriggers(list []interface{}) ([]string, map[string][]map[string]interface{}) {
	var order []string
	groups := map[string][]map[string]interface{}{}
	for _, it := range list {
		t, _ := it.(map[string]interface{})
		cat := str(t, "category")
		if _, ok := groups[cat]; !ok {
			order = append(order, cat)
		}
		groups[cat] = append(groups[cat], t)
	}
	return order, groups
}

func printNotifTriggers(data map[string]interface{}) {
	list, _ := data["triggers"].([]interface{})
	output.Header(fmt.Sprintf("Notification triggers  %v/%v on", data["enabled"], data["total"]))
	order, groups := notifGroupTriggers(list)
	for _, cat := range order {
		fmt.Printf("  %s\n", output.Bold.Sprint(cat))
		for _, t := range groups[cat] {
			dot := output.Dim.Sprint("○")
			if b, _ := t["enabled"].(bool); b {
				dot = output.Green.Sprint("●")
			}
			fmt.Printf("    %s %-24s %s\n", dot, str(t, "id"), output.Dim.Sprint(str(t, "label")))
		}
	}
	fmt.Println()
}

func notifTriggersPut(body map[string]interface{}, path string) error {
	client, err := mustClient()
	if err != nil {
		return err
	}
	var result notifResponse
	if path == "" {
		err = client.Put("/api/notifications/triggers", body, &result)
	} else {
		err = client.Post(path, nil, &result)
	}
	if err != nil {
		output.Error("Failed to change the triggers: %s", featureErrorHint(err, notificationsMinAPI, "notifications-manage"))
		return err
	}
	if jsonFlag {
		output.PrintJSON(result)
		return nil
	}
	var off []string
	if list, ok := result.Data["triggers"].([]interface{}); ok {
		for _, it := range list {
			t, _ := it.(map[string]interface{})
			if b, _ := t["enabled"].(bool); !b {
				off = append(off, str(t, "id"))
			}
		}
	}
	sort.Strings(off)
	output.Success("Triggers on: %v/%v", result.Data["enabled"], result.Data["total"])
	if len(off) > 0 && len(off) <= 20 {
		output.Dim.Printf("  off: %s\n", strings.Join(off, ", "))
	}
	fmt.Println()
	return nil
}

func newNotifTriggerSetCmd(state string) *cobra.Command {
	return &cobra.Command{
		Use:   state + " <trigger>…",
		Short: "Turn triggers " + state,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ids []string
			for _, a := range args {
				for _, p := range strings.Split(a, ",") {
					if p = strings.TrimSpace(p); p != "" {
						ids = append(ids, p)
					}
				}
			}
			return notifTriggersPut(map[string]interface{}{state: ids}, "")
		},
	}
}

func newNotifTriggerAllCmd(on bool) *cobra.Command {
	name := "all-off"
	if on {
		name = "all-on"
	}
	c := &cobra.Command{
		Use:   name,
		Short: "Every trigger " + strings.TrimPrefix(name, "all-") + " (--except: the ones set the other way)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{"all": on}
			if ex, _ := cmd.Flags().GetStringSlice("except"); len(ex) > 0 {
				key := "on"
				if on {
					key = "off"
				}
				body[key] = ex
			}
			return notifTriggersPut(body, "")
		},
	}
	c.Flags().StringSlice("except", nil, "Triggers set the other way, comma-separated")
	return c
}

var notifTriggersResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Every trigger back on (the default)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return notifTriggersPut(nil, "/api/notifications/triggers/reset")
	},
}

func init() {
	addNotifFlags(notifAddCmd)
	notifAddCmd.Flags().Bool("disabled", false, "Add it turned off")
	addNotifFlags(notifUpdateCmd)
	notifUpdateCmd.Flags().Bool("clear-secret", false, "Webhook: drop the signing secret")
	notifUpdateCmd.Flags().Bool("enable", false, "Turn the channel on")
	notifUpdateCmd.Flags().Bool("disable", false, "Turn the channel off")
	notifRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	notifTriggersCmd.AddCommand(
		newNotifTriggerSetCmd("on"),
		newNotifTriggerSetCmd("off"),
		newNotifTriggerAllCmd(true),
		newNotifTriggerAllCmd(false),
		notifTriggersResetCmd,
	)
	notificationsCmd.AddCommand(
		notifAddCmd,
		notifUpdateCmd,
		notifRemoveCmd,
		newNotifToggleCmd("enable"),
		newNotifToggleCmd("disable"),
		notifTestCmd,
		notifTriggersCmd,
	)
	rootCmd.AddCommand(notificationsCmd)
}
