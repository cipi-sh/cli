package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// How the server updates Cipi (cipi/api 1.48, Cipi 5.18.0): the release
// signature check and the update delay — both off unless turned on.

const updatesAPI = "1.48"

var fingerprintRe = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// updatesDelayHours reads --delay: 12h, 2d, 12, off.
func updatesDelayHours(v string) (int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	mult := 1
	switch {
	case v == "off" || v == "none" || v == "0":
		return 0, nil
	case v == "on":
		return 12, nil
	case strings.HasSuffix(v, "d"):
		mult, v = 24, strings.TrimSuffix(v, "d")
	case strings.HasSuffix(v, "h"):
		v = strings.TrimSuffix(v, "h")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n*mult > 168 {
		return 0, fmt.Errorf("--delay is hours (12h), days (2d) or off — a week at most")
	}
	return n * mult, nil
}

type policyAnswer struct {
	Data struct {
		Verify       bool `json:"verify"`
		DelayHours   int  `json:"delay_hours"`
		SignersReady bool `json:"signers_ready"`
		Signers      []struct {
			Fingerprint string `json:"fingerprint"`
			Principal   string `json:"principal"`
		} `json:"signers"`
		Shipped []struct {
			Fingerprint string `json:"fingerprint"`
			Principal   string `json:"principal"`
		} `json:"shipped_signers"`
		Pending *struct {
			Sha     string  `json:"sha"`
			Version *string `json:"version"`
		} `json:"pending"`
		Held *struct {
			Reason string `json:"reason"`
		} `json:"held"`
	} `json:"data"`
}

var updatesCmd = &cobra.Command{
	Use:   "updates",
	Short: "How the server updates Cipi: signed releases only, a delay",
	Long: `Every night a Cipi server installs the latest Cipi from GitHub. Two
safeguards can be added — so that an account that is taken over cannot reach
every server within a day. Both are off unless turned on.

  cipi-cli updates                                  what is set, trusted, waiting
  cipi-cli updates --delay 12h                      wait 12 hours before installing a new release
  cipi-cli updates --verify on --fingerprint SHA256:…   only releases signed by a trusted key
  cipi-cli updates --verify off --delay off

--verify on: a release is installed only when its tag v<version> is signed by
a key the server trusts, and points at what was downloaded. The first time,
the keys shipped with the release are trusted once --fingerprint matches one
of them: compare it with the one published at cipi.sh/docs/self-update.

--delay: a new release is installed once the server has seen it for that long
(hours or days, a week at most). On the server, "cipi self-update --now"
skips it; the signature is still checked.

Starting an update is not done from here.

Requires Cipi 5.18.0+, cipi/api 1.48+, updates-view and updates-manage.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]interface{}{}
		if cmd.Flags().Changed("verify") {
			v, _ := cmd.Flags().GetString("verify")
			switch strings.ToLower(v) {
			case "on", "true", "yes":
				body["verify"] = true
			case "off", "false", "no":
				body["verify"] = false
			default:
				err := fmt.Errorf("--verify is on or off")
				output.Error("%s", err)
				return err
			}
		}
		if cmd.Flags().Changed("delay") {
			v, _ := cmd.Flags().GetString("delay")
			h, err := updatesDelayHours(v)
			if err != nil {
				output.Error("%s", err)
				return err
			}
			body["delay_hours"] = h
		}
		if fp, _ := cmd.Flags().GetString("fingerprint"); fp != "" {
			if !fingerprintRe.MatchString(fp) {
				err := fmt.Errorf("--fingerprint is SHA256:… as ssh-keygen -l prints it")
				output.Error("%s", err)
				return err
			}
			body["fingerprint"] = fp
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer policyAnswer
		if len(body) == 0 {
			err = client.Get("/api/self-update/policy", &answer)
		} else {
			err = client.Put("/api/self-update/policy", body, &answer)
		}
		if err != nil {
			output.Error("%s", featureErrorHint(err, updatesAPI, "updates-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(answer)
			return nil
		}
		d := answer.Data
		output.Header("How this server updates Cipi")
		if d.Verify {
			output.KeyValue(nil, "Signature", "required")
		} else {
			output.KeyValue(nil, "Signature", "not checked (--verify on)")
		}
		if d.DelayHours > 0 {
			output.KeyValue(nil, "Delay", fmt.Sprintf("%d hours", d.DelayHours))
		} else {
			output.KeyValue(nil, "Delay", "none (--delay 12h)")
		}
		for _, s := range d.Signers {
			output.KeyValue(nil, "Trusted key", s.Fingerprint+"  "+s.Principal)
		}
		if !d.SignersReady {
			for _, s := range d.Shipped {
				output.KeyValue(nil, "Shipped key", s.Fingerprint+"  "+s.Principal+"  (compare it with cipi.sh)")
			}
		}
		if d.Pending != nil {
			v := ""
			if d.Pending.Version != nil {
				v = "v" + *d.Pending.Version + " "
			}
			short := d.Pending.Sha
			if len(short) > 12 {
				short = short[:12]
			}
			output.KeyValue(nil, "Waiting", v+short)
		}
		if d.Held != nil {
			output.KeyValue(nil, "Last held", d.Held.Reason)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	updatesCmd.Flags().String("verify", "", "Only signed releases: on or off")
	updatesCmd.Flags().String("delay", "", "Wait before installing a new release: 12h, 2d, off")
	updatesCmd.Flags().String("fingerprint", "", "The first trust: a fingerprint of the shipped keys, as published on cipi.sh")
	rootCmd.AddCommand(updatesCmd)
}
